package cli

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSchema_BackupVerify_JSON_ManifestSuccessV2Contract(t *testing.T) {
	tmp := t.TempDir()
	journalDir := filepath.Join(tmp, "journal")
	backupRoot := filepath.Join(tmp, "backups")
	set := writeBackupSet(t, backupRoot, "espocrm-dev", "2026-04-07_01-00-00", "dev", map[string]string{
		"storage/a.txt": "hello",
	})

	out, err := runRootCommandWithOptions(t, []testAppOption{
		withFixedTestRuntime(time.Date(2026, 4, 15, 12, 0, 0, 0, time.UTC), "op-vb-manifest-1"),
	},
		"--journal-dir", journalDir,
		"--json",
		"backup",
		"verify",
		"--manifest", set.ManifestJSON,
	)
	if err != nil {
		t.Fatalf("command failed: %v\noutput=%s", err, out)
	}

	obj := decodeCLIJSON(t, out)
	if command := requireJSONString(t, obj, "command"); command != "backup verify" {
		t.Fatalf("unexpected command: %s", command)
	}
	if !requireJSONBool(t, obj, "ok") {
		t.Fatal("expected ok=true")
	}
	if exitCode := requireJSONInt(t, obj, "process_exit_code"); exitCode != 0 {
		t.Fatalf("unexpected process_exit_code: %d", exitCode)
	}
	if sourceKind := requireJSONString(t, obj, "details", "source_kind"); sourceKind != "manifest" {
		t.Fatalf("expected manifest source kind, got %v", sourceKind)
	}
	if !requireJSONBool(t, obj, "details", "ready") {
		t.Fatal("expected ready=true")
	}
	requireArtifactPathsExist(t, obj, "manifest", "db_backup", "db_checksum", "files_backup", "files_checksum")
}

func TestSchema_BackupVerify_JSON_BackupRoot_SelectsLatestCompleteSet(t *testing.T) {
	tmp := t.TempDir()
	journalDir := filepath.Join(tmp, "journal")
	backupRoot := filepath.Join(tmp, "backups")

	opts := []testAppOption{
		withFixedTestRuntime(time.Date(2026, 4, 15, 12, 0, 0, 0, time.UTC), "op-vb-root-1"),
	}

	validSet := writeBackupSet(t, backupRoot, "espocrm-dev", "2026-04-07_01-00-00", "dev", map[string]string{
		"storage/a.txt": "hello",
	})
	writeIncompleteManifestForRootSelection(t, backupRoot, "espocrm-dev", "2026-04-07_02-00-00", "dev")

	out, err := runRootCommandWithOptions(t, opts,
		"--journal-dir", journalDir,
		"--json",
		"backup",
		"verify",
		"--backup-root", backupRoot,
	)
	if err != nil {
		t.Fatalf("command failed: %v\noutput=%s", err, out)
	}

	obj := decodeCLIJSON(t, out)

	if manifest := requireJSONString(t, obj, "artifacts", "manifest"); manifest != validSet.ManifestJSON {
		t.Fatalf("expected selected manifest %s, got %v", validSet.ManifestJSON, manifest)
	}
	if backupRootJSON := requireJSONString(t, obj, "artifacts", "backup_root"); backupRootJSON != backupRoot {
		t.Fatalf("expected backup root %s, got %v", backupRoot, backupRootJSON)
	}
	if sourceKind := requireJSONString(t, obj, "details", "source_kind"); sourceKind != "backup_root" {
		t.Fatalf("expected backup_root source kind, got %v", sourceKind)
	}
	if !requireJSONBool(t, obj, "details", "ready") {
		t.Fatal("expected ready=true")
	}
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
