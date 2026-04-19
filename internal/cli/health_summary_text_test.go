package cli

import (
	"strings"
	"testing"
)

func TestText_HealthSummary_Degraded(t *testing.T) {
	fixture := prepareSupportBundleFixture(t, "dev")
	prependSupportBundleFakeDocker(t)

	out, err := runRootCommandWithOptions(
		t,
		[]testAppOption{withFixedTestRuntime(fixture.fixedNow, "op-health-summary-text-1")},
		"--journal-dir", fixture.journalDir,
		"health-summary",
		"--scope", "dev",
		"--project-dir", fixture.projectDir,
	)
	if err != nil {
		t.Fatalf("command failed: %v\noutput=%s", err, out)
	}

	for _, needle := range []string{
		"EspoCRM health summary",
		"Verdict: degraded",
		"Sections:",
		"Alerts:",
		"[DEGRADED][INCLUDED] Latest operation: restore (failed)",
		"[WARNING][LATEST OPERATION] Latest restore operation is failed",
	} {
		if !strings.Contains(out, needle) {
			t.Fatalf("expected output to contain %q, got:\n%s", needle, out)
		}
	}
}
