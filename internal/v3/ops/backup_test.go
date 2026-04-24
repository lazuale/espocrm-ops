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

func TestBackupValidFullSet(t *testing.T) {
	root := t.TempDir()
	storageDir := filepath.Join(root, "runtime", "prod", "espo")
	if err := os.MkdirAll(filepath.Join(storageDir, "data"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(storageDir, "data", "hello.txt"), []byte("hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	rt := &fakeBackupRuntime{
		running: []string{"db", "espocrm", "espocrm-daemon"},
		dbDump:  gzipBytes(t, "create table test(id int);\n"),
	}
	cfg := v3config.BackupConfig{
		Scope:       "prod",
		ProjectDir:  root,
		ComposeFile: filepath.Join(root, "compose.yaml"),
		EnvFile:     filepath.Join(root, ".env.prod"),
		BackupRoot:  filepath.Join(root, "backups", "prod"),
		NamePrefix:  "espocrm-prod",
		StorageDir:  storageDir,
		DBService:   "db",
		DBUser:      "espocrm",
		DBPassword:  "db-secret",
		DBName:      "espocrm",
	}

	result, err := Backup(context.Background(), cfg, rt, time.Date(2026, 4, 24, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("Backup failed: %v", err)
	}
	if err := rt.requireCalls("validate", "running_services", "stop_services", "dump_database", "start_services"); err != nil {
		t.Fatal(err)
	}
	if result.Manifest == "" || result.DBBackup == "" || result.FilesBackup == "" {
		t.Fatalf("unexpected result: %#v", result)
	}
	if _, err := os.Stat(result.Manifest); err != nil {
		t.Fatalf("manifest missing: %v", err)
	}
	if _, err := VerifyBackup(context.Background(), result.Manifest); err != nil {
		t.Fatalf("VerifyBackup on produced set failed: %v", err)
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
		running: []string{"db", "espocrm"},
		dbDump:  []byte("not gzip"),
	}
	cfg := v3config.BackupConfig{
		Scope:       "prod",
		ProjectDir:  root,
		ComposeFile: filepath.Join(root, "compose.yaml"),
		EnvFile:     filepath.Join(root, ".env.prod"),
		BackupRoot:  filepath.Join(root, "backups", "prod"),
		NamePrefix:  "espocrm-prod",
		StorageDir:  storageDir,
		DBService:   "db",
		DBUser:      "espocrm",
		DBPassword:  "db-secret",
		DBName:      "espocrm",
	}

	result, err := Backup(context.Background(), cfg, rt, time.Date(2026, 4, 24, 12, 0, 0, 0, time.UTC))
	assertVerifyErrorKind(t, err, ErrorKindArchive)
	if err := rt.requireCalls("validate", "running_services", "stop_services", "dump_database", "start_services"); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{result.Manifest, result.DBBackup, result.FilesBackup} {
		if path == "" {
			continue
		}
		if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
			t.Fatalf("expected cleanup for %s, got %v", path, statErr)
		}
	}
}

type fakeBackupRuntime struct {
	running      []string
	dbDump       []byte
	validateErr  error
	runningErr   error
	stopErr      error
	startErr     error
	dumpErr      error
	calls        []string
	lastTarget   v3runtime.Target
	lastServices []string
}

func (f *fakeBackupRuntime) Validate(_ context.Context, target v3runtime.Target) error {
	f.calls = append(f.calls, "validate")
	f.lastTarget = target
	return f.validateErr
}

func (f *fakeBackupRuntime) RunningServices(_ context.Context, target v3runtime.Target) ([]string, error) {
	f.calls = append(f.calls, "running_services")
	f.lastTarget = target
	return append([]string(nil), f.running...), f.runningErr
}

func (f *fakeBackupRuntime) StopServices(_ context.Context, target v3runtime.Target, services ...string) error {
	f.calls = append(f.calls, "stop_services")
	f.lastTarget = target
	f.lastServices = append([]string(nil), services...)
	return f.stopErr
}

func (f *fakeBackupRuntime) StartServices(_ context.Context, target v3runtime.Target, services ...string) error {
	f.calls = append(f.calls, "start_services")
	f.lastTarget = target
	f.lastServices = append([]string(nil), services...)
	return f.startErr
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
