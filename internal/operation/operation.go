// Package operation provides the fail-closed, request-keyed authority for
// mutating MCP operations. It intentionally does not reuse audit: audit is a
// fail-open history while this store decides whether an effect may run.
package operation

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const ledgerVersion = 1

var (
	ErrUnavailable       = errors.New("operation ledger unavailable")
	ErrNotFound          = errors.New("operation not found")
	ErrRequestExists     = errors.New("operation request already exists")
	ErrIdentityMismatch  = errors.New("operation request identity mismatch")
	ErrEquivalentUnknown = errors.New("equivalent operation outcome is unknown")
	ErrInvalidTransition = errors.New("invalid operation state transition")
)

// State is the durable result of a mutation attempt.
type State string

const (
	StateStarted   State = "started"
	StateCompleted State = "completed"
	StateFailed    State = "failed"
	StateUnknown   State = "unknown"
)

// Evidence is observed during reconciliation. Client assertions are never
// stored as evidence; the app boundary verifies an observable first.
type Evidence struct {
	Kind       string    `json:"kind"`
	Value      string    `json:"value"`
	ObservedAt time.Time `json:"observed_at"`
}

// Operation identifies exactly one mutation request and its durable outcome.
type Operation struct {
	Root        string          `json:"root"`
	RequestID   string          `json:"request_id"`
	Tool        string          `json:"tool"`
	Fingerprint string          `json:"fingerprint"`
	State       State           `json:"state"`
	Response    json.RawMessage `json:"response,omitempty"`
	Error       string          `json:"error,omitempty"`
	Evidence    []Evidence      `json:"evidence,omitempty"`
	StartedAt   time.Time       `json:"started_at"`
	UpdatedAt   time.Time       `json:"updated_at"`
}

type document struct {
	Version    int         `json:"version"`
	Operations []Operation `json:"operations"`
}

// Store is scoped to a single Drupal project root.
type Store struct{ projectPath string }

// OutcomeError tells the transport that the durable operation result is not
// an ordinary handler failure. In particular, unknown must never be retried.
type OutcomeError struct {
	State State
	Err   error
}

func (e *OutcomeError) Error() string          { return e.Err.Error() }
func (e *OutcomeError) Unwrap() error          { return e.Err }
func (e *OutcomeError) OperationState() string { return string(e.State) }

func UnknownError(err error) error { return &OutcomeError{State: StateUnknown, Err: err} }

func IsUnknown(err error) bool {
	var outcome *OutcomeError
	return errors.As(err, &outcome) && outcome.State == StateUnknown
}

var ledgerMu sync.Mutex

func NewStore(projectPath string) *Store { return &Store{projectPath: projectPath} }

func (s *Store) path() string { return filepath.Join(s.projectPath, ".drup", "operations.v1.json") }

// Fingerprint returns a stable semantic identity for a tool invocation. It
// normalizes JSON object ordering and deliberately excludes request_id.
func Fingerprint(tool string, raw json.RawMessage) (string, error) {
	var fields map[string]interface{}
	if err := json.Unmarshal(raw, &fields); err != nil {
		return "", fmt.Errorf("decode operation arguments: %w", err)
	}
	delete(fields, "request_id")
	canonical, err := json.Marshal(fields)
	if err != nil {
		return "", fmt.Errorf("canonicalize operation arguments: %w", err)
	}
	sum := sha256.Sum256(append([]byte(tool+"\x00"), canonical...))
	return hex.EncodeToString(sum[:]), nil
}

func (s *Store) FindRequest(requestID string) (Operation, error) {
	ledgerMu.Lock()
	defer ledgerMu.Unlock()
	doc, err := s.read()
	if err != nil {
		return Operation{}, err
	}
	for _, op := range doc.Operations {
		if op.RequestID == requestID {
			return op, nil
		}
	}
	return Operation{}, ErrNotFound
}

// Start persists the intent before any effect may be attempted.
func (s *Store) Start(requestID, tool, fingerprint string) (Operation, error) {
	ledgerMu.Lock()
	defer ledgerMu.Unlock()
	if requestID == "" || tool == "" || fingerprint == "" {
		return Operation{}, fmt.Errorf("%w: request_id, tool, and fingerprint are required", ErrUnavailable)
	}
	doc, err := s.read()
	if err != nil {
		return Operation{}, err
	}
	for _, op := range doc.Operations {
		if op.RequestID == requestID {
			if op.Tool != tool || op.Fingerprint != fingerprint {
				return Operation{}, ErrIdentityMismatch
			}
			return op, ErrRequestExists
		}
		if op.Tool == tool && op.Fingerprint == fingerprint && op.State == StateUnknown {
			return Operation{}, ErrEquivalentUnknown
		}
	}
	now := time.Now().UTC()
	op := Operation{Root: s.projectPath, RequestID: requestID, Tool: tool, Fingerprint: fingerprint, State: StateStarted, StartedAt: now, UpdatedAt: now}
	doc.Operations = append(doc.Operations, op)
	if err := s.write(doc); err != nil {
		return Operation{}, err
	}
	return op, nil
}

