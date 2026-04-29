package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

func Backup(cfg Config) error {
	if err := requireDir(cfg.BackupRoot, "BACKUP_ROOT"); err != nil {
		return err
	}
	if err := requireDir(cfg.EspoStorageDir, "ESPO_STORAGE_DIR"); err != nil {
		return err
	}

	now := time.Now().UTC()
	stamp := now.Format(timestampFormat)
	tempDir := filepath.Join(cfg.BackupRoot, ".tmp-"+stamp)
	finalDir := filepath.Join(cfg.BackupRoot, stamp)

	if _, err := os.Stat(finalDir); err == nil {
		return fmt.Errorf("backup directory already exists: %s", finalDir)
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("stat final backup dir: %w", err)
	}
	if err := os.Mkdir(tempDir, 0700); err != nil {
		return fmt.Errorf("create temp backup dir: %w", err)
	}
	cleanupTemp := true
	defer func() {
		if cleanupTemp {
			_ = safeRemoveTempDir(tempDir, cfg.BackupRoot)
		}
	}()

	dbPath := filepath.Join(tempDir, dbFileName)
	dbFile, err := os.Create(dbPath)
	if err != nil {
		return fmt.Errorf("create db dump: %w", err)
	}
	if err := dumpDatabase(cfg, dbFile); err != nil {
		dbFile.Close()
		return err
	}
	if err := dbFile.Close(); err != nil {
		return fmt.Errorf("close db dump: %w", err)
	}

	filesPath := filepath.Join(tempDir, filesFileName)
	if err := createFilesArchive(cfg.EspoStorageDir, filesPath); err != nil {
		return err
	}

	dbHash, dbSize, err := fileSHA256(dbPath)
	if err != nil {
		return err
	}
	filesHash, filesSize, err := fileSHA256(filesPath)
	if err != nil {
		return err
	}

	manifest := newManifest(now, checksumFile{Hash: dbHash, Size: dbSize}, checksumFile{Hash: filesHash, Size: filesSize})
	if err := writeManifest(filepath.Join(tempDir, manifestFileName), manifest); err != nil {
		return err
	}
	if err := writeSHA256SUMS(filepath.Join(tempDir, sumsFileName), dbHash, filesHash); err != nil {
		return err
	}
	if err := ValidateBackup(tempDir); err != nil {
		return fmt.Errorf("created backup failed validation: %w", err)
	}
	if err := os.Rename(tempDir, finalDir); err != nil {
		return fmt.Errorf("move backup into place: %w", err)
	}
	cleanupTemp = false
	fmt.Printf("backup created: %s\n", finalDir)
	return nil
}
