package cli

import (
	"strings"
	"testing"
)

func TestText_StatusReport_IncludesCanonicalSections(t *testing.T) {
	fixture := prepareSupportBundleFixture(t, "dev")
	prependSupportBundleFakeDocker(t)

	out, err := runRootCommandWithOptions(
		t,
		[]testAppOption{withFixedTestRuntime(fixture.fixedNow, "op-status-report-text-1")},
		"--journal-dir", fixture.journalDir,
		"status-report",
		"--scope", "dev",
		"--project-dir", fixture.projectDir,
	)
	if err != nil {
		t.Fatalf("command failed: %v\noutput=%s", err, out)
	}

	for _, want := range []string{
		"EspoCRM status report",
		"Included sections: context, doctor, runtime, latest_operation, artifacts",
		"Context:",
		"Doctor:",
		"Runtime:",
		"Latest Operation:",
		"Artifacts:",
		"Latest ready backup: espocrm-dev_2026-04-19_08-00-00",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected output to contain %q\n%s", want, out)
		}
	}
}
