package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/lazuale/espocrm-ops/internal/contract/exitcode"
)

func TestSchema_StatusReport_JSON_FailureIncludesFailedAndOmittedSections(t *testing.T) {
	projectDir := t.TempDir()
	journalDir := filepath.Join(t.TempDir(), "journal")

	if err := os.WriteFile(filepath.Join(projectDir, "compose.yaml"), []byte("services: {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	prependDoctorFakeDocker(t)

	outcome := executeCLIWithOptions(
		[]testAppOption{withFixedTestRuntime(time.Date(2026, 4, 19, 9, 0, 0, 0, time.UTC), "op-status-report-fail-1")},
		"--journal-dir", journalDir,
		"--json",
		"status-report",
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

	if command := requireJSONPath(t, obj, "command"); command != "status-report" {
		t.Fatalf("unexpected command: %v", command)
	}
	if ok, _ := requireJSONPath(t, obj, "ok").(bool); ok {
		t.Fatalf("expected ok=false")
	}
	if code := requireJSONPath(t, obj, "error", "code"); code != "status_report_failed" {
		t.Fatalf("unexpected error code: %v", code)
	}

	assertJSONListContains(t, obj, []string{"details", "failed_sections"}, "context")
	assertJSONListContains(t, obj, []string{"details", "failed_sections"}, "doctor")
	assertJSONListContains(t, obj, []string{"details", "omitted_sections"}, "runtime")
	assertJSONListContains(t, obj, []string{"details", "omitted_sections"}, "artifacts")

	items := requireJSONPath(t, obj, "items").([]any)
	if len(items) != 5 {
		t.Fatalf("expected 5 status-report sections, got %d", len(items))
	}
}
