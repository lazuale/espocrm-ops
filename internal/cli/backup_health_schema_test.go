package cli

import (
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/lazuale/espocrm-ops/internal/contract/exitcode"
)

func TestSchema_BackupHealth_JSON_Healthy(t *testing.T) {
	tmp := t.TempDir()
	journalDir := filepath.Join(tmp, "journal")
	backupRoot := filepath.Join(tmp, "backups")

	opts := []testAppOption{
		withFixedTestRuntime(time.Date(2026, 4, 15, 12, 0, 0, 0, time.UTC), "op-bh-healthy-1"),
	}

	writeCLICatalogBackupSet(t, backupRoot, "espocrm-dev", "2026-04-15_10-00-00", "dev")

	out, err := runRootCommandWithOptions(t, opts,
		"--journal-dir", journalDir,
		"--json",
		"backup-health",
		"--backup-root", backupRoot,
		"--max-age-hours", "48",
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
	requireJSONPath(t, obj, "details", "verdict")
	requireJSONPath(t, obj, "details", "restore_ready")
	requireJSONPath(t, obj, "details", "freshness_satisfied")
	requireJSONPath(t, obj, "details", "verification_satisfied")
	requireJSONPath(t, obj, "details", "latest_ready_set_id")
	requireJSONPath(t, obj, "artifacts", "latest_set", "id")
	requireJSONPath(t, obj, "artifacts", "latest_ready_set", "id")

	if obj["command"] != "backup-health" {
		t.Fatalf("unexpected command: %v", obj["command"])
	}
	if ok, _ := obj["ok"].(bool); !ok {
		t.Fatalf("expected ok=true, got %v", obj["ok"])
	}
	if verdict := requireJSONPath(t, obj, "details", "verdict"); verdict != "healthy" {
		t.Fatalf("unexpected verdict: %v", verdict)
	}
	if items, ok := obj["items"].([]any); ok && len(items) != 0 {
		t.Fatalf("expected no alerts for healthy posture, got %#v", items)
	}
}

func TestSchema_BackupHealth_JSON_BlockedIncludesStructuredResult(t *testing.T) {
	tmp := t.TempDir()
	journalDir := filepath.Join(tmp, "journal")
	backupRoot := filepath.Join(tmp, "backups")

	opts := []testAppOption{
		withFixedTestRuntime(time.Date(2026, 4, 15, 12, 0, 0, 0, time.UTC), "op-bh-blocked-1"),
	}

	writeCLICatalogDBOnly(t, backupRoot, "espocrm-dev", "2026-04-15_10-00-00")

	outcome := executeCLIWithOptions(opts,
		"--journal-dir", journalDir,
		"--json",
		"backup-health",
		"--backup-root", backupRoot,
	)

	if outcome.ExitCode != exitcode.ValidationError {
		t.Fatalf("expected validation exit code, got %d stdout=%s stderr=%s", outcome.ExitCode, outcome.Stdout, outcome.Stderr)
	}
	if outcome.Stderr != "" {
		t.Fatalf("expected empty stderr, got %q", outcome.Stderr)
	}

	var obj map[string]any
	if err := json.Unmarshal([]byte(outcome.Stdout), &obj); err != nil {
		t.Fatal(err)
	}

	if command := requireJSONPath(t, obj, "command"); command != "backup-health" {
		t.Fatalf("unexpected command: %v", command)
	}
	if ok, _ := requireJSONPath(t, obj, "ok").(bool); ok {
		t.Fatalf("expected ok=false")
	}
	if verdict := requireJSONPath(t, obj, "details", "verdict"); verdict != "blocked" {
		t.Fatalf("unexpected verdict: %v", verdict)
	}
	if code := requireJSONPath(t, obj, "error", "code"); code != "backup_health_blocked" {
		t.Fatalf("unexpected error code: %v", code)
	}
	exitCode, _ := requireJSONPath(t, obj, "error", "exit_code").(float64)
	if int(exitCode) != exitcode.ValidationError {
		t.Fatalf("unexpected error exit code: %v", exitCode)
	}

	items, ok := obj["items"].([]any)
	if !ok || len(items) == 0 {
		t.Fatalf("expected non-empty backup-health alerts, got %#v", obj["items"])
	}
	item, ok := items[0].(map[string]any)
	if !ok {
		t.Fatalf("expected alert object, got %#v", items[0])
	}
	if item["code"] != "no_restore_ready_backup" || item["level"] != "breach" {
		t.Fatalf("unexpected alert payload: %#v", item)
	}
}
