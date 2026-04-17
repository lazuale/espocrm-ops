package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/lazuale/espocrm-ops/internal/contract/exitcode"
)

func TestSchema_ShowOperation_JSON_Error(t *testing.T) {
	tmp := t.TempDir()
	journalDir := filepath.Join(tmp, "journal")
	if err := os.MkdirAll(journalDir, 0o755); err != nil {
		t.Fatal(err)
	}

	outcome := executeCLI(
		"--journal-dir", journalDir,
		"--json",
		"show-operation",
		"--id", "missing-op",
	)

	if outcome.ExitCode != exitcode.ValidationError {
		t.Fatalf("expected validation exit code, got %d stdout=%s stderr=%s", outcome.ExitCode, outcome.Stdout, outcome.Stderr)
	}

	var obj map[string]any
	if err := json.Unmarshal([]byte(outcome.Stdout), &obj); err != nil {
		t.Fatal(err)
	}

	requireJSONPath(t, obj, "command")
	requireJSONPath(t, obj, "ok")
	requireJSONPath(t, obj, "error", "code")
	requireJSONPath(t, obj, "error", "kind")
	requireJSONPath(t, obj, "error", "exit_code")
	requireJSONPath(t, obj, "error", "message")

	if ok, _ := obj["ok"].(bool); ok {
		t.Fatalf("expected ok=false")
	}
	if code := requireJSONPath(t, obj, "error", "code"); code != "operation_not_found" {
		t.Fatalf("expected operation_not_found, got %v", code)
	}
	if kind := requireJSONPath(t, obj, "error", "kind"); kind != "not_found" {
		t.Fatalf("expected not_found kind, got %v", kind)
	}
	exitCode, _ := requireJSONPath(t, obj, "error", "exit_code").(float64)
	if int(exitCode) != exitcode.ValidationError {
		t.Fatalf("expected json error exit_code %d, got %v", exitcode.ValidationError, exitCode)
	}
}
