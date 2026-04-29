package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

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

	stamp := time.Now().UTC().Format(timestampFormat)
	storageParent := filepath.Dir(cfg.EspoStorageDir)
	storageBase := filepath.Base(cfg.EspoStorageDir)
	tempStorage := filepath.Join(storageParent, ".tmp-restore-"+stamp)
	oldStorage := filepath.Join(storageParent, storageBase+".before-restore-"+stamp)

	if _, err := os.Stat(oldStorage); err == nil {
		return fmt.Errorf("restore storage backup already exists: %s", oldStorage)
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("stat old storage target: %w", err)
	}
	if err := os.Mkdir(tempStorage, 0700); err != nil {
		return fmt.Errorf("create temp storage dir: %w", err)
	}
	extracted := false
	defer func() {
		if !extracted {
			_ = safeRemoveTempDir(tempStorage, storageParent)
		}
	}()

	if err := extractFilesArchive(filepath.Join(backupDir, filesFileName), tempStorage); err != nil {
		return err
	}
	extracted = true

	if err := resetDatabase(cfg); err != nil {
		return err
	}
	dbDump, err := os.Open(filepath.Join(backupDir, dbFileName))
	if err != nil {
		return fmt.Errorf("open db dump: %w", err)
	}
	defer dbDump.Close()
	if err := restoreDatabase(cfg, dbDump); err != nil {
		return err
	}

	if err := os.Rename(cfg.EspoStorageDir, oldStorage); err != nil {
		return fmt.Errorf("move current storage to restore backup: %w", err)
	}
	if err := os.Rename(tempStorage, cfg.EspoStorageDir); err != nil {
		return fmt.Errorf("move restored storage into place: %w", err)
	}

	fmt.Printf("restore complete; previous storage kept at %s\n", oldStorage)
	return nil
}
