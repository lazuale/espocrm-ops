package ops

import (
	"compress/gzip"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/lazuale/espocrm-ops/internal/config"
	"github.com/lazuale/espocrm-ops/internal/runtime"
)

func TestBackupAndRestoreStayLinear(t *testing.T) {
	cfg := testConfig(t)
	dumpPath := filepath.Join(t.TempDir(), "dump.sql")
	importPath := filepath.Join(t.TempDir(), "import.sql")
	callsPath := filepath.Join(t.TempDir(), "calls")
	if err := os.WriteFile(dumpPath, []byte("create table account(id int);\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	installFakeDocker(t, callsPath, dumpPath, importPath)

	now := time.Date(2026, 4, 26, 13, 14, 15, 0, time.UTC)
	result, err := Backup(context.Background(), cfg, now)
	if err != nil {
		t.Fatalf("Backup failed: %v", err)
	}

	wantDir := filepath.Join(cfg.BackupRoot, cfg.Scope, now.Format(backupTimestampFormat))
	if result.Manifest != filepath.Join(wantDir, ManifestName) {
		t.Fatalf("unexpected manifest: %s", result.Manifest)
	}
	assertManifestOnlyHasChecksums(t, result.Manifest)

	writeStorageFile(t, cfg.StorageDir, "data/file.txt", "changed\n")
	if err := Restore(context.Background(), cfg, result.Manifest); err != nil {
		t.Fatalf("Restore failed: %v", err)
	}

	restored, err := os.ReadFile(filepath.Join(cfg.StorageDir, "data", "file.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(restored) != "initial storage\n" {
		t.Fatalf("storage was not restored: %q", restored)
	}
	imported, err := os.ReadFile(importPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(imported) != "create table account(id int);\n" {
		t.Fatalf("wrong imported sql: %q", imported)
	}

	calls, err := os.ReadFile(callsPath)
	if err != nil {
		t.Fatal(err)
	}
	got := strings.Join(strings.Fields(strings.TrimSpace(string(calls))), " ")
	want := "stop espocrm,espocrm-daemon dump db start espocrm,espocrm-daemon stop espocrm,espocrm-daemon reset db import db start espocrm,espocrm-daemon"
	if got != want {
		t.Fatalf("unexpected calls:\nwant %s\ngot  %s", want, got)
	}
}

func TestVerifyDoesNotInspectTarContents(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, DBFileName)
	filesPath := filepath.Join(dir, FilesFileName)
	writeGzipFile(t, dbPath, "sql\n")
	writeGzipFile(t, filesPath, "not a tar stream\n")

	dbSHA, err := runtime.SHA256(context.Background(), dbPath)
	if err != nil {
		t.Fatal(err)
	}
	filesSHA, err := runtime.SHA256(context.Background(), filesPath)
	if err != nil {
		t.Fatal(err)
	}
	manifestPath := filepath.Join(dir, ManifestName)
	if err := writeManifest(manifestPath, dbSHA, filesSHA); err != nil {
		t.Fatal(err)
	}

	if _, err := VerifyBackup(context.Background(), manifestPath); err != nil {
		t.Fatalf("VerifyBackup should only require gzip readability: %v", err)
	}
}

func TestVerifyRejectsChecksumMismatch(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, DBFileName)
	filesPath := filepath.Join(dir, FilesFileName)
	writeGzipFile(t, dbPath, "sql\n")
	writeGzipFile(t, filesPath, "files\n")

	filesSHA, err := runtime.SHA256(context.Background(), filesPath)
	if err != nil {
		t.Fatal(err)
	}
	manifestPath := filepath.Join(dir, ManifestName)
	if err := writeManifest(manifestPath, strings.Repeat("0", 64), filesSHA); err != nil {
		t.Fatal(err)
	}

	if _, err := VerifyBackup(context.Background(), manifestPath); err == nil || !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("expected checksum mismatch, got %v", err)
	}
}

func assertManifestOnlyHasChecksums(t *testing.T, path string) {
	t.Helper()

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var values map[string]string
	if err := json.Unmarshal(raw, &values); err != nil {
		t.Fatal(err)
	}
	if len(values) != 2 || values["db"] == "" || values["files"] == "" {
		t.Fatalf("manifest must contain only db/files checksums: %#v", values)
	}
}

func installFakeDocker(t *testing.T, callsPath, dumpPath, importPath string) {
	t.Helper()

	bin := t.TempDir()
	script := `#!/bin/sh
set -eu

log() {
  printf '%s\n' "$1" >> "$ESP_TEST_CALLS"
}

if [ "${1:-}" = "compose" ]; then
  shift
fi
while [ "$#" -gt 0 ]; do
  case "$1" in
    --env-file|-f)
      shift 2
      ;;
    *)
      break
      ;;
  esac
done

cmd="${1:-}"
if [ "$#" -gt 0 ]; then
  shift
fi

case "$cmd" in
  config)
    exit 0
    ;;
  stop)
    log "stop $*"
    exit 0
    ;;
  up)
    if [ "${1:-}" = "-d" ]; then
      shift
    fi
    log "start $*"
    exit 0
    ;;
  exec)
    while [ "$#" -gt 0 ]; do
      case "$1" in
        -T)
          shift
          ;;
        -e)
          shift 2
          ;;
        *)
          break
          ;;
      esac
    done
    service="${1:-}"
    if [ "$#" -gt 0 ]; then
      shift
    fi
    tool="${1:-}"
    if [ "$#" -gt 0 ]; then
      shift
    fi
    case "$tool" in
      mariadb-dump)
        log "dump $service"
        cat "$ESP_TEST_DUMP"
        exit 0
        ;;
      mariadb)
        args="$*"
        case "$args" in
          *"SELECT 1"*)
            log "ping $service"
            exit 0
            ;;
          *"DROP DATABASE"*)
            log "reset $service"
            exit 0
            ;;
          *)
            log "import $service"
            cat > "$ESP_TEST_IMPORT"
            exit 0
            ;;
        esac
        ;;
    esac
    ;;
esac

echo "unexpected docker args: $cmd $*" >&2
exit 1
`
	path := filepath.Join(bin, "docker")
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("ESP_TEST_CALLS", callsPath)
	t.Setenv("ESP_TEST_DUMP", dumpPath)
	t.Setenv("ESP_TEST_IMPORT", importPath)
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
		AppServices:    []string{"espocrm,espocrm-daemon"},
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

func writeGzipFile(t *testing.T, path, body string) {
	t.Helper()
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
}
