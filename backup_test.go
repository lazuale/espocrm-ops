package main

import (
	"compress/gzip"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestBackupFinalDirCollisionFailsClearly(t *testing.T) {
	dir := t.TempDir()
	backupRoot := filepath.Join(dir, "backups")
	storage := filepath.Join(dir, "storage")
	if err := os.Mkdir(backupRoot, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(storage, 0700); err != nil {
		t.Fatal(err)
	}

	oldNow := nowUTC
	defer func() { nowUTC = oldNow }()
	nowUTC = func() time.Time { return time.Date(2026, 4, 29, 12, 0, 0, 0, time.UTC) }

	finalDir := filepath.Join(backupRoot, "20260429-120000")
	if err := os.Mkdir(finalDir, 0700); err != nil {
		t.Fatal(err)
	}

	err := Backup(Config{BackupRoot: backupRoot, EspoStorageDir: storage})
	if err == nil || !strings.Contains(err.Error(), "backup directory already exists") {
		t.Fatalf("expected final directory collision error, got %v", err)
	}
}

func TestBackupRenameFailureLeavesValidatedTempBackup(t *testing.T) {
	dir := t.TempDir()
	backupRoot := filepath.Join(dir, "backups")
	storage := filepath.Join(dir, "storage")
	if err := os.Mkdir(backupRoot, 0700); err != nil {
		t.Fatal(err)
	}
	mustWriteFile(t, filepath.Join(storage, "file.txt"), "storage")

	oldDump := dumpDatabaseForBackup
	oldRename := renameBackupDir
	oldNow := nowUTC
	defer func() {
		dumpDatabaseForBackup = oldDump
		renameBackupDir = oldRename
		nowUTC = oldNow
	}()

	nowUTC = func() time.Time { return time.Date(2026, 4, 29, 12, 0, 0, 0, time.UTC) }
	dumpDatabaseForBackup = func(_ Config, out io.Writer) error {
		return writeGzipToWriter(t, out)
	}
	renameBackupDir = func(string, string) error {
		return errors.New("rename failed")
	}

	err := Backup(Config{BackupRoot: backupRoot, EspoStorageDir: storage})
	if err == nil || !strings.Contains(err.Error(), "valid backup remains at") {
		t.Fatalf("expected valid backup remains error, got %v", err)
	}
	matches, err := filepath.Glob(filepath.Join(backupRoot, ".tmp-20260429-120000-*"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 1 {
		t.Fatalf("expected temp backup to remain, got %v", matches)
	}
	if err := ValidateBackup(matches[0]); err != nil {
		t.Fatalf("remaining temp backup should be valid: %v", err)
	}
	if _, err := os.Stat(filepath.Join(backupRoot, "20260429-120000")); !os.IsNotExist(err) {
		t.Fatalf("final backup dir should not exist, stat err=%v", err)
	}
}

func writeGzipToWriter(t *testing.T, out io.Writer) error {
	t.Helper()
	gz := gzip.NewWriter(out)
	if _, err := gz.Write([]byte("CREATE TABLE test (id int);\n")); err != nil {
		gz.Close()
		return err
	}
	return gz.Close()
}
