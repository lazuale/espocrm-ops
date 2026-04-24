package ops

import (
	"compress/gzip"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	v3config "github.com/lazuale/espocrm-ops/internal/v3/config"
	v3runtime "github.com/lazuale/espocrm-ops/internal/v3/runtime"
)

func TestBackupWritesArtifactsAndVerifies(t *testing.T) {
	root := t.TempDir()
	storageDir := filepath.Join(root, "runtime", "prod", "espo")
	if err := os.MkdirAll(filepath.Join(storageDir, "data"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(storageDir, "data", "hello.txt"), []byte("hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	rt := &fakeBackupRuntime{
		dbDump: gzipBytes(t, "create table test(id int);\n"),
	}
	cfg := backupTestConfig(root, storageDir)

	result, err := Backup(context.Background(), cfg, rt, time.Date(2026, 4, 24, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("Backup failed: %v", err)
	}
	if err := rt.requireCalls("validate", "dump_database"); err != nil {
		t.Fatal(err)
	}
	if result.Manifest == "" || result.DBBackup == "" || result.FilesBackup == "" {
		t.Fatalf("unexpected result: %#v", result)
	}

	for _, path := range []string{
		result.Manifest,
		result.DBBackup,
		result.DBBackup + ".sha256",
		result.FilesBackup,
		result.FilesBackup + ".sha256",
	} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected artifact %s: %v", path, err)
		}
	}
	if _, err := VerifyBackup(context.Background(), result.Manifest); err != nil {
		t.Fatalf("VerifyBackup on produced set failed: %v", err)
	}

	matches, err := filepath.Glob(filepath.Join(cfg.BackupRoot, "*", "*.tmp-*"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 0 {
		t.Fatalf("unexpected temporary files after success: %v", matches)
	}
}

func TestBackupFailsClosedWhenSelfVerifyFails(t *testing.T) {
	root := t.TempDir()
	storageDir := filepath.Join(root, "runtime", "prod", "espo")
	if err := os.MkdirAll(storageDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(storageDir, "hello.txt"), []byte("hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	rt := &fakeBackupRuntime{
		dbDump: []byte("not gzip"),
	}
	cfg := backupTestConfig(root, storageDir)

	result, err := Backup(context.Background(), cfg, rt, time.Date(2026, 4, 24, 12, 0, 0, 0, time.UTC))
	assertVerifyErrorKind(t, err, ErrorKindArchive)
	if err := rt.requireCalls("validate", "dump_database"); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{
		result.Manifest,
		result.DBBackup,
		result.DBBackup + ".sha256",
		result.FilesBackup,
		result.FilesBackup + ".sha256",
	} {
		if path == "" {
			continue
		}
		if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
			t.Fatalf("expected cleanup for %s, got %v", path, statErr)
		}
	}
}

func TestBackupFailsWhenFilesArtifactWriteFails(t *testing.T) {
	root := t.TempDir()
	storageDir := filepath.Join(root, "runtime", "prod", "espo")
	if err := os.MkdirAll(filepath.Join(storageDir, "data"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(storageDir, "data", "hello.txt"), []byte("hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	backupRoot := filepath.Join(root, "backups", "prod")
	for _, dir := range []string{
		filepath.Join(backupRoot, "db"),
		filepath.Join(backupRoot, "files"),
		filepath.Join(backupRoot, "manifests"),
	} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Chmod(filepath.Join(backupRoot, "files"), 0o555); err != nil {
		t.Fatal(err)
	}

	rt := &fakeBackupRuntime{
		dbDump: gzipBytes(t, "create table test(id int);\n"),
	}
	cfg := backupTestConfig(root, storageDir)

	result, err := Backup(context.Background(), cfg, rt, time.Date(2026, 4, 24, 12, 0, 0, 0, time.UTC))
	if err == nil {
		t.Fatal("expected backup failure")
	}
	verifyErr, ok := err.(*VerifyError)
	if !ok {
		t.Fatalf("expected VerifyError, got %T", err)
	}
	if verifyErr.Kind != ErrorKindIO && verifyErr.Kind != ErrorKindArchive {
		t.Fatalf("unexpected error kind: %s", verifyErr.Kind)
	}
	for _, path := range []string{
		result.Manifest,
		result.DBBackup,
		result.DBBackup + ".sha256",
		result.FilesBackup,
		result.FilesBackup + ".sha256",
	} {
		if path == "" {
			continue
		}
		if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
			t.Fatalf("expected cleanup for %s, got %v", path, statErr)
		}
	}
}

func TestBackupFailsWhenStorageDirIsBroken(t *testing.T) {
	root := t.TempDir()
	storagePath := filepath.Join(root, "runtime", "prod", "espo")
	if err := os.MkdirAll(filepath.Dir(storagePath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(storagePath, []byte("not a directory"), 0o644); err != nil {
		t.Fatal(err)
	}

	rt := &fakeBackupRuntime{
		dbDump: gzipBytes(t, "create table test(id int);\n"),
	}
	cfg := backupTestConfig(root, storagePath)

	result, err := Backup(context.Background(), cfg, rt, time.Date(2026, 4, 24, 12, 0, 0, 0, time.UTC))
	if err == nil {
		t.Fatal("expected backup failure")
	}
	assertVerifyErrorKind(t, err, ErrorKindArchive)
	for _, path := range []string{
		result.Manifest,
		result.DBBackup,
		result.DBBackup + ".sha256",
		result.FilesBackup,
		result.FilesBackup + ".sha256",
	} {
		if path == "" {
			continue
		}
		if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
			t.Fatalf("expected cleanup for %s, got %v", path, statErr)
		}
	}
}

func backupTestConfig(root, storageDir string) v3config.BackupConfig {
	return v3config.BackupConfig{
		Scope:       "prod",
		ProjectDir:  root,
		ComposeFile: filepath.Join(root, "compose.yaml"),
		EnvFile:     filepath.Join(root, ".env.prod"),
		BackupRoot:  filepath.Join(root, "backups", "prod"),
		StorageDir:  storageDir,
		DBService:   "db",
		DBUser:      "espocrm",
		DBPassword:  "db-secret",
		DBName:      "espocrm",
	}
}

type fakeBackupRuntime struct {
	dbDump      []byte
	validateErr error
	dumpErr     error
	calls       []string
	lastTarget  v3runtime.Target
}

func (f *fakeBackupRuntime) Validate(_ context.Context, target v3runtime.Target) error {
	f.calls = append(f.calls, "validate")
	f.lastTarget = target
	return f.validateErr
}

func (f *fakeBackupRuntime) DumpDatabase(_ context.Context, target v3runtime.Target, destPath string) error {
	f.calls = append(f.calls, "dump_database")
	f.lastTarget = target
	if f.dumpErr != nil {
		return f.dumpErr
	}
	return os.WriteFile(destPath, append([]byte(nil), f.dbDump...), 0o644)
}

func (f *fakeBackupRuntime) requireCalls(want ...string) error {
	if strings.Join(f.calls, ",") != strings.Join(want, ",") {
		return errf("unexpected call order: got %v want %v", f.calls, want)
	}
	return nil
}

func gzipBytes(t *testing.T, body string) []byte {
	t.Helper()

	path := filepath.Join(t.TempDir(), "db.sql.gz")
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	writer := gzip.NewWriter(file)
	if _, err := writer.Write([]byte(body)); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

type backupTestError string

func (e backupTestError) Error() string { return string(e) }

func errf(format string, args ...any) error {
	return backupTestError(strings.TrimSpace(fmt.Sprintf(format, args...)))
}
