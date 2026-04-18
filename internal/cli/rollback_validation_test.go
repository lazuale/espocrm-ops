package cli

import (
	"encoding/json"
	"testing"

	"github.com/lazuale/espocrm-ops/internal/contract/exitcode"
)

func TestRollback_Validation_RequiresForceForExecute(t *testing.T) {
	outcome := executeCLI(
		"--json",
		"rollback",
		"--scope", "dev",
		"--project-dir", t.TempDir(),
	)

	assertUsageErrorOutput(t, outcome, "rollback requires an explicit --force flag")
}

func TestRollback_Validation_RequiresProdConfirmationForExecute(t *testing.T) {
	outcome := executeCLI(
		"--json",
		"rollback",
		"--scope", "prod",
		"--project-dir", t.TempDir(),
		"--force",
	)

	assertUsageErrorOutput(t, outcome, "prod rollback also requires --confirm-prod prod")
}

func TestRollback_Validation_DryRunDoesNotRequireForce(t *testing.T) {
	outcome := executeCLI(
		"--json",
		"rollback",
		"--scope", "dev",
		"--project-dir", t.TempDir(),
		"--dry-run",
	)

	if outcome.ExitCode == exitcode.UsageError {
		t.Fatalf("expected dry-run to bypass destructive guardrails, got usage error stdout=%s stderr=%s", outcome.Stdout, outcome.Stderr)
	}

	var obj map[string]any
	if err := json.Unmarshal([]byte(outcome.Stdout), &obj); err != nil {
		t.Fatal(err)
	}
	if code := requireJSONPath(t, obj, "error", "code"); code == "usage_error" {
		t.Fatalf("expected non-usage error for dry-run, got %#v", obj)
	}
}
