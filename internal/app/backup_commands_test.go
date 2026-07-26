package app

import "testing"

func TestRunBackupCommandsRequireArguments(t *testing.T) {
	for _, args := range [][]string{
		{"test-backup-create"},
		{"test-backup-restore", "/tmp", "id"},
		{"test-backup-delete", "/tmp"},
	} {
		if err := Run(args); err == nil {
			t.Errorf("Run(%v) succeeded; expected usage error", args)
		}
	}
}
