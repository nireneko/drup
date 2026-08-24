package exec

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os/exec"
	"sync"
	"syscall"
)

// running tracks live child processes so they can be terminated with the
// process that spawned them. Without this, killing drup (a timeout, Ctrl-C, an
// agent giving up) leaves drush and composer running inside the container.
var (
	runningMu sync.Mutex
	running   = map[int]*exec.Cmd{}
)

func trackProcess(cmd *exec.Cmd) {
	if cmd.Process == nil {
		return
	}
	runningMu.Lock()
	running[cmd.Process.Pid] = cmd
	runningMu.Unlock()
}

func untrackProcess(cmd *exec.Cmd) {
	if cmd.Process == nil {
		return
	}
	runningMu.Lock()
	delete(running, cmd.Process.Pid)
	runningMu.Unlock()
}

// KillChildren terminates every running child process group. Call it before
// exiting on a signal so long analyses do not outlive the command.
func KillChildren() {
	runningMu.Lock()
	cmds := make([]*exec.Cmd, 0, len(running))
	for _, cmd := range running {
		cmds = append(cmds, cmd)
	}
	running = map[int]*exec.Cmd{}
	runningMu.Unlock()

	for _, cmd := range cmds {
		if cmd.Process == nil {
			continue
		}
		// Negative pid signals the whole process group.
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM)
	}
}

// commandRunner abstracts command execution for testability.
type commandRunner interface {
	Output() (stdout, stderr string, exitCode int, err error)
}

// execCommandInDir builds a runner whose working directory is dir. Container
// CLIs resolve the project from the current directory, so a command issued
// from anywhere else fails or, worse, acts on the wrong project.
var execCommandInDir = func(dir, cmd string, args ...string) commandRunner {
	c := exec.Command(cmd, args...)
	c.Dir = dir
	c.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	return &realCmd{cmd: c}
}

// execCommand creates a commandRunner. Package-level var for test overrides.
var execCommand = func(cmd string, args ...string) commandRunner {
	c := exec.Command(cmd, args...)
	// Own process group: lets the whole tree be signalled at once.
	c.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	return &realCmd{cmd: c}
}

// realCmd wraps exec.Cmd to implement commandRunner.
type realCmd struct {
	cmd *exec.Cmd
}

func (r *realCmd) Output() (stdout, stderr string, exitCode int, err error) {
	var stdoutBuf, stderrBuf bytes.Buffer
	r.cmd.Stdout = &stdoutBuf
	r.cmd.Stderr = &stderrBuf

	if err = r.cmd.Start(); err != nil {
		return "", "", -1, err
	}
	trackProcess(r.cmd)
	err = r.cmd.Wait()
	untrackProcess(r.cmd)
	stdout = stdoutBuf.String()
	stderr = stderrBuf.String()

	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return stdout, stderr, exitErr.ExitCode(), nil
		}
		return stdout, stderr, -1, err
	}

	return stdout, stderr, 0, nil
}

// Run executes cmd with args and returns stdout, stderr, exit code, and error.
// A non-zero exit code is NOT an error — it's returned in exitCode.
// An error is only returned if the command cannot be started (e.g., not found).
// Package-level var for test overrides.
var Run = func(cmd string, args ...string) (stdout, stderr string, exitCode int, err error) {
	return execCommand(cmd, args...).Output()
}

// RunWithEnv prepends prefix tokens to cmd and runs it from dir.
// Example: RunWithEnv("/srv/site", []string{"ddev", "exec"}, "composer", "require", "pkg")
// executes: ddev exec composer require pkg, with /srv/site as the working
// directory. The directory is explicit because container CLIs resolve the
// project from it, and drup is usually driven by an MCP server whose own
// working directory has nothing to do with the project. An empty dir inherits
// the process working directory.
// Package-level var for test overrides.
var RunWithEnv = func(dir string, prefix []string, cmd string, args ...string) (stdout, stderr string, exitCode int, err error) {
	fullArgs := make([]string, 0, len(prefix)+1+len(args))
	fullArgs = append(fullArgs, prefix...)
	fullArgs = append(fullArgs, cmd)
	fullArgs = append(fullArgs, args...)
	name, rest := fullArgs[0], fullArgs[1:]
	if dir == "" {
		return execCommand(name, rest...).Output()
	}
	return execCommandInDir(dir, name, rest...).Output()
}

