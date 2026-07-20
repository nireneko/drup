package exec

import (
	"bytes"
	"os/exec"
)

// commandRunner abstracts command execution for testability.
type commandRunner interface {
	Output() (stdout, stderr string, exitCode int, err error)
}

// execCommand creates a commandRunner. Package-level var for test overrides.
var execCommand = func(cmd string, args ...string) commandRunner {
	return &realCmd{cmd: exec.Command(cmd, args...)}
}

// realCmd wraps exec.Cmd to implement commandRunner.
type realCmd struct {
	cmd *exec.Cmd
}

func (r *realCmd) Output() (stdout, stderr string, exitCode int, err error) {
	var stdoutBuf, stderrBuf bytes.Buffer
	r.cmd.Stdout = &stdoutBuf
	r.cmd.Stderr = &stderrBuf

	err = r.cmd.Run()
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
func Run(cmd string, args ...string) (stdout, stderr string, exitCode int, err error) {
	return execCommand(cmd, args...).Output()
}
