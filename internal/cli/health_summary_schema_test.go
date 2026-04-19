package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/lazuale/espocrm-ops/internal/contract/exitcode"
)

func TestSchema_HealthSummary_JSON_BlockedIncludesStructuredResult(t *testing.T) {
	projectDir := t.TempDir()
	journalDir := filepath.Join(t.TempDir(), "journal")

	if err := os.MkdirAll(journalDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, "compose.yaml"), []byte("services: {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeDoctorEnvFile(t, projectDir, "dev", nil)
	for _, path := range []string{
		filepath.Join(projectDir, "runtime", "dev", "db"),
		filepath.Join(projectDir, "runtime", "dev", "espo"),
		filepath.Join(projectDir, "backups", "dev"),
	} {
		if err := os.MkdirAll(path, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	prependSupportBundleFakeDocker(t)

	outcome := executeCLIWithOptions(
		[]testAppOption{withFixedTestRuntime(time.Date(2026, 4, 19, 9, 0, 0, 0, time.UTC), "op-health-summary-blocked-1")},
		"--journal-dir", journalDir,
		"--json",
		"health-summary",
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

	if command := requireJSONPath(t, obj, "command"); command != "health-summary" {
		t.Fatalf("unexpected command: %v", command)
	}
	if ok, _ := requireJSONPath(t, obj, "ok").(bool); ok {
		t.Fatalf("expected ok=false")
	}
	if verdict := requireJSONPath(t, obj, "details", "verdict"); verdict != "blocked" {
		t.Fatalf("unexpected verdict: %v", verdict)
	}
	if code := requireJSONPath(t, obj, "error", "code"); code != "health_summary_blocked" {
		t.Fatalf("unexpected error code: %v", code)
	}
	if backupState := requireJSONPath(t, obj, "details", "backup_state"); backupState != "blocked" {
		t.Fatalf("unexpected backup state: %v", backupState)
	}

	items := requireJSONPath(t, obj, "items").([]any)
	if len(items) == 0 {
		t.Fatalf("expected blocking alerts, got %#v", obj["items"])
	}
	first, ok := items[0].(map[string]any)
	if !ok {
		t.Fatalf("expected alert object, got %#v", items[0])
	}
	if first["section"] != "backup" || first["severity"] != "blocking" {
		t.Fatalf("unexpected first alert payload: %#v", first)
	}
}

func TestSchema_HealthSummary_JSON_FailedIncludesSectionStatus(t *testing.T) {
	projectDir := t.TempDir()
	journalDir := filepath.Join(t.TempDir(), "journal")

	if err := os.MkdirAll(journalDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, "compose.yaml"), []byte("services: {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	prependDoctorFakeDocker(t)

	outcome := executeCLIWithOptions(
		[]testAppOption{withFixedTestRuntime(time.Date(2026, 4, 19, 9, 0, 0, 0, time.UTC), "op-health-summary-failed-1")},
		"--journal-dir", journalDir,
		"--json",
		"health-summary",
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

	if command := requireJSONPath(t, obj, "command"); command != "health-summary" {
		t.Fatalf("unexpected command: %v", command)
	}
	if verdict := requireJSONPath(t, obj, "details", "verdict"); verdict != "failed" {
		t.Fatalf("unexpected verdict: %v", verdict)
	}
	if code := requireJSONPath(t, obj, "error", "code"); code != "health_summary_failed" {
		t.Fatalf("unexpected error code: %v", code)
	}

	assertJSONListContains(t, obj, []string{"details", "omitted_sections"}, "runtime")
	assertJSONListContains(t, obj, []string{"details", "omitted_sections"}, "backup")
	assertJSONListContains(t, obj, []string{"details", "omitted_sections"}, "maintenance")

	sections := requireJSONPath(t, obj, "details", "section_results").([]any)
	if len(sections) != 5 {
		t.Fatalf("expected 5 section results, got %d", len(sections))
	}
}
