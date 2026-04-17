package cli

import (
	"encoding/json"
	"path/filepath"
	"testing"
	"time"
)

func TestSchema_RestoreFiles_JSON_DryRun(t *testing.T) {
	tmp := t.TempDir()
	journalDir := filepath.Join(tmp, "journal")
	targetDir := filepath.Join(tmp, "storage")

	opts := []testAppOption{
		withFixedTestRuntime(time.Date(2026, 4, 15, 12, 0, 0, 0, time.UTC), "op-rf-1"),
	}

	dbName := "db.sql.gz"
	filesName := "files.tar.gz"
	manifestPath := filepath.Join(tmp, "manifest.json")
	dbPath := filepath.Join(tmp, dbName)
	filesPath := filepath.Join(tmp, filesName)

	writeGzipFile(t, dbPath, []byte("select 1;"))
	writeTarGzFile(t, filesPath, map[string]string{
		"storage/a.txt": "hello",
	})
	writeJSON(t, manifestPath, map[string]any{
		"version":    1,
		"scope":      "prod",
		"created_at": "2026-04-15T11:00:00Z",
		"artifacts": map[string]any{
			"db_backup":    dbName,
			"files_backup": filesName,
		},
		"checksums": map[string]any{
			"db_backup":    sha256OfFile(t, dbPath),
			"files_backup": sha256OfFile(t, filesPath),
		},
	})

	out, err := runRootCommandWithOptions(t, opts,
		"--journal-dir", journalDir,
		"--json",
		"restore-files",
		"--manifest", manifestPath,
		"--target-dir", targetDir,
		"--dry-run",
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
	requireJSONPath(t, obj, "details", "dry_run")
	requireJSONPath(t, obj, "details", "plan", "source_kind")
	requireJSONPath(t, obj, "details", "plan", "source_path")
	requireJSONPath(t, obj, "details", "plan", "destructive")
	requireJSONPath(t, obj, "details", "plan", "changes")
	requireJSONPath(t, obj, "details", "plan", "non_changes")
	requireJSONPath(t, obj, "details", "plan", "checks")
	requireJSONPath(t, obj, "details", "plan", "next_step")
	requireJSONPath(t, obj, "artifacts", "manifest")
	requireJSONPath(t, obj, "artifacts", "target_dir")

	if obj["command"] != "restore-files" {
		t.Fatalf("unexpected command: %v", obj["command"])
	}
	if ok, _ := obj["ok"].(bool); !ok {
		t.Fatalf("expected ok=true, got %v", obj["ok"])
	}
	if dryRun, _ := requireJSONPath(t, obj, "details", "dry_run").(bool); !dryRun {
		t.Fatalf("expected details.dry_run=true")
	}
	if sourceKind := requireJSONPath(t, obj, "details", "plan", "source_kind"); sourceKind != "manifest" {
		t.Fatalf("unexpected details.plan.source_kind: %v", sourceKind)
	}
	if destructive, _ := requireJSONPath(t, obj, "details", "plan", "destructive").(bool); !destructive {
		t.Fatalf("expected details.plan.destructive=true")
	}
	checks, ok := requireJSONPath(t, obj, "details", "plan", "checks").([]any)
	if !ok || len(checks) == 0 {
		t.Fatalf("expected non-empty details.plan.checks, got %v", requireJSONPath(t, obj, "details", "plan", "checks"))
	}
}
