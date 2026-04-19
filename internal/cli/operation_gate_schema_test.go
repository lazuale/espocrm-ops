package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/lazuale/espocrm-ops/internal/contract/exitcode"
)

func TestSchema_OperationGate_JSON_BlockedIncludesStructuredResult(t *testing.T) {
	projectDir := filepath.Join(t.TempDir(), "project")
	journalDir := filepath.Join(t.TempDir(), "journal")
	fixedNow := time.Date(2026, 4, 19, 9, 0, 0, 0, time.UTC)

	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(journalDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(projectDir, "backups", "prod"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, "compose.yaml"), []byte("services: {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	writeDoctorEnvFile(t, projectDir, "dev", map[string]string{
		"ESPOCRM_IMAGE": "espocrm/espocrm:9.3.4-apache",
	})
	writeDoctorEnvFile(t, projectDir, "prod", map[string]string{
		"ESPOCRM_IMAGE": "espocrm/espocrm:9.4.0-apache",
	})
	writeRollbackBackupSet(t, filepath.Join(projectDir, "backups", "dev"), "espocrm-dev", "2026-04-19_08-00-00", "dev")
	writeJournalEntryFile(t, journalDir, "backup.json", map[string]any{
		"operation_id": "op-backup-1",
		"command":      "backup",
		"started_at":   "2026-04-19T08:00:00Z",
		"finished_at":  "2026-04-19T08:05:00Z",
		"ok":           true,
		"message":      "backup created",
		"details": map[string]any{
			"scope": "dev",
		},
		"artifacts": map[string]any{
			"manifest_txt":  filepath.Join(projectDir, "backups", "dev", "manifests", "espocrm-dev_2026-04-19_08-00-00.manifest.txt"),
			"manifest_json": filepath.Join(projectDir, "backups", "dev", "manifests", "espocrm-dev_2026-04-19_08-00-00.manifest.json"),
			"db_backup":     filepath.Join(projectDir, "backups", "dev", "db", "espocrm-dev_2026-04-19_08-00-00.sql.gz"),
			"files_backup":  filepath.Join(projectDir, "backups", "dev", "files", "espocrm-dev_files_2026-04-19_08-00-00.tar.gz"),
		},
	})
	prependSupportBundleFakeDocker(t)

	outcome := executeCLIWithOptions(
		[]testAppOption{withFixedTestRuntime(fixedNow, "op-operation-gate-blocked-1")},
		"--journal-dir", journalDir,
		"--json",
		"operation-gate",
		"--action", "migrate-backup",
		"--from", "dev",
		"--to", "prod",
		"--project-dir", projectDir,
	)

	if outcome.ExitCode != exitcode.ValidationError {
		t.Fatalf("expected validation exit code %d, got %d\nstdout=%s\nstderr=%s", exitcode.ValidationError, outcome.ExitCode, outcome.Stdout, outcome.Stderr)
	}
	if outcome.Stderr != "" {
		t.Fatalf("expected empty stderr, got %q", outcome.Stderr)
	}

	var obj map[string]any
	if err := json.Unmarshal([]byte(outcome.Stdout), &obj); err != nil {
		t.Fatal(err)
	}

	if command := requireJSONPath(t, obj, "command"); command != "operation-gate" {
		t.Fatalf("unexpected command: %v", command)
	}
	if action := requireJSONPath(t, obj, "details", "action"); action != "migrate-backup" {
		t.Fatalf("unexpected action: %v", action)
	}
	if ok, _ := requireJSONPath(t, obj, "ok").(bool); ok {
		t.Fatalf("expected ok=false")
	}
	if decision := requireJSONPath(t, obj, "details", "decision"); decision != "blocked" {
		t.Fatalf("unexpected decision: %v", decision)
	}
	if code := requireJSONPath(t, obj, "error", "code"); code != "operation_gate_blocked" {
		t.Fatalf("unexpected error code: %v", code)
	}
	if targetVerdict := requireJSONPath(t, obj, "details", "target_health_verdict"); targetVerdict != "blocked" {
		t.Fatalf("unexpected target health verdict: %v", targetVerdict)
	}

	items := requireJSONPath(t, obj, "items").([]any)
	if len(items) == 0 {
		t.Fatalf("expected blocking alerts, got %#v", obj["items"])
	}
	first, ok := items[0].(map[string]any)
	if !ok {
		t.Fatalf("expected alert object, got %#v", items[0])
	}
	if first["section"] != "action_readiness" {
		t.Fatalf("unexpected first alert payload: %#v", first)
	}
}

func TestSchema_OperationGate_JSON_FailedIncludesSectionStatus(t *testing.T) {
	projectDir := t.TempDir()
	journalDir := filepath.Join(t.TempDir(), "journal")

	if err := os.MkdirAll(journalDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, "compose.yaml"), []byte("services: {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	outcome := executeCLI(
		"--journal-dir", journalDir,
		"--json",
		"operation-gate",
		"--action", "update",
		"--scope", "dev",
		"--project-dir", projectDir,
	)

	if outcome.ExitCode != exitcode.ValidationError {
		t.Fatalf("expected validation exit code %d, got %d\nstdout=%s\nstderr=%s", exitcode.ValidationError, outcome.ExitCode, outcome.Stdout, outcome.Stderr)
	}
	if outcome.Stderr != "" {
		t.Fatalf("expected empty stderr, got %q", outcome.Stderr)
	}

	var obj map[string]any
	if err := json.Unmarshal([]byte(outcome.Stdout), &obj); err != nil {
		t.Fatal(err)
	}

	if decision := requireJSONPath(t, obj, "details", "decision"); decision != "blocked" {
		t.Fatalf("unexpected decision: %v", decision)
	}
	if code := requireJSONPath(t, obj, "error", "code"); code != "operation_gate_failed" {
		t.Fatalf("unexpected error code: %v", code)
	}

	assertJSONListContains(t, obj, []string{"details", "failed_sections"}, "health")
}
