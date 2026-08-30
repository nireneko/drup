package backup

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// RestoreState is persisted before every external restore boundary. A state
// other than completed is intentionally a recovery stop, never a retry hint.
type RestoreState string

const (
	RestorePreflight        RestoreState = "preflight"
	RestoreRescueCreated    RestoreState = "rescue_created"
	RestoreFilesStaged      RestoreState = "files_staged"
	RestoreDatabaseRestored RestoreState = "database_restored"
	RestoreFilesystemSwap   RestoreState = "filesystem_swapping"
	RestoreCompleted        RestoreState = "completed"
	RestoreRecoveryRequired RestoreState = "recovery_required"
)

// RestoreJournal gives an operator an explicit recovery procedure. Database
// import is not generally atomic, so a failure after it is always reported as
// a recoverable window rather than claimed as a rolled-back transaction.
type RestoreJournal struct {
	Version      int          `json:"version"`
	ID           string       `json:"id"`
	BackupID     string       `json:"backup_id"`
	RescueBackup string       `json:"rescue_backup_id,omitempty"`
	State        RestoreState `json:"state"`
	PreviousPath string       `json:"previous_path,omitempty"`
	DatabaseMode string       `json:"database_mode"`
	Continuation string       `json:"continuation"`
	Error        string       `json:"error,omitempty"`
	UpdatedAt    time.Time    `json:"updated_at"`
}

func restoreJournalDir(project string) string { return filepath.Join(project, ".drup", "restores") }
func restoreJournalPath(project, id string) string {
	return filepath.Join(restoreJournalDir(project), id+".json")
}

// HasIncompleteRestore is the read-only upgrade interlock. Unknown or corrupt
// journal records fail closed: they are evidence of an ambiguous prior effect.
func HasIncompleteRestore(project string) bool {
	project, err := validateProject(project)
	if err != nil {
		return true
	}
	journals, err := ListRestoreJournals(project)
	if err != nil {
		return true
	}
	for _, journal := range journals {
		if journal.State != RestoreCompleted {
			return true
		}
	}
	return false
}

func ListRestoreJournals(project string) ([]RestoreJournal, error) {
	project, err := validateProject(project)
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(restoreJournalDir(project))
	if os.IsNotExist(err) {
		return []RestoreJournal{}, nil
	}
	if err != nil {
		return nil, err
	}
	result := make([]RestoreJournal, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(restoreJournalDir(project), entry.Name()))
		if err != nil {
			return nil, fmt.Errorf("read restore journal: %w", err)
		}
		var journal RestoreJournal
		if err := json.Unmarshal(data, &journal); err != nil || journal.Version != 1 || journal.ID == "" {
			return nil, fmt.Errorf("invalid restore journal")
		}
		result = append(result, journal)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].UpdatedAt.After(result[j].UpdatedAt) })
	return result, nil
}

func writeRestoreJournal(project string, journal *RestoreJournal) error {
	journal.UpdatedAt = time.Now().UTC()
	if err := os.MkdirAll(restoreJournalDir(project), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(journal, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(restoreJournalDir(project), ".restore-*.tmp")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name)
	if _, err = tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err = tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return err
	}
	if err = tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err = tmp.Close(); err != nil {
		return err
	}
	return os.Rename(name, restoreJournalPath(project, journal.ID))
}

func recovery(journal *RestoreJournal, err error) error {
	journal.State = RestoreRecoveryRequired
	journal.Error = sanitizeRestoreError(err)
	journal.Continuation = "inspect rescue_backup_id and previous_path; database restore is non-atomic and must be reconciled before retrying"
	return err
}

func sanitizeRestoreError(err error) string {
	if err == nil {
		return ""
	}
	return strings.TrimSpace(err.Error())
}

