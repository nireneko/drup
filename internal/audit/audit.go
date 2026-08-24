// Package audit implements the persistent, cross-run JSONL mutation ledger
// (specs/mutation-audit): one record per mutating MCP tool invocation, a
// configurable cap on mutation volume, and the read helpers backing the
// pipeline_status tool. See internal/projectconfig for the per-project cap
// configuration this package enforces.
package audit

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/nireneko/drup/internal/projectconfig"
)

// Result values recorded for a mutating tool call.
const (
	ResultSuccess = "success"
	ResultFailure = "failure"
)

// Record is one JSONL entry in a project's mutation ledger.
type Record struct {
	Tool       string    `json:"tool"`
	ArgsHash   string    `json:"args_hash"`
	Result     string    `json:"result"`
	CommitHash string    `json:"commit_hash,omitempty"`
	Timestamp  time.Time `json:"timestamp"`
}

// logFn reports a ledger write failure. Package-level seam so tests can
// capture it instead of asserting against the real os.Stderr stream —
// mirrors internal/session's warnFn pattern.
var logFn = func(msg string) { fmt.Fprintln(os.Stderr, msg) }

// ledgerPath returns the per-project ledger file path.
func ledgerPath(projectPath string) string {
	return filepath.Join(projectPath, ".drup", "audit.jsonl")
}

// HashArgs returns the hex-encoded SHA256 of raw tool-call arguments, used
// as the ledger's args_hash field so raw (potentially sensitive) argument
// values are never persisted verbatim.
func HashArgs(raw []byte) string {
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

// Append appends one record to projectPath's mutation ledger using an
// atomic tmp-file+rename write, mirroring internal/state's atomic-write
// pattern. A write failure is logged via logFn and never returned: the
// caller's tool response must never block on ledger I/O (specs/mutation-audit
// "Ledger write does not block the tool response").
func Append(projectPath, tool string, rawArgs []byte, result, commitHash string) {
	rec := Record{
		Tool:       tool,
		ArgsHash:   HashArgs(rawArgs),
		Result:     result,
		CommitHash: commitHash,
		Timestamp:  time.Now().UTC(),
	}
	if err := appendRecord(projectPath, rec); err != nil {
		logFn(fmt.Sprintf("drup: audit ledger write failed for %q: %v", tool, err))
	}
}

// appendRecord performs the actual write. Because an atomic rename replaces
// the whole file rather than appending to it, the existing ledger is read
// first and the new line is appended in memory before the tmp+rename swap —
// this keeps every write atomic (no reader ever observes a partial line)
// while still preserving ledger history, unlike internal/state's Save
// (which always overwrites the single current state).
func appendRecord(projectPath string, rec Record) error {
	dir := filepath.Join(projectPath, ".drup")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	path := ledgerPath(projectPath)
	existing, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}

	line, err := json.Marshal(rec)
	if err != nil {
		return err
	}
	data := append(existing, append(line, '\n')...)

	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmpPath, path)
}

// ReadAll returns every record in projectPath's ledger. A missing ledger
// file returns an empty slice and no error (specs/mutation-audit's
// pipeline_status "Status on empty ledger" scenario). A line that fails to
// parse is skipped rather than failing the whole read — one corrupt line
// must not hide every other recorded mutation.
func ReadAll(projectPath string) ([]Record, error) {
	data, err := os.ReadFile(ledgerPath(projectPath))
	if err != nil {
		if os.IsNotExist(err) {
			return []Record{}, nil
		}
		return nil, err
	}

	records := []Record{}
	for _, line := range strings.Split(strings.TrimRight(string(data), "\n"), "\n") {
		if line == "" {
			continue
		}
		var r Record
		if err := json.Unmarshal([]byte(line), &r); err != nil {
			continue
		}
		records = append(records, r)
	}
	return records, nil
}

// Count returns how many ledger records for projectPath have a timestamp at
// or after since.
func Count(projectPath string, since time.Time) (int, error) {
	records, err := ReadAll(projectPath)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, r := range records {
		if !r.Timestamp.Before(since) {
			n++
		}
	}
	return n, nil
}

// CheckCap enforces the mutation cap for projectPath (specs/mutation-audit's
// Mutation Caps requirement). When hasSession is true, the cap is
// per-session: only mutations logged since sessionOpenedAt count against
// projectconfig's MutationCapPerSession. Otherwise it is per-day: mutations
// logged in the trailing 24h count against MutationCapPerDay (the
// agent-session opt-out path). projectconfig.Load already applies a safe
// built-in default for either cap when .drup/config.json has none
// configured, so an unconfigured project is never left with an unlimited
// cap. A ledger read error fails OPEN (allowed=true) rather than blocking
// every future mutation on a corrupted/unreadable ledger — the guard's other
// layers (session, backup-freshness) remain the primary safety net.
func CheckCap(projectPath string, hasSession bool, sessionOpenedAt time.Time) (allowed bool, count, capN int, err error) {
	cfg := projectconfig.Load(projectPath)

	since := sessionOpenedAt
	capN = cfg.MutationCapPerSession
	if !hasSession {
		since = time.Now().Add(-24 * time.Hour)
		capN = cfg.MutationCapPerDay
	}

	count, err = Count(projectPath, since)
	if err != nil {
		return true, count, capN, err
	}
	return count < capN, count, capN, nil
}