// RunWithEnvInput executes a command with stdin without invoking a shell.
var RunWithEnvInput = func(prefix []string, input io.Reader, cmd string, args ...string) (stdout, stderr string, exitCode int, err error) {
	fullArgs := append(append([]string{}, prefix...), cmd)
	fullArgs = append(fullArgs, args...)
	r := &realCmd{cmd: exec.Command(fullArgs[0], fullArgs[1:]...)}
	r.cmd.Stdin = input
	return r.Output()
}

// --- Bounded (context-aware) siblings ---
//
// A stuck drush or composer call previously blocked its caller forever —
// there was no way to bound Run/RunWithEnv/RunWithEnvInput by a deadline.
// RunCtx, RunWithEnvCtx, and RunWithEnvInputCtx race the underlying call
// against ctx: if ctx is done first, whatever process the call just spawned
// is identified via the existing process-group tracking (trackProcess /
// untrackProcess / the running map) and sent SIGTERM, exactly like
// KillChildren does on shutdown — but scoped to only the process this call
// started, so a deadline on one call never touches an unrelated concurrent
// one. The function then returns a timeout error immediately instead of
// waiting for the (now-signalled) process to actually exit.
//
// Each *Ctx function calls through the corresponding package-level var
// (Run, RunWithEnv, RunWithEnvInput) rather than reimplementing execution,
// so existing test overrides of those vars keep working unchanged for
// callers that switch to the Ctx sibling.

// snapshotRunningPIDs returns the pids currently tracked as running child
// processes, used as a "before" baseline to identify which new process a
// bounded call spawns.
func snapshotRunningPIDs() map[int]bool {
	runningMu.Lock()
	defer runningMu.Unlock()
	pids := make(map[int]bool, len(running))
	for pid := range running {
		pids[pid] = true
	}
	return pids
}

// killNewProcessGroups sends SIGTERM to the process group of every tracked
// pid that was not present in before.
func killNewProcessGroups(before map[int]bool) {
	runningMu.Lock()
	var toKill []int
	for pid := range running {
		if !before[pid] {
			toKill = append(toKill, pid)
		}
	}
	runningMu.Unlock()
	for _, pid := range toKill {
		// Negative pid signals the whole process group, mirroring KillChildren.
		_ = syscall.Kill(-pid, syscall.SIGTERM)
	}
}

// boundedResult carries a (stdout, stderr, exitCode, err) tuple across the
// goroutine boundary in runBounded.
type boundedResult struct {
	stdout, stderr string
	exitCode       int
	err            error
}

// runBounded races fn — a blocking call into Run/RunWithEnv/RunWithEnvInput
// — against ctx. If fn finishes first, its result is returned unchanged. If
// ctx is done first, the process fn just spawned is terminated and a
// timeout error is returned without waiting for fn to actually return.
func runBounded(ctx context.Context, fn func() (stdout, stderr string, exitCode int, err error)) (string, string, int, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	before := snapshotRunningPIDs()
	done := make(chan boundedResult, 1)
	go func() {
		so, se, ec, e := fn()
		done <- boundedResult{so, se, ec, e}
	}()

	select {
	case r := <-done:
		return r.stdout, r.stderr, r.exitCode, r.err
	case <-ctx.Done():
		killNewProcessGroups(before)
		return "", "", -1, fmt.Errorf("command timed out: %w", ctx.Err())
	}
}

// RunCtx behaves like Run but bounds execution by ctx.
var RunCtx = func(ctx context.Context, cmd string, args ...string) (stdout, stderr string, exitCode int, err error) {
	return runBounded(ctx, func() (string, string, int, error) {
		return Run(cmd, args...)
	})
}

// RunWithEnvCtx behaves like RunWithEnv but bounds execution by ctx.
var RunWithEnvCtx = func(ctx context.Context, dir string, prefix []string, cmd string, args ...string) (stdout, stderr string, exitCode int, err error) {
	return runBounded(ctx, func() (string, string, int, error) {
		return RunWithEnv(dir, prefix, cmd, args...)
	})
}

// RunWithEnvInputCtx behaves like RunWithEnvInput but bounds execution by ctx.
var RunWithEnvInputCtx = func(ctx context.Context, prefix []string, input io.Reader, cmd string, args ...string) (stdout, stderr string, exitCode int, err error) {
	return runBounded(ctx, func() (string, string, int, error) {
		return RunWithEnvInput(prefix, input, cmd, args...)
	})
}
