package main

import (
	"fmt"
	"os"
	"path/filepath"
)

var resetDatabaseForRestore = resetDatabase
var restoreDatabaseForRestore = restoreDatabase

func Restore(cfg Config, backupDir string) error {
	if backupDir == "" {
		return fmt.Errorf("backup directory is required")
	}
	if err := requireDir(cfg.EspoStorageDir, "ESPO_STORAGE_DIR"); err != nil {
		return err
	}
	if err := ValidateBackup(backupDir); err != nil {
		return err
	}

	storageParent := filepath.Dir(cfg.EspoStorageDir)
	stamp := nowUTC().Format(timestampFormat)
	tempStorage, err := os.MkdirTemp(storageParent, ".tmp-restore-"+stamp+"-*")
	if err != nil {
		return fmt.Errorf("create temp storage dir: %w", err)
	}
	cleanupTemp := true
	defer func() {
		if cleanupTemp {
			_ = safeRemoveTempDir(tempStorage, storageParent)
		}
	}()

	if err := extractFilesArchive(filepath.Join(backupDir, filesFileName), tempStorage); err != nil {
		return err
	}

	oldStorage, err := beforeRestorePath(cfg.EspoStorageDir, stamp)
	if err != nil {
		return err
	}

	// The database is reset before the storage swap so EspoCRM never sees new
	// files with an old schema. If import fails after reset, storage remains
	// untouched and the operator must repair the database from the backup.
	if err := resetDatabaseForRestore(cfg); err != nil {
		return err
	}
	dbDump, err := os.Open(filepath.Join(backupDir, dbFileName))
	if err != nil {
		return fmt.Errorf("open db dump: %w", err)
	}
	defer dbDump.Close()
	if err := restoreDatabaseForRestore(cfg, dbDump); err != nil {
		return fmt.Errorf("database restore failed after reset; storage was not swapped and manual database recovery is required: %w", err)
	}

	if err := swapStorage(cfg.EspoStorageDir, tempStorage, oldStorage, os.Rename); err != nil {
		return err
	}
	cleanupTemp = false

	fmt.Printf("restore complete; previous storage kept at %s\n", oldStorage)
	return nil
}

func beforeRestorePath(storageDir string, stamp string) (string, error) {
	base := filepath.Join(filepath.Dir(storageDir), filepath.Base(storageDir)+".before-restore-"+stamp)
	candidate := base
	for i := 0; i < 100; i++ {
		if i > 0 {
			candidate = fmt.Sprintf("%s.%d", base, i)
		}
		if _, err := os.Stat(candidate); os.IsNotExist(err) {
			return candidate, nil
		} else if err != nil {
			return "", fmt.Errorf("stat restore storage backup target: %w", err)
		}
	}
	return "", fmt.Errorf("could not choose unique restore storage backup path starting at %s", base)
}

func swapStorage(liveStorage string, tempStorage string, oldStorage string, rename func(string, string) error) error {
	if err := rename(liveStorage, oldStorage); err != nil {
		return fmt.Errorf("move current storage to restore backup: %w", err)
	}
	if err := rename(tempStorage, liveStorage); err != nil {
		rollbackErr := rename(oldStorage, liveStorage)
		if rollbackErr != nil {
			return fmt.Errorf("fatal storage restore failure: moved live storage to %s, then failed to move restored storage %s to %s: %v; rollback also failed: %v; manually move %s back to %s before retrying", oldStorage, tempStorage, liveStorage, err, rollbackErr, oldStorage, liveStorage)
		}
		return fmt.Errorf("move restored storage into place failed and rollback restored live storage from %s to %s: %w", oldStorage, liveStorage, err)
	}
	return nil
}
