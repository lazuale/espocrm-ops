package cli

import (
	"path/filepath"
	"testing"
)

func TestGolden_OperationGate_JSON(t *testing.T) {
	fixture := prepareSupportBundleFixture(t, "dev")
	prependSupportBundleFakeDocker(t)

	out, err := runRootCommandWithOptions(
		t,
		[]testAppOption{withFixedTestRuntime(fixture.fixedNow, "op-operation-gate-1")},
		"--journal-dir", fixture.journalDir,
		"--json",
		"operation-gate",
		"--action", "restore-drill",
		"--scope", "dev",
		"--project-dir", fixture.projectDir,
	)
	if err != nil {
		t.Fatalf("command failed: %v\noutput=%s", err, out)
	}

	normalized := normalizeOperationGateJSON(t, []byte(out), fixture)
	assertGoldenJSON(t, normalized, filepath.Join("testdata", "operation_gate_ok.golden.json"))
}
