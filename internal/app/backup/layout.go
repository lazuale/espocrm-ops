package backup

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	domainbackup "github.com/lazuale/espocrm-ops/internal/domain/backup"
)

type allocatedBackupSet struct {
	Set       domainbackup.BackupSet
	CreatedAt time.Time
}

func allocateBackupSet(root, prefix string, createdAt time.Time) (allocatedBackupSet, error) {
	if createdAt.IsZero() {
		return allocatedBackupSet{}, fmt.Errorf("created_at is required")
	}

	for current := createdAt.UTC(); ; current = current.Add(time.Second) {
		stamp := current.Format("2006-01-02_15-04-05")
		set := domainbackup.BuildBackupSet(root, prefix, stamp)
		if err := ensureBackupSetDirs(set); err != nil {
			return allocatedBackupSet{}, err
		}
		if !backupSetExists(set) {
			return allocatedBackupSet{
				Set:       set,
				CreatedAt: current,
			}, nil
		}
	}
}

func ensureBackupSetDirs(set domainbackup.BackupSet) error {
	for _, dir := range []string{
		filepath.Dir(set.DBBackup.Path),
		filepath.Dir(set.FilesBackup.Path),
		filepath.Dir(set.ManifestTXT.Path),
		filepath.Dir(set.ManifestJSON.Path),
	} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("ensure backup dir %s: %w", dir, err)
		}
	}
	return nil
}

func backupSetExists(set domainbackup.BackupSet) bool {
	for _, path := range []string{
		set.DBBackup.Path,
		set.FilesBackup.Path,
		set.ManifestTXT.Path,
		set.ManifestJSON.Path,
	} {
		if _, err := os.Stat(path); err == nil {
			return true
		}
	}
	return false
}
