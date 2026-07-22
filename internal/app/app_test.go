package app

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"
)

func TestRun_Help(t *testing.T) {
	err := Run([]string{"help"})
	if err != nil {
		t.Errorf("help should not error, got: %v", err)
	}
}

func TestRun_NoArgs(t *testing.T) {
	err := Run([]string{})
	if err != nil {
		t.Errorf("no args should not error, got: %v", err)
	}
}

func TestRun_UnknownCommand(t *testing.T) {
	err := Run([]string{"nonexistent"})
	if err == nil {
		t.Error("unknown command should return error")
	}
}

func TestRun_Version(t *testing.T) {
	err := Run([]string{"version"})
	if err != nil {
		t.Errorf("version should not error, got: %v", err)
	}
}

func TestRun_ScanMissingPath(t *testing.T) {
	err := Run([]string{"scan"})
	if err == nil {
		t.Error("scan without path should return error")
	}
}

func TestRun_ContribMissingModule(t *testing.T) {
	err := Run([]string{"contrib"})
	if err == nil {
		t.Error("contrib without module should return error")
	}
}

func TestRun_IssueMissingArg(t *testing.T) {
	err := Run([]string{"issue"})
	if err == nil {
		t.Error("issue without arg should return error")
	}
}

func TestRun_ReportMissingPath(t *testing.T) {
	err := Run([]string{"report"})
	if err == nil {
		t.Error("report without path should return error")
	}
}

func TestRun_UpgradeCoreMissingArg(t *testing.T) {
	err := Run([]string{"upgrade-core"})
	if err == nil {
		t.Error("upgrade-core without target version should return error")
	}
}

func TestRun_UpgradeCoreInUsage(t *testing.T) {
	// Capture stdout to verify usage includes upgrade-core.
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	Run([]string{"help"})

	w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	io.Copy(&buf, r)
	output := buf.String()

	if !strings.Contains(output, "upgrade-core") {
		t.Errorf("usage output should mention 'upgrade-core', got: %s", output)
	}
}
