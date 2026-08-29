package app

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/nireneko/drup/internal/runstate"
)

func TestRunHandlersPersistStatusAndRejectOutOfPhaseMutation(t *testing.T) {
	dir := newDrupalProjectDir(t)
	created, err := realHandleRunCreate(json.RawMessage(`{"project_path":` + jsonStr(dir) + `,"target_major":11,"scope":["all"]}`))
	if err != nil {
		t.Fatal(err)
	}
	var payload struct {
		Run runstate.Run `json:"run"`
	}
	if err := json.Unmarshal(created, &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Run.ID == "" || payload.Run.Phase != runstate.PhaseGitSafety {
		t.Fatalf("run_create payload = %s", created)
	}
	if _, err := realHandleRunRecord(json.RawMessage(`{"project_path":` + jsonStr(dir) + `,"run_id":` + jsonStr(payload.Run.ID) + `,"action":"record_environment","evidence":{"kind":"check","summary":"wrong"}}`)); err == nil || !strings.Contains(err.Error(), "invalid run transition") {
		t.Fatalf("out-of-order run_record error = %v, want invalid transition", err)
	}

	status, err := realHandleRunStatus(json.RawMessage(`{"project_path":` + jsonStr(dir) + `,"run_id":` + jsonStr(payload.Run.ID) + `}`))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(status, &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Run.Phase != runstate.PhaseGitSafety || len(payload.Run.AllowedActions) != 1 || payload.Run.AllowedActions[0] != runstate.ActionRecordGitSafety {
		t.Fatalf("run_status after restart-safe read = %s", status)
	}
}
