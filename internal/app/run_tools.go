package app

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/nireneko/drup/internal/runstate"
	"github.com/nireneko/drup/internal/session"
)

type runParams struct {
	ProjectPath string `json:"project_path"`
	RunID       string `json:"run_id"`
}

func canonicalRunStore(projectPath string) (*runstate.Store, string, error) {
	if projectPath == "" {
		return nil, "", fmt.Errorf("project_path is required")
	}
	root, err := session.CanonicalRoot(projectPath)
	if err != nil {
		return nil, "", err
	}
	return runstate.NewStore(root), root, nil
}

func realHandleRunCreate(args json.RawMessage) (json.RawMessage, error) {
	var params struct {
		ProjectPath    string   `json:"project_path"`
		TargetMajor    int      `json:"target_major"`
		CommitStrategy string   `json:"commit_strategy"`
		Scope          []string `json:"scope"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return nil, err
	}
	store, _, err := canonicalRunStore(params.ProjectPath)
	if err != nil {
		return nil, err
	}
	run, err := store.Create(runstate.CreateInput{
		ID:             newRunID(),
		TargetMajor:    params.TargetMajor,
		CommitStrategy: params.CommitStrategy,
		Scope:          params.Scope,
	})
	if err != nil {
		return nil, err
	}
	return json.Marshal(map[string]interface{}{"success": true, "run": run})
}

func realHandleRunStatus(args json.RawMessage) (json.RawMessage, error) {
	var params runParams
	if err := json.Unmarshal(args, &params); err != nil {
		return nil, err
	}
	store, _, err := canonicalRunStore(params.ProjectPath)
	if err != nil {
		return nil, err
	}
	run, err := getRequestedRun(store, params.RunID)
	if err != nil {
		return nil, err
	}
	return json.Marshal(map[string]interface{}{"success": true, "run": run})
}

func realHandleRunRecord(args json.RawMessage) (json.RawMessage, error) {
	var params struct {
		runParams
		Action   runstate.Action `json:"action"`
		Evidence struct {
			Kind    string          `json:"kind"`
			Summary string          `json:"summary"`
			Payload json.RawMessage `json:"payload"`
		} `json:"evidence"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return nil, err
	}
	store, _, err := canonicalRunStore(params.ProjectPath)
	if err != nil {
		return nil, err
	}
	if params.RunID == "" {
		return nil, fmt.Errorf("run_id is required")
	}
	run, err := store.Record(params.RunID, runstate.RecordInput{Action: params.Action, Kind: params.Evidence.Kind, Summary: params.Evidence.Summary, Payload: params.Evidence.Payload})
	if err != nil {
		return nil, err
	}
	return json.Marshal(map[string]interface{}{"success": true, "run": run})
}

func realHandleRunConfirm(args json.RawMessage) (json.RawMessage, error) {
	var params struct {
		runParams
		Action runstate.Action `json:"action"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return nil, err
	}
	store, _, err := canonicalRunStore(params.ProjectPath)
	if err != nil {
		return nil, err
	}
	if params.RunID == "" {
		return nil, fmt.Errorf("run_id is required")
	}
	run, err := store.Confirm(params.RunID, params.Action)
	if err != nil {
		return nil, err
	}
	return json.Marshal(map[string]interface{}{"success": true, "run": run})
}

func realHandleRunBlock(args json.RawMessage) (json.RawMessage, error) {
	var params struct {
		runParams
		Reason string `json:"reason"`
		Target string `json:"target"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return nil, err
	}
	store, _, err := canonicalRunStore(params.ProjectPath)
	if err != nil {
		return nil, err
	}
	if params.RunID == "" {
		return nil, fmt.Errorf("run_id is required")
	}
	run, err := store.Block(params.RunID, params.Reason, params.Target)
	if err != nil {
		return nil, err
	}
	return json.Marshal(map[string]interface{}{"success": true, "run": run})
}

func realHandleRunAbandon(args json.RawMessage) (json.RawMessage, error) {
	var params struct {
		runParams
		Reason string `json:"reason"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return nil, err
	}
	store, _, err := canonicalRunStore(params.ProjectPath)
	if err != nil {
		return nil, err
	}
	if params.RunID == "" {
		return nil, fmt.Errorf("run_id is required")
	}
	run, err := store.Abandon(params.RunID, params.Reason)
	if err != nil {
		return nil, err
	}
	return json.Marshal(map[string]interface{}{"success": true, "run": run})
}

func getRequestedRun(store *runstate.Store, id string) (runstate.Run, error) {
	if id != "" {
		return store.Get(id)
	}
	return store.Active()
}

func newRunID() string {
	var suffix [4]byte
	if _, err := rand.Read(suffix[:]); err != nil {
		return fmt.Sprintf("run-%d", time.Now().UTC().UnixNano())
	}
	return fmt.Sprintf("run-%s-%s", time.Now().UTC().Format("20060102T150405.000000000Z"), hex.EncodeToString(suffix[:]))
}
