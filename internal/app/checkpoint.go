package app

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/nireneko/drup/internal/gitops"
	"github.com/nireneko/drup/internal/runstate"
)

// CheckpointCommitInput is shared by the CLI and MCP adapters. It carries no
// derived authority: strategy and scope are checked against the persisted run
// and the candidate identity is recomputed from Git immediately before commit.
type CheckpointCommitInput struct {
	ProjectPath    string
	RunID          string
	Strategy       runstate.CommitStrategy
	Scope          []string
	Paths          []string
	ValidationHash string
	Target         string
	Message        string
}

type CheckpointCommitResult struct {
	Success       bool                    `json:"success"`
	Skipped       bool                    `json:"skipped"`
	Strategy      runstate.CommitStrategy `json:"commit_strategy"`
	CommitHash    string                  `json:"commit_hash,omitempty"`
	CandidateHash string                  `json:"candidate_hash,omitempty"`
	ChangedFiles  []string                `json:"changed_files,omitempty"`
	Message       string                  `json:"message"`
}

// CheckpointCommit is the sole publication service. Mutation tools leave a
// reportable diff; this function proves the diff still equals independently
// recorded validation evidence before asking gitops for a scoped commit.
func CheckpointCommit(input CheckpointCommitInput) (CheckpointCommitResult, error) {
	if input.ProjectPath == "" || input.RunID == "" {
		return CheckpointCommitResult{}, fmt.Errorf("project_path and run_id are required")
	}
	store, root, err := canonicalRunStore(input.ProjectPath)
	if err != nil {
		return CheckpointCommitResult{}, err
	}
	_ = root
	run, err := store.Get(input.RunID)
	if err != nil {
		return CheckpointCommitResult{}, err
	}
	strategy := input.Strategy
	if strategy == "" {
		strategy = run.CommitStrategy
	}
	paths := normalizedStrings(input.Paths)
	if strategy == runstate.CommitStrategyNone && !requiresCheckpointEvidence(run) {
		if _, err := store.AuthorizeCheckpoint(run.ID, runstate.CheckpointInput{Strategy: strategy, Scope: normalizedStrings(input.Scope)}); err != nil {
			return CheckpointCommitResult{}, err
		}
		return CheckpointCommitResult{Success: true, Skipped: true, Strategy: strategy, Message: "commit strategy none leaves the validated diff unpublished"}, nil
	}
	candidate, err := gitops.CandidateForPaths(run.Root, paths)
	if err != nil {
		return CheckpointCommitResult{}, err
	}
	checkpoint := runstate.CheckpointInput{
		Strategy: strategy, Scope: normalizedStrings(input.Scope), Paths: candidate.Paths,
		ValidationHash: input.ValidationHash, CandidateHash: candidate.Hash, Target: input.Target,
	}
	if _, err := store.AuthorizeCheckpoint(run.ID, checkpoint); err != nil {
		return CheckpointCommitResult{}, err
	}
	if strategy == runstate.CommitStrategyNone {
		return CheckpointCommitResult{Success: true, Skipped: true, Strategy: strategy, CandidateHash: candidate.Hash, ChangedFiles: candidate.Paths, Message: "commit strategy none leaves the validated diff unpublished"}, nil
	}
	message := strings.TrimSpace(input.Message)
	if message == "" {
		message = fmt.Sprintf("chore(checkpoint): validated %s upgrade changes", input.Target)
	}
	commitHash, err := gitops.Commit(run.Root, message, candidate.Paths)
	if err != nil {
		return CheckpointCommitResult{}, err
	}
	if _, err := store.RecordCheckpoint(run.ID, checkpoint, commitHash); err != nil {
		return CheckpointCommitResult{}, fmt.Errorf("checkpoint commit %s created but its receipt could not be persisted: %w", commitHash, err)
	}
	return CheckpointCommitResult{Success: true, Strategy: strategy, CommitHash: commitHash, CandidateHash: candidate.Hash, ChangedFiles: candidate.Paths, Message: "validated checkpoint committed"}, nil
}

func requiresCheckpointEvidence(run runstate.Run) bool {
	switch run.Phase {
	case runstate.PhaseCustomTheme, runstate.PhaseContribPatch, runstate.PhaseContribMinor, runstate.PhaseContribMajor, runstate.PhaseCoreLoop, runstate.PhaseCleanup:
		return true
	default:
		return run.CheckpointPlan != nil || len(run.CheckpointHistory) > 0
	}
}

func normalizedStrings(values []string) []string {
	result := append([]string(nil), values...)
	sort.Strings(result)
	return result
}

func realHandleCheckpointCommit(args json.RawMessage) (json.RawMessage, error) {
	var params struct {
		ProjectPath    string                  `json:"project_path"`
		RunID          string                  `json:"run_id"`
		Strategy       runstate.CommitStrategy `json:"commit_strategy"`
		Scope          []string                `json:"scope"`
		Paths          []string                `json:"paths"`
		ValidationHash string                  `json:"validation_hash"`
		Target         string                  `json:"target"`
		Message        string                  `json:"commit_message"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return nil, err
	}
	result, err := CheckpointCommit(CheckpointCommitInput{
		ProjectPath: params.ProjectPath, RunID: params.RunID, Strategy: params.Strategy,
		Scope: params.Scope, Paths: params.Paths, ValidationHash: params.ValidationHash,
		Target: params.Target, Message: params.Message,
	})
	if err != nil {
		return nil, err
	}
	return json.Marshal(result)
}

// RunCheckpointCommit is the CLI adapter for the shared publication service.
// Arguments use only explicit named flags so callers cannot accidentally turn
// a positional path into a commit message or validation hash.
func RunCheckpointCommit(args []string) error {
	params := CheckpointCommitInput{Scope: []string{}, Paths: []string{}}
	for _, arg := range args {
		switch {
		case strings.HasPrefix(arg, "--project-path="):
			params.ProjectPath = strings.TrimPrefix(arg, "--project-path=")
		case strings.HasPrefix(arg, "--run-id="):
			params.RunID = strings.TrimPrefix(arg, "--run-id=")
		case strings.HasPrefix(arg, "--commit-strategy="):
			params.Strategy = runstate.CommitStrategy(strings.TrimPrefix(arg, "--commit-strategy="))
		case strings.HasPrefix(arg, "--scope="):
			params.Scope = strings.Split(strings.TrimPrefix(arg, "--scope="), ",")
		case strings.HasPrefix(arg, "--paths="):
			params.Paths = strings.Split(strings.TrimPrefix(arg, "--paths="), ",")
		case strings.HasPrefix(arg, "--validation-hash="):
			params.ValidationHash = strings.TrimPrefix(arg, "--validation-hash=")
		case strings.HasPrefix(arg, "--target="):
			params.Target = strings.TrimPrefix(arg, "--target=")
		case strings.HasPrefix(arg, "--message="):
			params.Message = strings.TrimPrefix(arg, "--message=")
		default:
			return fmt.Errorf("unknown checkpoint-commit argument %q", arg)
		}
	}
	result, err := CheckpointCommit(params)
	if err != nil {
		return err
	}
	return json.NewEncoder(os.Stdout).Encode(result)
}
