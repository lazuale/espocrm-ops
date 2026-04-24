package cli

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/lazuale/espocrm-ops/internal/ops"
)

func TestMigrateCLIJSONSuccess(t *testing.T) {
	manifestPath, wantSQL := writeVerifiedScopedBackupSet(t, "dev")

	projectDir := t.TempDir()
	storageDir := filepath.Join(projectDir, "runtime", "prod", "espo")
	if err := os.MkdirAll(storageDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(storageDir, "old.txt"), []byte("old\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, "compose.yaml"), []byte("services: {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, ".env.prod"), []byte(strings.Join([]string{
		"ESPO_CONTOUR=prod",
		"BACKUP_ROOT=./backups/prod",
		"ESPO_STORAGE_DIR=./runtime/prod/espo",
		"ESPO_RUNTIME_UID=" + currentRuntimeUIDString(),
		"ESPO_RUNTIME_GID=" + currentRuntimeGIDString(),
		"APP_SERVICES=espocrm,espocrm-daemon,espocrm-websocket",
		"DB_SERVICE=db",
		"DB_USER=espocrm",
		"DB_PASSWORD=db-secret",
		"DB_ROOT_PASSWORD=root-secret",
		"DB_NAME=espocrm",
		"",
	}, "\n")), 0o644); err != nil {
		t.Fatal(err)
	}

	stdinLogPath := filepath.Join(t.TempDir(), "migrate-db.sql")
	prependRestoreFakeDocker(t)
	t.Setenv("TEST_RESTORE_DOCKER_STDIN_LOG", stdinLogPath)

	oldNow := migrateNow
	migrateNow = func() time.Time {
		return time.Date(2026, 4, 24, 19, 0, 0, 0, time.UTC)
	}
	defer func() { migrateNow = oldNow }()

	stdout := &strings.Builder{}
	stderr := &strings.Builder{}
	exitCode := Execute([]string{"migrate", "--from-scope", "dev", "--to-scope", "prod", "--project-dir", projectDir, "--manifest", manifestPath}, stdout, stderr)
	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d stdout=%s stderr=%s", exitCode, stdout.String(), stderr.String())
	}

	var obj map[string]any
	if err := json.Unmarshal([]byte(stdout.String()), &obj); err != nil {
		t.Fatal(err)
	}
	if command := requireJSONString(t, obj, "command"); command != "migrate" {
		t.Fatalf("unexpected command: %s", command)
	}
	if !requireJSONBool(t, obj, "ok") {
		t.Fatal("expected ok=true")
	}
	if message := requireJSONString(t, obj, "message"); message != "migrate completed" {
		t.Fatalf("unexpected message: %s", message)
	}
	if gotManifest := requireJSONString(t, obj, "result", "manifest"); gotManifest != manifestPath {
		t.Fatalf("unexpected manifest: %s", gotManifest)
	}
	snapshotManifest := requireJSONString(t, obj, "result", "snapshot_manifest")
	if _, err := ops.VerifyBackup(context.Background(), snapshotManifest); err != nil {
		t.Fatalf("snapshot manifest did not verify: %v", err)
	}
	raw, err := os.ReadFile(stdinLogPath)
	if err != nil {
		t.Fatal(err)
	}
	if body := string(raw); body != wantSQL {
		t.Fatalf("unexpected restore db body: %q", body)
	}
	if stderr.Len() != 0 {
		t.Fatalf("expected empty stderr, got %q", stderr.String())
	}
	if _, err := os.Stat(filepath.Join(storageDir, "restored.txt")); err != nil {
		t.Fatalf("expected restored file: %v", err)
	}
	if _, err := os.Stat(filepath.Join(storageDir, "old.txt")); !os.IsNotExist(err) {
		t.Fatalf("expected old file removed, got %v", err)
	}
}

