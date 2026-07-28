package exec

import (
	"bytes"
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
