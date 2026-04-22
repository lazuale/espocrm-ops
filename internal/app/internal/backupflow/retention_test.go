package backupflow

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	domainbackup "github.com/lazuale/espocrm-ops/internal/domain/backup"
	backupstoreadapter "github.com/lazuale/espocrm-ops/internal/platform/backupstoreadapter"
)

func TestCleanupRetention_RemovesWholeBackupSet(t *testing.T) {
	root := t.TempDir()
	oldSet := domainbackup.BuildBackupSet(root, "espocrm-dev", "2026-04-01_00-00-00")
	newSet := domainbackup.BuildBackupSet(root, "espocrm-dev", "2026-04-20_00-00-00")

	for _, path := range []string{
		oldSet.ManifestJSON.Path,
		oldSet.DBBackup.Path + ".sha256",
		newSet.ManifestJSON.Path,
		newSet.FilesBackup.Path,
	} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	service := Service{store: backupstoreadapter.BackupStore{}}
	if err := service.cleanupRetention(root, 7, time.Date(2026, 4, 22, 0, 0, 0, 0, time.UTC)); err != nil {
		t.Fatalf("cleanupRetention failed: %v", err)
	}

	for _, removed := range []string{
		oldSet.ManifestJSON.Path,
		oldSet.DBBackup.Path + ".sha256",
	} {
		if _, err := os.Stat(removed); !os.IsNotExist(err) {
			t.Fatalf("expected old backup-set path removed: %s err=%v", removed, err)
		}
	}

	for _, kept := range []string{
		newSet.ManifestJSON.Path,
		newSet.FilesBackup.Path,
	} {
		if _, err := os.Stat(kept); err != nil {
			t.Fatalf("expected recent backup-set path kept: %s err=%v", kept, err)
		}
	}
}
