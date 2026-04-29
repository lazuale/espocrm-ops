package main

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestStorageSwapSuccessKeepsOldStorage(t *testing.T) {
	dir := t.TempDir()
	live := filepath.Join(dir, "storage")
	temp := filepath.Join(dir, "restore-temp")
	old := filepath.Join(dir, "storage.before-restore-20260429-120000")
	mustWriteFile(t, filepath.Join(live, "old.txt"), "old")
	mustWriteFile(t, filepath.Join(temp, "new.txt"), "new")

	if err := swapStorage(live, temp, old, os.Rename); err != nil {
		t.Fatalf("swapStorage returned error: %v", err)
	}
	if _, err := os.Stat(filepath.Join(old, "old.txt")); err != nil {
		t.Fatalf("old storage not preserved: %v", err)
	}
	if data, err := os.ReadFile(filepath.Join(live, "new.txt")); err != nil || string(data) != "new" {
		t.Fatalf("live storage not replaced correctly, data=%q err=%v", data, err)
	}
}

func TestStorageSwapSecondRenameFailureAttemptsRollback(t *testing.T) {
	calls := [][2]string{}
	errSecond := errors.New("second rename failed")
	err := swapStorage("live", "temp", "old", func(from, to string) error {
		calls = append(calls, [2]string{from, to})
		if len(calls) == 2 {
			return errSecond
		}
		return nil
	})
	if err == nil || !strings.Contains(err.Error(), "rollback restored live storage") {
		t.Fatalf("expected rollback success error, got %v", err)
	}
	want := [][2]string{{"live", "old"}, {"temp", "live"}, {"old", "live"}}
	if len(calls) != len(want) {
		t.Fatalf("expected %d rename calls, got %d: %#v", len(want), len(calls), calls)
	}
	for i := range want {
		if calls[i] != want[i] {
			t.Fatalf("rename call %d = %#v, want %#v", i, calls[i], want[i])
		}
	}
}

func TestStorageSwapRollbackFailureReturnsFatalManualRecoveryError(t *testing.T) {
	err := swapStorage("live", "temp", "old", func(from, to string) error {
		switch {
		case from == "live" && to == "old":
			return nil
		case from == "temp" && to == "live":
			return errors.New("second failed")
		case from == "old" && to == "live":
			return errors.New("rollback failed")
		default:
			return nil
		}
	})
	if err == nil {
		t.Fatal("expected error")
	}
	for _, text := range []string{"fatal storage restore failure", "manually move old back to live", "temp", "old", "live"} {
		if !strings.Contains(err.Error(), text) {
			t.Fatalf("expected error to contain %q, got %v", text, err)
		}
	}
}

func TestBeforeRestoreCollisionUsesSafeUniqueSuffix(t *testing.T) {
	dir := t.TempDir()
	live := filepath.Join(dir, "storage")
	if err := os.Mkdir(live, 0700); err != nil {
		t.Fatal(err)
	}
	base := live + ".before-restore-20260429-120000"
	if err := os.Mkdir(base, 0700); err != nil {
		t.Fatal(err)
	}
	got, err := beforeRestorePath(live, "20260429-120000")
	if err != nil {
		t.Fatal(err)
	}
	if got != base+".1" {
		t.Fatalf("got %s, want %s", got, base+".1")
	}
}

func TestRestoreDoesNotSwapStorageIfDBImportFails(t *testing.T) {
	dir := t.TempDir()
	storage := filepath.Join(dir, "storage")
	mustWriteFile(t, filepath.Join(storage, "live.txt"), "live")
	backupDir := createValidBackupFixture(t)

	oldReset := resetDatabaseForRestore
	oldRestore := restoreDatabaseForRestore
	oldNow := nowUTC
	defer func() {
		resetDatabaseForRestore = oldReset
		restoreDatabaseForRestore = oldRestore
		nowUTC = oldNow
	}()

	resetDatabaseForRestore = func(Config) error { return nil }
	restoreDatabaseForRestore = func(Config, io.Reader) error { return errors.New("import failed") }
	nowUTC = func() time.Time { return time.Date(2026, 4, 29, 12, 0, 0, 0, time.UTC) }

	cfg := Config{EspoStorageDir: storage, DBName: "espocrm", DBUser: "espocrm"}
	err := Restore(cfg, backupDir)
	if err == nil || !strings.Contains(err.Error(), "database restore failed after reset") {
		t.Fatalf("expected explicit DB restore failure, got %v", err)
	}
	if data, err := os.ReadFile(filepath.Join(storage, "live.txt")); err != nil || string(data) != "live" {
		t.Fatalf("storage was swapped after DB import failure, data=%q err=%v", data, err)
	}
	matches, err := filepath.Glob(storage + ".before-restore-*")
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 0 {
		t.Fatalf("storage backup path should not exist after DB import failure: %v", matches)
	}
}

func TestRestoreKeepsTempStorageOnFatalRollbackFailure(t *testing.T) {
	dir := t.TempDir()
	storage := filepath.Join(dir, "storage")
	mustWriteFile(t, filepath.Join(storage, "live.txt"), "live")
	backupDir := createValidBackupFixture(t)

	oldReset := resetDatabaseForRestore
	oldRestore := restoreDatabaseForRestore
	oldRename := renameStorageForRestore
	oldNow := nowUTC
	defer func() {
		resetDatabaseForRestore = oldReset
		restoreDatabaseForRestore = oldRestore
		renameStorageForRestore = oldRename
		nowUTC = oldNow
	}()

	resetDatabaseForRestore = func(Config) error { return nil }
	restoreDatabaseForRestore = func(Config, io.Reader) error { return nil }
	nowUTC = func() time.Time { return time.Date(2026, 4, 29, 12, 0, 0, 0, time.UTC) }
	renameStorageForRestore = func(from, to string) error {
		switch {
		case from == storage && strings.Contains(to, ".before-restore-20260429-120000"):
			return nil
		case strings.Contains(from, ".tmp-restore-20260429-120000-") && to == storage:
			return errors.New("move restored storage failed")
		case strings.Contains(from, ".before-restore-20260429-120000") && to == storage:
			return errors.New("rollback failed")
		default:
			return nil
		}
	}

	err := Restore(Config{EspoStorageDir: storage, DBName: "espocrm", DBUser: "espocrm"}, backupDir)
	if err == nil || !strings.Contains(err.Error(), "fatal storage restore failure") {
		t.Fatalf("expected fatal storage rollback error, got %v", err)
	}
	matches, err := filepath.Glob(filepath.Join(dir, ".tmp-restore-20260429-120000-*"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 1 {
		t.Fatalf("expected temp storage to remain, got %v", matches)
	}
	if _, err := os.Stat(filepath.Join(matches[0], "storage", "file.txt")); err != nil {
		t.Fatalf("expected restored temp storage contents to remain: %v", err)
	}
}

func mustWriteFile(t *testing.T, path string, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
}
