package cli

import (
	"path/filepath"
	"testing"
)

func TestGolden_StatusReport_JSON(t *testing.T) {
	fixture := prepareSupportBundleFixture(t, "dev")
	prependSupportBundleFakeDocker(t)

	out, err := runRootCommandWithOptions(
		t,
		[]testAppOption{withFixedTestRuntime(fixture.fixedNow, "op-status-report-1")},
		"--journal-dir", fixture.journalDir,
		"--json",
		"status-report",
		"--scope", "dev",
		"--project-dir", fixture.projectDir,
	)
	if err != nil {
		t.Fatalf("command failed: %v\noutput=%s", err, out)
	}

	normalized := normalizeStatusReportJSON(t, []byte(out), fixture)
	assertGoldenJSON(t, normalized, filepath.Join("testdata", "status_report_ok.golden.json"))
}