// restoreTransactional is intentionally private until callers can consume the
// structured journal. Restore retains its compatibility signature while now
// providing transactional filesystem recovery and an explicit DB window.
func (m *Manager) restoreTransactional(project, id string) (err error) {
	dir := filepath.Join(project, ".drup", "backups", id)
	var manifest Manifest
	data, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if os.IsNotExist(err) {
		return fmt.Errorf("backup not found: %s", id)
	}
	if err != nil {
		return err
	}
	if json.Unmarshal(data, &manifest) != nil {
		return fmt.Errorf("checksum failure: invalid manifest")
	}
	files, db := filepath.Join(dir, "files.tar.gz"), filepath.Join(dir, "database.sql.gz")
	if manifest.FilesChecksum == "" || manifest.DatabaseChecksum == "" || checksum(files) != manifest.FilesChecksum || checksum(db) != manifest.DatabaseChecksum {
		return fmt.Errorf("checksum failure")
	}

	journal := &RestoreJournal{Version: 1, ID: time.Now().UTC().Format("20060102T150405.000000000Z") + "-" + randomID(), BackupID: id, State: RestorePreflight, DatabaseMode: "non_atomic", Continuation: "create an independent rescue backup before importing the database"}
	if err = writeRestoreJournal(project, journal); err != nil {
		return err
	}
	fail := func(cause error) error { _ = writeRestoreJournal(project, journal); return cause }

	rescue, cause := m.Create(project)
	if cause != nil {
		return fail(recovery(journal, fmt.Errorf("rescue backup failure: %w", cause)))
	}
	journal.RescueBackup = rescue.BackupID
	journal.State = RestoreRescueCreated
	journal.Continuation = "restore files into same-volume staging before database import"
	if cause = writeRestoreJournal(project, journal); cause != nil {
		return cause
	}

	stage, cause := os.MkdirTemp(filepath.Dir(project), ".drup-restore-stage-")
	if cause != nil {
		return fail(recovery(journal, cause))
	}
	defer os.RemoveAll(stage)
	if cause = extract(files, stage); cause != nil {
		return fail(recovery(journal, fmt.Errorf("filesystem restore failure: %w", cause)))
	}
	journal.State = RestoreFilesStaged
	journal.Continuation = "database import is next and is a declared non-atomic recovery window"
	if cause = writeRestoreJournal(project, journal); cause != nil {
		return cause
	}

	detection, cause := detectEnv(project)
	if cause != nil {
		return fail(recovery(journal, cause))
	}
	in, cause := os.Open(db)
	if cause != nil {
		return fail(recovery(journal, cause))
	}
	_, stderr, code, runErr := runInput(detection.CommandPrefix, in, "drush", "sql:cli", "--root="+project)
	in.Close()
	if runErr != nil || code != 0 {
		return fail(recovery(journal, fmt.Errorf("database restore failure: %s", commandError(runErr, stderr, code))))
	}
	journal.State = RestoreDatabaseRestored
	journal.Continuation = "filesystem swap is next; retain rescue backup and previous tree until verification"
	if cause = writeRestoreJournal(project, journal); cause != nil {
		return cause
	}

	journal.State = RestoreFilesystemSwap
	journal.PreviousPath = filepath.Join(restoreJournalDir(project), journal.ID, "previous")
	journal.Continuation = "if interrupted, restore the preserved previous tree and reconcile the non-atomic database"
	if cause = writeRestoreJournal(project, journal); cause != nil {
		return cause
	}
	if cause = swapProject(project, stage, journal.PreviousPath); cause != nil {
		return fail(recovery(journal, fmt.Errorf("filesystem restore failure: %w", cause)))
	}
	journal.State = RestoreCompleted
	journal.Continuation = "restore completed; rescue backup and previous tree are retained for explicit operator cleanup"
	journal.Error = ""
	return writeRestoreJournal(project, journal)
}

var movePath = os.Rename

func swapProject(project, stage, previous string) error {
	if err := os.MkdirAll(previous, 0o700); err != nil {
		return err
	}
	current, err := os.ReadDir(project)
	if err != nil {
		return err
	}
	moved := make([]string, 0, len(current))
	for _, entry := range current {
		if entry.Name() == ".drup" {
			continue
		}
		if err := movePath(filepath.Join(project, entry.Name()), filepath.Join(previous, entry.Name())); err != nil {
			rollbackSwap(project, previous, moved)
			return err
		}
		moved = append(moved, entry.Name())
	}
	staged, err := os.ReadDir(stage)
	if err != nil {
		rollbackSwap(project, previous, moved)
		return err
	}
	installed := make([]string, 0, len(staged))
	for _, entry := range staged {
		if entry.Name() == ".drup" {
			continue
		}
		if err := movePath(filepath.Join(stage, entry.Name()), filepath.Join(project, entry.Name())); err != nil {
			rollbackInstalled(project, stage, previous, installed, moved)
			return err
		}
		installed = append(installed, entry.Name())
	}
	return nil
}

func rollbackSwap(project, previous string, moved []string) {
	for i := len(moved) - 1; i >= 0; i-- {
		_ = movePath(filepath.Join(previous, moved[i]), filepath.Join(project, moved[i]))
	}
}
func rollbackInstalled(project, stage, previous string, installed, moved []string) {
	for i := len(installed) - 1; i >= 0; i-- {
		_ = movePath(filepath.Join(project, installed[i]), filepath.Join(stage, installed[i]))
	}
	rollbackSwap(project, previous, moved)
}
