package cli

import "testing"

func TestUpdatePlan_Validation_RejectsUnsupportedScope(t *testing.T) {
	outcome := executeCLI(
		"--journal-dir", t.TempDir(),
		"--json",
		"update-plan",
		"--scope", "all",
	)

	assertUsageErrorOutput(t, outcome, "--scope must be dev or prod")
}
