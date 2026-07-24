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
