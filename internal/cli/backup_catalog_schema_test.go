package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSchema_BackupCatalog_JSON_ReadyVerified(t *testing.T) {
	tmp := t.TempDir()
	journalDir := filepath.Join(tmp, "journal")
	backupRoot := filepath.Join(tmp, "backups")
	if err := os.MkdirAll(journalDir, 0o755); err != nil {
		t.Fatal(err)
	}

	opts := []testAppOption{
		withFixedTestRuntime(time.Date(2026, 4, 15, 12, 0, 0, 0, time.UTC), "op-bc-1"),
	}

	writeCLICatalogBackupSet(t, backupRoot, "espocrm-dev", "2026-04-07_01-00-00", "dev")
	writeGzipFile(t, filepath.Join(backupRoot, "db", "espocrm-dev_2026-04-07_02-00-00.sql.gz"), []byte("select 2;"))
	writeJournalEntryFile(t, journalDir, "update.json", map[string]any{
		"operation_id": "op-update-1",
		"command":      "update",
		"started_at":   "2026-04-07T01:05:00Z",
		"finished_at":  "2026-04-07T01:07:00Z",
		"ok":           true,
		"artifacts": map[string]any{
			"manifest_json": filepath.Join(backupRoot, "manifests", "espocrm-dev_2026-04-07_01-00-00.manifest.json"),
		},
	})

	out, err := runRootCommandWithOptions(t, opts,
		"--journal-dir", journalDir,
		"--json",
		"backup-catalog",
		"--backup-root", backupRoot,
		"--ready-only",
		"--verify-checksum",
		"--latest-only",
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
	requireJSONPath(t, obj, "details", "backup_root")
	requireJSONPath(t, obj, "details", "verify_checksum")
	requireJSONPath(t, obj, "details", "ready_only")
	requireJSONPath(t, obj, "details", "limit")
	requireJSONPath(t, obj, "details", "total_sets")
	requireJSONPath(t, obj, "details", "shown_sets")
	requireJSONPath(t, obj, "details", "total_files_seen")
	requireJSONPath(t, obj, "items")

	if obj["command"] != "backup-catalog" {
		t.Fatalf("unexpected command: %v", obj["command"])
	}
	if ok, _ := obj["ok"].(bool); !ok {
		t.Fatalf("expected ok=true, got %v", obj["ok"])
	}

	items, ok := obj["items"].([]any)
	if !ok || len(items) != 1 {
		t.Fatalf("expected one catalog item, got %#v", obj["items"])
	}
	item, ok := items[0].(map[string]any)
	if !ok {
		t.Fatalf("expected object item, got %#v", items[0])
	}
	if item["id"] != "espocrm-dev_2026-04-07_01-00-00" {
		t.Fatalf("unexpected id: %v", item["id"])
	}
	if item["restore_readiness"] != "ready_verified" {
		t.Fatalf("unexpected readiness: %v", item["restore_readiness"])
	}
	if item["scope"] != "dev" {
		t.Fatalf("unexpected scope: %v", item["scope"])
	}
	origin, ok := item["origin"].(map[string]any)
	if !ok {
		t.Fatalf("expected origin object, got %#v", item["origin"])
	}
	if origin["kind"] != "update_recovery_point" {
		t.Fatalf("unexpected origin: %#v", origin)
	}
}

func TestSchema_BackupCatalog_JSON_CorruptedSidecarRemainsStructuredResult(t *testing.T) {
	tmp := t.TempDir()
	journalDir := filepath.Join(tmp, "journal")
	backupRoot := filepath.Join(tmp, "backups")

	opts := []testAppOption{
		withFixedTestRuntime(time.Date(2026, 4, 15, 12, 0, 0, 0, time.UTC), "op-bc-2"),
	}

	writeCLICatalogBackupSet(t, backupRoot, "espocrm-dev", "2026-04-07_01-00-00", "dev")
	if err := os.WriteFile(filepath.Join(backupRoot, "db", "espocrm-dev_2026-04-07_01-00-00.sql.gz.sha256"), []byte("bad sidecar\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	out, err := runRootCommandWithOptions(t, opts,
		"--journal-dir", journalDir,
		"--json",
		"backup-catalog",
		"--backup-root", backupRoot,
		"--verify-checksum",
		"--latest-only",
	)
	if err != nil {
		t.Fatalf("command failed: %v\noutput=%s", err, out)
	}

	var obj map[string]any
	if err := json.Unmarshal([]byte(out), &obj); err != nil {
		t.Fatal(err)
	}

	items, ok := obj["items"].([]any)
	if !ok || len(items) != 1 {
		t.Fatalf("expected one catalog item, got %#v", obj["items"])
	}
	item, ok := items[0].(map[string]any)
	if !ok {
		t.Fatalf("expected object item, got %#v", items[0])
	}
	if item["restore_readiness"] != "corrupted" {
		t.Fatalf("unexpected readiness: %v", item["restore_readiness"])
	}
	db, ok := item["db"].(map[string]any)
	if !ok {
		t.Fatalf("expected db artifact object, got %#v", item["db"])
	}
	if db["checksum_status"] != "mismatch" {
		t.Fatalf("unexpected checksum status: %v", db["checksum_status"])
	}
}

func writeCLICatalogBackupSet(t *testing.T, root, prefix, stamp, scope string) {
	t.Helper()

	dbName := prefix + "_" + stamp + ".sql.gz"
	filesName := prefix + "_files_" + stamp + ".tar.gz"
	dbPath := filepath.Join(root, "db", dbName)
	filesPath := filepath.Join(root, "files", filesName)
	manifestTXTPath := filepath.Join(root, "manifests", prefix+"_"+stamp+".manifest.txt")
	manifestJSONPath := filepath.Join(root, "manifests", prefix+"_"+stamp+".manifest.json")

	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(filesPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(manifestTXTPath), 0o755); err != nil {
		t.Fatal(err)
	}

	writeGzipFile(t, dbPath, []byte("select 1;"))
	writeTarGzFile(t, filesPath, map[string]string{
		"storage/test.txt": "hello",
	})
	writeCLICatalogSidecar(t, dbPath)
	writeCLICatalogSidecar(t, filesPath)

	if err := os.WriteFile(manifestTXTPath, []byte("created_at="+stamp+"\ncontour="+scope+"\ncompose_project=espocrm-test\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeJSON(t, manifestJSONPath, map[string]any{
		"version":    1,
		"scope":      scope,
		"created_at": "2026-04-07T01:00:00Z",
		"artifacts": map[string]any{
			"db_backup":    dbName,
			"files_backup": filesName,
		},
		"checksums": map[string]any{
			"db_backup":    sha256OfFile(t, dbPath),
			"files_backup": sha256OfFile(t, filesPath),
		},
	})
}

func writeCLICatalogSidecar(t *testing.T, filePath string) {
	t.Helper()

	body := sha256OfFile(t, filePath) + "  " + filepath.Base(filePath) + "\n"
	if err := os.WriteFile(filePath+".sha256", []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}
