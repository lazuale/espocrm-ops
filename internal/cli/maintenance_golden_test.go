package cli

import (
	"path/filepath"
	"testing"
)

func TestGolden_Maintenance_JSON(t *testing.T) {
	fixture := prepareMaintenanceFixture(t, "dev")
	useJournalClockForTest(t, fixture.fixedNow)

	out, err := runRootCommandWithOptions(
		t,
		[]testAppOption{withFixedTestRuntime(fixture.fixedNow, "op-maintenance-1")},
		"--journal-dir", fixture.journalDir,
		"--json",
		"maintenance",
		"--scope", "dev",
		"--project-dir", fixture.projectDir,
		"--journal-keep-days", "30",
		"--journal-keep-last", "2",
		"--unattended",
	)
	if err != nil {
		t.Fatalf("command failed: %v\noutput=%s", err, out)
	}

	normalized := normalizeMaintenanceJSON(t, []byte(out), fixture)
	assertGoldenJSON(t, normalized, filepath.Join("testdata", "maintenance_ok.golden.json"))
}
