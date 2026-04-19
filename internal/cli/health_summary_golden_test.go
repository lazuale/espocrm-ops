package cli

import (
	"path/filepath"
	"testing"
)

func TestGolden_HealthSummary_JSON(t *testing.T) {
	fixture := prepareSupportBundleFixture(t, "dev")
	prependSupportBundleFakeDocker(t)

	out, err := runRootCommandWithOptions(
		t,
		[]testAppOption{withFixedTestRuntime(fixture.fixedNow, "op-health-summary-1")},
		"--journal-dir", fixture.journalDir,
		"--json",
		"health-summary",
		"--scope", "dev",
		"--project-dir", fixture.projectDir,
	)
	if err != nil {
		t.Fatalf("command failed: %v\noutput=%s", err, out)
	}

	normalized := normalizeHealthSummaryJSON(t, []byte(out), fixture)
	assertGoldenJSON(t, normalized, filepath.Join("testdata", "health_summary_ok.golden.json"))
}
