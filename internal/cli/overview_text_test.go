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
		"EspoCRM operator dashboard",
		"Included sections: context, doctor, runtime, latest_operation, backup",
		"Context:",
		"Contour: dev",
		"Doctor:",
		"Runtime:",
		"Latest Operation:",
		"Backup:",
		"Latest ready backup: espocrm-dev_2026-04-19_08-00-00",
		"restore  FAILED",
		"Action: Use show-operation --id op-restore-1",
		"Action: Use backup-catalog for inventory detail",
	} {
		if !strings.Contains(out, fragment) {
			t.Fatalf("expected output to contain %q\n%s", fragment, out)
		}
	}
}
