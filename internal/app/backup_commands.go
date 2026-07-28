package app

import (
	"encoding/json"
	"fmt"

	"github.com/nireneko/drup/internal/backup"
)

func RunTestBackupCreate(projectPath string) error {
	manifest, err := backup.NewManager(projectPath).Create(projectPath)
	if err != nil {
		return fmt.Errorf("create testing backup: %w", err)
	}
	return printBackupJSON(manifest)
}

// RunTestBackupList prints the backups recorded for a project. The skill's
// stage 9 enumerates backups by id, so the CLI has to expose the same view the
// MCP tool does.
func RunTestBackupList(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: drup test-backup-list <path>")
	}
	manifests, err := backup.NewManager(args[0]).List(args[0])
	if err != nil {
		return fmt.Errorf("list testing backups: %w", err)
	}
	return printBackupJSON(manifests)
}

func RunTestBackupRestore(projectPath, backupID string) error {
	if err := backup.NewManager(projectPath).Restore(projectPath, backupID, true); err != nil {
		return fmt.Errorf("restore testing backup: %w", err)
	}
	return printBackupJSON(map[string]interface{}{"backup_id": backupID, "restored": true})
}

func RunTestBackupDelete(projectPath, backupID string) error {
	if err := backup.NewManager(projectPath).Delete(projectPath, backupID); err != nil {
		return fmt.Errorf("delete testing backup: %w", err)
	}
	return printBackupJSON(map[string]interface{}{"backup_id": backupID, "deleted": true})
}

func printBackupJSON(value interface{}) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal backup result: %w", err)
	}
	fmt.Println(string(data))
	return nil
}
