package tests

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBackupScriptsSupportMandatoryAgeEncryptionAndRestore(t *testing.T) {
	backup, err := os.ReadFile(filepath.Join("..", "deploy", "backup", "backup-postgres.sh"))
	if err != nil {
		t.Fatal(err)
	}
	restore, err := os.ReadFile(filepath.Join("..", "deploy", "backup", "verify-restore.sh"))
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{"GAMESERVICE_BACKUP_REQUIRE_ENCRYPTION", "GAMESERVICE_BACKUP_AGE_RECIPIENT", "GAMESERVICE_BACKUP_DATABASE_USER", "age -r", "GAMESERVICE_BACKUP_S3_URI", "sha256sum"} {
		if !strings.Contains(string(backup), required) {
			t.Errorf("backup script missing %q", required)
		}
	}
	for _, required := range []string{"GAMESERVICE_BACKUP_AGE_IDENTITY", "age -d", "pg_restore"} {
		if !strings.Contains(string(restore), required) {
			t.Errorf("restore script missing %q", required)
		}
	}
	for _, required := range []string{`createdb -U "$database_user"`, `dropdb -U "$database_user"`} {
		if !strings.Contains(string(restore), required) {
			t.Errorf("restore script must use the deployment database role for temporary database lifecycle: %q", required)
		}
	}
}
