package ops

import (
	"compress/gzip"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/lazuale/espocrm-ops/internal/config"
	"github.com/lazuale/espocrm-ops/internal/manifest"
	"github.com/lazuale/espocrm-ops/internal/runtime"
)

func TestBackupCreatesMinimalV1BackupSet(t *testing.T) {
	cfg := testConfig(t)
	rt := &fakeRuntime{dumpSQL: "create table account(id int);\n"}
	now := time.Date(2026, 4, 26, 13, 14, 15, 16, time.UTC)

	result, err := Backup(context.Background(), cfg, rt, now)
	if err != nil {
		t.Fatalf("Backup failed: %v", err)
	}

	wantDir := filepath.Join(cfg.BackupRoot, "prod", now.Format(backupTimestampFormat))
	if result.Manifest != filepath.Join(wantDir, manifest.ManifestName) {
		t.Fatalf("unexpected manifest path: %s", result.Manifest)
	}
	for _, path := range []string{
		filepath.Join(wantDir, manifest.ManifestName),
		filepath.Join(wantDir, manifest.DBFileName),
		filepath.Join(wantDir, manifest.FilesFileName),
	} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected backup artifact %s: %v", path, err)
		}
	}
	matches, err := filepath.Glob(filepath.Join(wantDir, "*.sha256"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 0 {
		t.Fatalf("sidecar files must not be written: %#v", matches)
	}

	loaded, err := manifest.Load(result.Manifest)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Version != manifest.Version || loaded.DB.File != manifest.DBFileName || loaded.Files.File != manifest.FilesFileName {
		t.Fatalf("unexpected manifest: %#v", loaded)
	}
	if strings.Join(rt.calls, ",") != "compose_config,stop_services,dump_database,start_services,service_health" {
		t.Fatalf("unexpected calls: %s", strings.Join(rt.calls, ","))
	}
}

func TestRestoreCreatesSnapshotAndSwitchesStorage(t *testing.T) {
	cfg := testConfig(t)
	rt := &fakeRuntime{dumpSQL: "source db\n"}
	sourceNow := time.Date(2026, 4, 26, 13, 0, 0, 0, time.UTC)

	source, err := Backup(context.Background(), cfg, rt, sourceNow)
	if err != nil {
		t.Fatalf("source Backup failed: %v", err)
	}
	writeStorageFile(t, cfg.StorageDir, "data/file.txt", "target before restore\n")
	rt.calls = nil
	rt.dumpSQL = "snapshot db\n"

	restoreNow := sourceNow.Add(time.Second)
	result, err := Restore(context.Background(), cfg, source.Manifest, rt, restoreNow)
	if err != nil {
		t.Fatalf("Restore failed: %v", err)
	}
	if result.SnapshotManifest == "" {
		t.Fatal("restore must create target snapshot")
	}
	body, err := os.ReadFile(filepath.Join(cfg.StorageDir, "data", "file.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "initial storage\n" {
		t.Fatalf("storage was not restored from source backup: %q", body)
	}
	if rt.restoredSQL != "source db\n" {
		t.Fatalf("database restore used wrong source: %q", rt.restoredSQL)
	}

	wantCalls := "compose_config,stop_services,dump_database,start_services,service_health,stop_services,reset_database,restore_database,start_services,service_health,db_ping"
	if strings.Join(rt.calls, ",") != wantCalls {
		t.Fatalf("unexpected restore calls:\nwant %s\ngot  %s", wantCalls, strings.Join(rt.calls, ","))
	}
}

func TestRestoreRejectsDifferentScopeBeforeSnapshot(t *testing.T) {
	cfg := testConfig(t)
	rt := &fakeRuntime{dumpSQL: "source db\n"}
	source, err := Backup(context.Background(), cfg, rt, time.Date(2026, 4, 26, 13, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("source Backup failed: %v", err)
	}

	loaded, err := manifest.Load(source.Manifest)
	if err != nil {
		t.Fatal(err)
	}
	loaded.Scope = "dev"
	if err := manifest.Write(source.Manifest, loaded); err != nil {
		t.Fatal(err)
	}

	rt.calls = nil
	_, err = Restore(context.Background(), cfg, source.Manifest, rt, time.Date(2026, 4, 26, 13, 0, 1, 0, time.UTC))
	if err == nil || !strings.Contains(err.Error(), "manifest scope") {
		t.Fatalf("expected scope error, got %v", err)
	}
	if len(rt.calls) != 0 {
		t.Fatalf("restore should fail before snapshot or mutation, got calls %#v", rt.calls)
	}
}

type fakeRuntime struct {
	calls       []string
	dumpSQL     string
	restoredSQL string
}

func (f *fakeRuntime) ComposeConfig(context.Context, runtime.Target) error {
	f.calls = append(f.calls, "compose_config")
	return nil
}

func (f *fakeRuntime) StopServices(context.Context, runtime.Target, []string) error {
	f.calls = append(f.calls, "stop_services")
	return nil
}

func (f *fakeRuntime) StartServices(context.Context, runtime.Target, []string) error {
	f.calls = append(f.calls, "start_services")
	return nil
}

func (f *fakeRuntime) DumpDatabase(_ context.Context, _ runtime.Target, destPath string) error {
	f.calls = append(f.calls, "dump_database")
	return writeGzipFile(destPath, f.dumpSQL)
}

func (f *fakeRuntime) RequireHealthyServices(context.Context, runtime.Target, []string) error {
	f.calls = append(f.calls, "service_health")
	return nil
}

func (f *fakeRuntime) ResetDatabase(context.Context, runtime.Target) error {
	f.calls = append(f.calls, "reset_database")
	return nil
}

func (f *fakeRuntime) RestoreDatabase(_ context.Context, _ runtime.Target, sourcePath string) error {
	f.calls = append(f.calls, "restore_database")
	body, err := readGzipFile(sourcePath)
	if err != nil {
		return err
	}
	f.restoredSQL = body
	return nil
}

func (f *fakeRuntime) DBPing(context.Context, runtime.Target) error {
	f.calls = append(f.calls, "db_ping")
	return nil
}

func testConfig(t *testing.T) config.Config {
	t.Helper()

	root := t.TempDir()
	projectDir := filepath.Join(root, "project")
	storageDir := filepath.Join(root, "storage")
	backupRoot := filepath.Join(root, "backups")
	if err := os.Mkdir(projectDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(storageDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(backupRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, "compose.yaml"), []byte("services: {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, ".env.prod"), []byte("BACKUP_ROOT=backups\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	writeStorageFile(t, storageDir, "data/file.txt", "initial storage\n")

	return config.Config{
		Scope:          "prod",
		ProjectDir:     projectDir,
		ComposeFile:    filepath.Join(projectDir, "compose.yaml"),
		EnvFile:        filepath.Join(projectDir, ".env.prod"),
		BackupRoot:     backupRoot,
		StorageDir:     storageDir,
		AppServices:    []string{"espocrm", "espocrm-daemon"},
		DBService:      "db",
		DBUser:         "espocrm",
		DBPassword:     "dbpass",
		DBRootPassword: "rootpass",
		DBName:         "espocrm",
	}
}

func writeStorageFile(t *testing.T, root, rel, body string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func writeGzipFile(path, body string) (err error) {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer closeResource(file, &err)

	writer := gzip.NewWriter(file)
	defer closeResource(writer, &err)
	_, err = writer.Write([]byte(body))
	return err
}

func readGzipFile(path string) (body string, err error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer closeResource(file, &err)

	reader, err := gzip.NewReader(file)
	if err != nil {
		return "", err
	}
	defer closeResource(reader, &err)

	raw, err := io.ReadAll(reader)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}
