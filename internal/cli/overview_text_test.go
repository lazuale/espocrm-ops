package cli

import (
	"strings"
	"testing"
)

func TestOverviewTextShowsCanonicalSectionSummary(t *testing.T) {
	fixture := prepareSupportBundleFixture(t, "dev")
	prependSupportBundleFakeDocker(t)

	out, err := runRootCommandWithOptions(
		t,
		[]testAppOption{withFixedTestRuntime(fixture.fixedNow, "op-overview-text-1")},
		"--journal-dir", fixture.journalDir,
		"overview",
		"--scope", "dev",
		"--project-dir", fixture.projectDir,
	)
	if err != nil {
		t.Fatalf("command failed: %v\noutput=%s", err, out)
	}

	for _, fragment := range []string{
		"EspoCRM contour overview",
		"Included sections: doctor, runtime, backup, recent_operations",
		"Doctor:",
		"Runtime:",
		"Backup:",
		"Recent Operations:",
		"Latest ready backup: espocrm-dev_2026-04-19_08-00-00",
		"backup  COMPLETED",
		"restore  FAILED",
	} {
		if !strings.Contains(out, fragment) {
			t.Fatalf("expected output to contain %q\n%s", fragment, out)
		}
	}
}
