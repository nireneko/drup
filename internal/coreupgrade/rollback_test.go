package coreupgrade

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRollback_RoundTrip_RestoresComposerJSON(t *testing.T) {
	requireGit(t)
	dir := t.TempDir()
	initGitRepo(t, dir)
	writeComposerFixture(t, dir)
	runGit(t, dir, "add", ".")
	runGit(t, dir, "commit", "-m", "initial")

	applyResult, err := Apply(dir, "11.0.9", false)
	if err != nil {
		t.Fatalf("Apply error: %v", err)
	}
	if !applyResult.Success {
		t.Fatalf("Apply did not succeed: %s", applyResult.Report)
	}

	if err := Rollback(dir, applyResult.RollbackCheckpoint); err != nil {
		t.Fatalf("Rollback error: %v", err)
	}

	after, err := os.ReadFile(filepath.Join(dir, "composer.json"))
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Require map[string]string `json:"require"`
	}
	if err := json.Unmarshal(after, &doc); err != nil {
		t.Fatal(err)
	}
	if doc.Require["drupal/core-recommended"] != "^10.1" {
		t.Errorf("after rollback, drupal/core-recommended = %q, want %q (restored)", doc.Require["drupal/core-recommended"], "^10.1")
	}
}

func TestRollback_EmptyCheckpoint(t *testing.T) {
	requireGit(t)
	dir := t.TempDir()
	initGitRepo(t, dir)
	writeComposerFixture(t, dir)
	runGit(t, dir, "add", ".")
	runGit(t, dir, "commit", "-m", "initial")

	err := Rollback(dir, "")
	if err == nil {
		t.Fatal("expected error for empty checkpoint SHA, got nil")
	}
}

func TestRollback_PathTraversalRejected(t *testing.T) {
	err := Rollback("/tmp/../../etc", "deadbeef")
	if err == nil {
		t.Fatal("expected error for path traversal, got nil")
	}
	if !strings.Contains(err.Error(), "..") {
		t.Errorf("error = %q, want it to mention '..'", err.Error())
	}
}

func TestRollback_InvalidCheckpoint(t *testing.T) {
	requireGit(t)
	dir := t.TempDir()
	initGitRepo(t, dir)
	writeComposerFixture(t, dir)
	runGit(t, dir, "add", ".")
	runGit(t, dir, "commit", "-m", "initial")

	err := Rollback(dir, "not-a-real-sha")
	if err == nil {
		t.Fatal("expected error for invalid checkpoint SHA, got nil")
	}
}
