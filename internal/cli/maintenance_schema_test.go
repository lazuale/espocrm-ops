package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/lazuale/espocrm-ops/internal/contract/exitcode"
)

func TestSchema_Maintenance_JSON_FailureIncludesFailedAndOmittedSections(t *testing.T) {
	projectDir := t.TempDir()
	journalDir := filepath.Join(t.TempDir(), "journal")

	if err := os.WriteFile(filepath.Join(projectDir, "compose.yaml"), []byte("services: {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	outcome := executeCLIWithOptions(
		[]testAppOption{withFixedTestRuntime(time.Date(2026, 4, 19, 9, 0, 0, 0, time.UTC), "op-maintenance-fail-1")},
		"--journal-dir", journalDir,
		"--json",
		"maintenance",
		"--scope", "dev",
		"--project-dir", projectDir,
		"--unattended",
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

	if command := requireJSONPath(t, obj, "command"); command != "maintenance" {
		t.Fatalf("unexpected command: %v", command)
	}
	if ok, _ := requireJSONPath(t, obj, "ok").(bool); ok {
		t.Fatalf("expected ok=false")
	}
	if code := requireJSONPath(t, obj, "error", "code"); code != "maintenance_failed" {
		t.Fatalf("unexpected error code: %v", code)
	}
	if mode := requireJSONPath(t, obj, "details", "mode"); mode != "preview" {
		t.Fatalf("unexpected mode: %v", mode)
	}
	if unattended, _ := requireJSONPath(t, obj, "details", "unattended").(bool); !unattended {
		t.Fatalf("expected unattended=true")
	}
	if outcome := requireJSONPath(t, obj, "details", "outcome"); outcome != "blocked" {
		t.Fatalf("unexpected outcome: %v", outcome)
	}

	if failed := requireJSONPath(t, obj, "details", "failed_sections").([]any); len(failed) != 1 || failed[0] != "context" {
		t.Fatalf("unexpected failed sections: %#v", failed)
	}
	if omitted := requireJSONPath(t, obj, "details", "omitted_sections").([]any); len(omitted) != 4 {
		t.Fatalf("unexpected omitted sections: %#v", omitted)
	}

	items := requireJSONPath(t, obj, "items").([]any)
	if len(items) != 5 {
		t.Fatalf("expected 5 maintenance sections, got %d", len(items))
	}
}
