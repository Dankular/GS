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
	for _, required := range []string{`createdb -U "$database_user"`, `dropdb -U "$database_user"`, `psql -U "$database_user"`} {
		if !strings.Contains(string(restore), required) {
			t.Errorf("restore script must use the deployment database role for temporary database lifecycle: %q", required)
		}
	}
}

func TestBackupScheduleRequiresConfiguredEncryptedEnvironment(t *testing.T) {
	service, err := os.ReadFile(filepath.Join("..", "deploy", "backup", "gameservice-postgres-backup.service"))
	if err != nil {
		t.Fatal(err)
	}
	unit := string(service)
	for _, required := range []string{
		"EnvironmentFile=/etc/gameservice/backup.env",
		"Environment=GAMESERVICE_DIR=/opt/gameservice",
		"ExecStart=/opt/gameservice/deploy/backup/backup-postgres.sh",
		"ProtectSystem=full",
		"ReadWritePaths=/var/backups/gameservice /run/docker.sock",
	} {
		if !strings.Contains(unit, required) {
			t.Errorf("backup service missing %q", required)
		}
	}
	timer, err := os.ReadFile(filepath.Join("..", "deploy", "backup", "gameservice-postgres-backup.timer"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(timer), "OnCalendar=*-*-* 00,06,12,18:00:00 UTC") || !strings.Contains(string(timer), "Persistent=true") {
		t.Fatal("backup timer must be persistent and run on a six-hour UTC schedule")
	}
}
