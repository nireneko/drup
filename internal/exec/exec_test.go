package exec

import (
	"testing"
)

func TestRun_CapturesStdout(t *testing.T) {
	stdout, stderr, exitCode, err := Run("echo", "hello world")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if exitCode != 0 {
		t.Errorf("exit code = %d, want 0", exitCode)
	}
	if stdout != "hello world\n" {
		t.Errorf("stdout = %q, want %q", stdout, "hello world\n")
	}
	if stderr != "" {
		t.Errorf("stderr = %q, want empty", stderr)
	}
}

func TestRun_CapturesStderr(t *testing.T) {
	stdout, stderr, exitCode, err := Run("sh", "-c", "echo error >&2")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if exitCode != 0 {
		t.Errorf("exit code = %d, want 0", exitCode)
	}
	if stdout != "" {
		t.Errorf("stdout = %q, want empty", stdout)
	}
	if stderr != "error\n" {
		t.Errorf("stderr = %q, want %q", stderr, "error\n")
	}
}

func TestRun_NonZeroExitCode(t *testing.T) {
	_, _, exitCode, err := Run("sh", "-c", "exit 42")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if exitCode != 42 {
		t.Errorf("exit code = %d, want 42", exitCode)
	}
}

func TestRun_CommandNotFound(t *testing.T) {
	_, _, _, err := Run("nonexistent-command-xyz")
	if err == nil {
		t.Fatal("expected error for missing command, got nil")
	}
}

// mockRunner implements commandRunner for testing.
type mockRunner struct {
	stdout   string
	stderr   string
	exitCode int
	err      error
}

func (m *mockRunner) Output() (string, string, int, error) {
	return m.stdout, m.stderr, m.exitCode, m.err
}

func TestRunWithEnv_PrefixPrepended(t *testing.T) {
	orig := execCommand
	defer func() { execCommand = orig }()

	var calledCmd string
	var calledArgs []string
	execCommand = func(cmd string, args ...string) commandRunner {
		calledCmd = cmd
		calledArgs = args
		return &mockRunner{stdout: "ok\n", stderr: "", exitCode: 0}
	}

	stdout, _, exitCode, err := RunWithEnv([]string{"ddev"}, "composer", "require", "drupal/token")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if calledCmd != "ddev" {
		t.Errorf("called cmd = %q, want %q", calledCmd, "ddev")
	}
	wantArgs := []string{"composer", "require", "drupal/token"}
	if len(calledArgs) != len(wantArgs) {
		t.Fatalf("called args len = %d, want %d", len(calledArgs), len(wantArgs))
	}
	for i, a := range wantArgs {
		if calledArgs[i] != a {
			t.Errorf("calledArgs[%d] = %q, want %q", i, calledArgs[i], a)
		}
	}
	if stdout != "ok\n" {
		t.Errorf("stdout = %q, want %q", stdout, "ok\n")
	}
	if exitCode != 0 {
		t.Errorf("exit code = %d, want 0", exitCode)
	}
}

func TestRunWithEnv_EmptyPrefix(t *testing.T) {
	orig := execCommand
	defer func() { execCommand = orig }()

	var calledCmd string
	var calledArgs []string
	execCommand = func(cmd string, args ...string) commandRunner {
		calledCmd = cmd
		calledArgs = args
		return &mockRunner{stdout: "direct\n", stderr: "", exitCode: 0}
	}

	stdout, _, exitCode, err := RunWithEnv(nil, "git", "status")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if calledCmd != "git" {
		t.Errorf("called cmd = %q, want %q", calledCmd, "git")
	}
	if len(calledArgs) != 1 || calledArgs[0] != "status" {
		t.Errorf("called args = %v, want [status]", calledArgs)
	}
	if stdout != "direct\n" {
		t.Errorf("stdout = %q, want %q", stdout, "direct\n")
	}
	if exitCode != 0 {
		t.Errorf("exit code = %d, want 0", exitCode)
	}
}

func TestRunWithEnv_MultiTokenPrefix(t *testing.T) {
	orig := execCommand
	defer func() { execCommand = orig }()

	var calledCmd string
	var calledArgs []string
	execCommand = func(cmd string, args ...string) commandRunner {
		calledCmd = cmd
		calledArgs = args
		return &mockRunner{stdout: "", stderr: "", exitCode: 0}
	}

	_, _, _, err := RunWithEnv([]string{"docker", "compose", "exec", "php"}, "drush", "status")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if calledCmd != "docker" {
		t.Errorf("called cmd = %q, want %q", calledCmd, "docker")
	}
	wantArgs := []string{"compose", "exec", "php", "drush", "status"}
	if len(calledArgs) != len(wantArgs) {
		t.Fatalf("called args len = %d, want %d", len(calledArgs), len(wantArgs))
	}
	for i, a := range wantArgs {
		if calledArgs[i] != a {
			t.Errorf("calledArgs[%d] = %q, want %q", i, calledArgs[i], a)
		}
	}
}

func TestRun_OverriddenExecCommand(t *testing.T) {
	orig := execCommand
	defer func() { execCommand = orig }()

	var calledCmd string
	var calledArgs []string
	execCommand = func(cmd string, args ...string) commandRunner {
		calledCmd = cmd
		calledArgs = args
		return &mockRunner{stdout: "mocked\n", stderr: "", exitCode: 0}
	}

	stdout, stderr, exitCode, err := Run("git", "status", "--porcelain")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if calledCmd != "git" {
		t.Errorf("called cmd = %q, want %q", calledCmd, "git")
	}
	if len(calledArgs) != 2 || calledArgs[0] != "status" || calledArgs[1] != "--porcelain" {
		t.Errorf("called args = %v, want [status --porcelain]", calledArgs)
	}
	if stdout != "mocked\n" {
		t.Errorf("stdout = %q, want %q", stdout, "mocked\n")
	}
	if stderr != "" {
		t.Errorf("stderr = %q, want empty", stderr)
	}
	if exitCode != 0 {
		t.Errorf("exit code = %d, want 0", exitCode)
	}
}
