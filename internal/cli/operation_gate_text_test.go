package cli

import (
	"strings"
	"testing"
)

func TestText_OperationGate_RestoreDrill(t *testing.T) {
	fixture := prepareSupportBundleFixture(t, "dev")
	prependSupportBundleFakeDocker(t)

	out, err := runRootCommandWithOptions(
		t,
		[]testAppOption{withFixedTestRuntime(fixture.fixedNow, "op-operation-gate-text-1")},
		"--journal-dir", fixture.journalDir,
		"operation-gate",
		"--action", "restore-drill",
		"--scope", "dev",
		"--project-dir", fixture.projectDir,
	)
	if err != nil {
		t.Fatalf("command failed: %v\noutput=%s", err, out)
	}

	for _, fragment := range []string{
		"EspoCRM operation gate",
		"Action: restore-drill",
		"Decision",
		"Restore-drill preflight",
		"Alerts:",
	} {
		if !strings.Contains(out, fragment) {
			t.Fatalf("expected output to contain %q\n%s", fragment, out)
		}
	}
}