func (s *Store) Complete(requestID string, response json.RawMessage) (Operation, error) {
	return s.transition(requestID, StateCompleted, response, "", nil)
}

func (s *Store) Fail(requestID string, reason string) (Operation, error) {
	return s.transition(requestID, StateFailed, nil, reason, nil)
}

// FailWithResponse records a domain-declared failure (for example a
// {"success":false} payload returned with nil Go error) without treating it
// as a confirmed successful operation.
func (s *Store) FailWithResponse(requestID string, response json.RawMessage, reason string) (Operation, error) {
	return s.transition(requestID, StateFailed, response, reason, nil)
}

func (s *Store) Unknown(requestID string, reason string) (Operation, error) {
	return s.transition(requestID, StateUnknown, nil, reason, nil)
}

// Reconcile resolves an unknown operation only after the caller supplied
// independently observed evidence.
func (s *Store) Reconcile(requestID string, evidence Evidence, response json.RawMessage) (Operation, error) {
	if evidence.Kind == "" || evidence.Value == "" {
		return Operation{}, fmt.Errorf("%w: evidence kind and value are required", ErrUnavailable)
	}
	if evidence.ObservedAt.IsZero() {
		evidence.ObservedAt = time.Now().UTC()
	}
	return s.transition(requestID, StateCompleted, response, "", []Evidence{evidence})
}

func (s *Store) transition(requestID string, to State, response json.RawMessage, reason string, evidence []Evidence) (Operation, error) {
	ledgerMu.Lock()
	defer ledgerMu.Unlock()
	doc, err := s.read()
	if err != nil {
		return Operation{}, err
	}
	for i := range doc.Operations {
		op := &doc.Operations[i]
		if op.RequestID != requestID {
			continue
		}
		if op.State != StateStarted && !(op.State == StateUnknown && to == StateCompleted) {
			return Operation{}, fmt.Errorf("%w: %s -> %s", ErrInvalidTransition, op.State, to)
		}
		op.State = to
		op.Response = append(op.Response[:0], response...)
		op.Error = reason
		if len(evidence) > 0 {
			op.Evidence = append(op.Evidence, evidence...)
		}
		op.UpdatedAt = time.Now().UTC()
		if err := s.write(doc); err != nil {
			return Operation{}, err
		}
		return *op, nil
	}
	return Operation{}, ErrNotFound
}

func (s *Store) read() (document, error) {
	data, err := os.ReadFile(s.path())
	if err != nil {
		if os.IsNotExist(err) {
			return document{Version: ledgerVersion, Operations: []Operation{}}, nil
		}
		return document{}, fmt.Errorf("%w: read ledger: %v", ErrUnavailable, err)
	}
	var doc document
	if err := json.Unmarshal(data, &doc); err != nil || doc.Version != ledgerVersion {
		if err != nil {
			return document{}, fmt.Errorf("%w: corrupt ledger: %v", ErrUnavailable, err)
		}
		return document{}, fmt.Errorf("%w: unsupported ledger version %d", ErrUnavailable, doc.Version)
	}
	return doc, nil
}

func (s *Store) write(doc document) error {
	dir := filepath.Dir(s.path())
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("%w: create ledger directory: %v", ErrUnavailable, err)
	}
	data, err := json.Marshal(doc)
	if err != nil {
		return fmt.Errorf("%w: encode ledger: %v", ErrUnavailable, err)
	}
	tmp, err := os.CreateTemp(dir, ".operations.*.tmp")
	if err != nil {
		return fmt.Errorf("%w: create temporary ledger: %v", ErrUnavailable, err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("%w: write temporary ledger: %v", ErrUnavailable, err)
	}
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return fmt.Errorf("%w: set ledger mode: %v", ErrUnavailable, err)
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return fmt.Errorf("%w: sync temporary ledger: %v", ErrUnavailable, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("%w: close temporary ledger: %v", ErrUnavailable, err)
	}
	if err := os.Rename(tmpName, s.path()); err != nil {
		return fmt.Errorf("%w: replace ledger: %v", ErrUnavailable, err)
	}
	return nil
}
