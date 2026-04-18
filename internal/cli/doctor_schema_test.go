package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/lazuale/espocrm-ops/internal/contract/exitcode"
)

func TestSchema_Doctor_JSON_FailureIncludesDetailedResult(t *testing.T) {
	tmp := t.TempDir()
	projectDir := filepath.Join(tmp, "project")
	journalDir := filepath.Join(tmp, "journal")

	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, "compose.yaml"), []byte("services: {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeDoctorEnvFile(t, projectDir, "prod", map[string]string{
		"APP_PORT": "18080",
		"WS_PORT":  "18080",
	})
	prependDoctorFakeDocker(t)

	outcome := executeCLI(
		"--journal-dir", journalDir,
		"--json",
		"doctor",
		"--scope", "prod",
		"--project-dir", projectDir,
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

	if command := requireJSONPath(t, obj, "command"); command != "doctor" {
		t.Fatalf("unexpected command: %v", command)
	}
	if ok, _ := requireJSONPath(t, obj, "ok").(bool); ok {
		t.Fatalf("expected ok=false")
	}
	if code := requireJSONPath(t, obj, "error", "code"); code != "doctor_failed" {
		t.Fatalf("unexpected error code: %v", code)
	}
	if kind := requireJSONPath(t, obj, "error", "kind"); kind != "validation" {
		t.Fatalf("unexpected error kind: %v", kind)
	}
	exitCode, _ := requireJSONPath(t, obj, "error", "exit_code").(float64)
	if int(exitCode) != exitcode.ValidationError {
		t.Fatalf("unexpected error exit code: %v", exitCode)
	}

	items, ok := obj["items"].([]any)
	if !ok || len(items) == 0 {
		t.Fatalf("expected non-empty doctor items, got %#v", obj["items"])
	}

	foundFailure := false
	for _, rawItem := range items {
		item, ok := rawItem.(map[string]any)
		if !ok {
			continue
		}
		if item["code"] == "env_contract" && item["status"] == "fail" {
			foundFailure = true
			break
		}
	}
	if !foundFailure {
		t.Fatalf("expected env_contract failure in %#v", items)
	}

	assertNoJournalFiles(t, journalDir)
}
