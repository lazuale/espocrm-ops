package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSchema_Backup_JSON_Metadata(t *testing.T) {
	tmp := t.TempDir()
	journalDir := filepath.Join(tmp, "journal")

	opts := []testAppOption{
		withFixedTestRuntime(time.Date(2026, 4, 15, 12, 0, 0, 0, time.UTC), "op-backup-1"),
	}

	dbPath := filepath.Join(tmp, "db", "espocrm-dev_2026-04-15_11-00-00.sql.gz")
	filesPath := filepath.Join(tmp, "files", "espocrm-dev_files_2026-04-15_11-00-00.tar.gz")
	manifestPath := filepath.Join(tmp, "manifests", "espocrm-dev_2026-04-15_11-00-00.manifest.json")
	dbChecksumPath := filepath.Join(tmp, "checksums", "db.sha256.tmp")
	filesChecksumPath := filepath.Join(tmp, "checksums", "files.sha256.tmp")
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(filesPath), 0o755); err != nil {
		t.Fatal(err)
	}
	writeGzipFile(t, dbPath, []byte("select 1;"))
	writeTarGzFile(t, filesPath, map[string]string{
		"storage/a.txt": "hello",
	})

	out, err := runRootCommandWithOptions(t, opts,
		"--journal-dir", journalDir,
		"--json",
		"backup",
		"--scope", "dev",
		"--created-at", "2026-04-15T11:00:00Z",
		"--db-backup", dbPath,
		"--files-backup", filesPath,
		"--manifest", manifestPath,
		"--db-checksum", dbChecksumPath,
		"--files-checksum", filesChecksumPath,
	)
	if err != nil {
		t.Fatalf("command failed: %v\noutput=%s", err, out)
	}

	var obj map[string]any
	if err := json.Unmarshal([]byte(out), &obj); err != nil {
		t.Fatal(err)
	}

	requireJSONPath(t, obj, "command")
	requireJSONPath(t, obj, "ok")
	requireJSONPath(t, obj, "message")
	requireJSONPath(t, obj, "details", "scope")
	requireJSONPath(t, obj, "details", "created_at")
	requireJSONPath(t, obj, "details", "sidecars")
	requireJSONPath(t, obj, "artifacts", "manifest")
	requireJSONPath(t, obj, "artifacts", "db_backup")
	requireJSONPath(t, obj, "artifacts", "files_backup")
	requireJSONPath(t, obj, "artifacts", "db_checksum")
	requireJSONPath(t, obj, "artifacts", "files_checksum")

	if obj["command"] != "backup" {
		t.Fatalf("unexpected command: %v", obj["command"])
	}
	if ok, _ := obj["ok"].(bool); !ok {
		t.Fatalf("expected ok=true, got %v", obj["ok"])
	}
	if scope := requireJSONPath(t, obj, "details", "scope"); scope != "dev" {
		t.Fatalf("unexpected scope: %v", scope)
	}
	if createdAt := requireJSONPath(t, obj, "details", "created_at"); createdAt != "2026-04-15T11:00:00Z" {
		t.Fatalf("unexpected created_at: %v", createdAt)
	}

	if dbChecksum := requireJSONPath(t, obj, "artifacts", "db_checksum"); dbChecksum != dbChecksumPath {
		t.Fatalf("unexpected db_checksum: %v", dbChecksum)
	}
	if filesChecksum := requireJSONPath(t, obj, "artifacts", "files_checksum"); filesChecksum != filesChecksumPath {
		t.Fatalf("unexpected files_checksum: %v", filesChecksum)
	}

	for _, path := range []string{manifestPath, dbChecksumPath, filesChecksumPath} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected backup metadata artifact %s: %v", path, err)
		}
	}
}
