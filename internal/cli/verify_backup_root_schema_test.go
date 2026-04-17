package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSchema_VerifyBackup_JSON_BackupRoot_SelectsLatestCompleteSet(t *testing.T) {
	tmp := t.TempDir()
	journalDir := filepath.Join(tmp, "journal")
	backupRoot := filepath.Join(tmp, "backups")

	opts := []testAppOption{
		withFixedTestRuntime(time.Date(2026, 4, 15, 12, 0, 0, 0, time.UTC), "op-vb-root-1"),
	}

	validManifest := writeBackupSetForRootSelection(t, backupRoot, "espocrm-dev", "2026-04-07_01-00-00", "dev")
	writeIncompleteManifestForRootSelection(t, backupRoot, "espocrm-dev", "2026-04-07_02-00-00", "dev")

	out, err := runRootCommandWithOptions(t, opts,
		"--journal-dir", journalDir,
		"--json",
		"verify-backup",
		"--backup-root", backupRoot,
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
	requireJSONPath(t, obj, "artifacts", "manifest")
	requireJSONPath(t, obj, "artifacts", "db_backup")
	requireJSONPath(t, obj, "artifacts", "files_backup")

	if manifest := requireJSONPath(t, obj, "artifacts", "manifest"); manifest != validManifest {
		t.Fatalf("expected selected manifest %s, got %v", validManifest, manifest)
	}
}

func writeBackupSetForRootSelection(t *testing.T, root, prefix, stamp, scope string) string {
	t.Helper()

	dbName := prefix + "_" + stamp + ".sql.gz"
	filesName := prefix + "_files_" + stamp + ".tar.gz"
	dbPath := filepath.Join(root, "db", dbName)
	filesPath := filepath.Join(root, "files", filesName)
	manifestPath := filepath.Join(root, "manifests", prefix+"_"+stamp+".manifest.json")

	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(filesPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(manifestPath), 0o755); err != nil {
		t.Fatal(err)
	}

	writeGzipFile(t, dbPath, []byte("select 1;"))
	writeTarGzFile(t, filesPath, map[string]string{
		"storage/a.txt": "hello",
	})
	writeJSON(t, manifestPath, map[string]any{
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

	return manifestPath
}

func writeIncompleteManifestForRootSelection(t *testing.T, root, prefix, stamp, scope string) string {
	t.Helper()

	dbName := prefix + "_" + stamp + ".sql.gz"
	filesName := prefix + "_files_" + stamp + ".tar.gz"
	manifestPath := filepath.Join(root, "manifests", prefix+"_"+stamp+".manifest.json")
	if err := os.MkdirAll(filepath.Dir(manifestPath), 0o755); err != nil {
		t.Fatal(err)
	}
	writeJSON(t, manifestPath, map[string]any{
		"version":    1,
		"scope":      scope,
		"created_at": "2026-04-07T02:00:00Z",
		"artifacts": map[string]any{
			"db_backup":    dbName,
			"files_backup": filesName,
		},
		"checksums": map[string]any{
			"db_backup":    "abababababababababababababababababababababababababababababababab",
			"files_backup": "cdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcd",
		},
	})

	return manifestPath
}
