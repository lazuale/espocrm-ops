package cli

import (
	"path/filepath"
	"testing"
)

func TestGolden_SupportBundle_JSON(t *testing.T) {
	fixture := prepareSupportBundleFixture(t, "dev")
	prependSupportBundleFakeDocker(t)

	outputPath := filepath.Join(t.TempDir(), "support-bundle-golden.tar.gz")
	outcome := executeCLIWithOptions(
		[]testAppOption{withFixedTestRuntime(fixture.fixedNow, "op-support-bundle-1")},
		"--journal-dir", fixture.journalDir,
		"--json",
		"support-bundle",
		"--scope", "dev",
		"--project-dir", fixture.projectDir,
		"--output", outputPath,
		"--tail", "42",
	)
	if outcome.ExitCode != 0 {
		t.Fatalf("command failed\nstdout=%s\nstderr=%s", outcome.Stdout, outcome.Stderr)
	}

	normalized := normalizeSupportBundleJSON(t, []byte(outcome.Stdout))
	assertGoldenJSON(t, normalized, filepath.Join("testdata", "support_bundle_ok.golden.json"))
}
