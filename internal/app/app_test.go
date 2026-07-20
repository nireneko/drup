package app

import (
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