func TestMigrateCLIJSONFailure(t *testing.T) {
	projectDir := t.TempDir()
	storageDir := filepath.Join(projectDir, "runtime", "prod", "espo")
	if err := os.MkdirAll(storageDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(storageDir, "old.txt"), []byte("old\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, "compose.yaml"), []byte("services: {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, ".env.prod"), []byte(strings.Join([]string{
		"ESPO_CONTOUR=prod",
		"BACKUP_ROOT=./backups/prod",
		"ESPO_STORAGE_DIR=./runtime/prod/espo",
		"ESPO_RUNTIME_UID=" + currentRuntimeUIDString(),
		"ESPO_RUNTIME_GID=" + currentRuntimeGIDString(),
		"APP_SERVICES=espocrm,espocrm-daemon,espocrm-websocket",
		"DB_SERVICE=db",
		"DB_USER=espocrm",
		"DB_PASSWORD=db-secret",
		"DB_ROOT_PASSWORD=root-secret",
		"DB_NAME=espocrm",
		"",
	}, "\n")), 0o644); err != nil {
		t.Fatal(err)
	}

	missingManifest := filepath.Join(projectDir, "missing.manifest.json")
	stdout := &strings.Builder{}
	stderr := &strings.Builder{}
	exitCode := Execute([]string{"migrate", "--from-scope", "prod", "--to-scope", "prod", "--project-dir", projectDir, "--manifest", missingManifest}, stdout, stderr)
	if exitCode != exitUsage {
		t.Fatalf("expected exit code %d, got %d stdout=%s stderr=%s", exitUsage, exitCode, stdout.String(), stderr.String())
	}

	var obj map[string]any
	if err := json.Unmarshal([]byte(stdout.String()), &obj); err != nil {
		t.Fatal(err)
	}
	if command := requireJSONString(t, obj, "command"); command != "migrate" {
		t.Fatalf("unexpected command: %s", command)
	}
	if requireJSONBool(t, obj, "ok") {
		t.Fatal("expected ok=false")
	}
	if message := requireJSONString(t, obj, "message"); message != "migrate failed" {
		t.Fatalf("unexpected message: %s", message)
	}
	if kind := requireJSONString(t, obj, "error", "kind"); kind != "usage" {
		t.Fatalf("unexpected error kind: %s", kind)
	}
	if errMessage := requireJSONString(t, obj, "error", "message"); errMessage != "--from-scope and --to-scope must differ" {
		t.Fatalf("unexpected error message: %s", errMessage)
	}
	if stderr.Len() != 0 {
		t.Fatalf("expected empty stderr, got %q", stderr.String())
	}
}

func writeVerifiedScopedBackupSet(t *testing.T, scope string) (manifestPath, dbSQL string) {
	t.Helper()

	root := t.TempDir()
	dbPath := filepath.Join(root, "db", "restore-source.sql.gz")
	filesPath := filepath.Join(root, "files", "restore-source.tar.gz")
	manifestPath = filepath.Join(root, "manifests", "restore-source.manifest.json")

	for _, dir := range []string{filepath.Dir(dbPath), filepath.Dir(filesPath), filepath.Dir(manifestPath)} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}

	dbSQL = "create table restored(id int);\n"
	writeGzipFile(t, dbPath, []byte(dbSQL))
	writeTarGzFile(t, filesPath, map[string]string{"restored.txt": "restored\n"})
	rewriteSidecar(t, dbPath)
	rewriteSidecar(t, filesPath)

	raw, err := json.MarshalIndent(map[string]any{
		"version":    1,
		"scope":      scope,
		"created_at": "2026-04-24T19:00:00Z",
		"artifacts": map[string]any{
			"db_backup":    filepath.Base(dbPath),
			"files_backup": filepath.Base(filesPath),
		},
		"checksums": map[string]any{
			"db_backup":    sha256OfFile(t, dbPath),
			"files_backup": sha256OfFile(t, filesPath),
		},
	}, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manifestPath, append(raw, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}

	return manifestPath, dbSQL
}
