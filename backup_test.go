package main

import (
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
