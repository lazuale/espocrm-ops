package cli

import (
	"path/filepath"
	"testing"
)

func TestGolden_Overview_JSON(t *testing.T) {
	fixture := prepareSupportBundleFixture(t, "dev")
	prependSupportBundleFakeDocker(t)

	out, err := runRootCommandWithOptions(
		t,
		[]testAppOption{withFixedTestRuntime(fixture.fixedNow, "op-overview-1")},
		"--journal-dir", fixture.journalDir,
		"--json",
		"overview",
		"--scope", "dev",
		"--project-dir", fixture.projectDir,
	)
	if err != nil {
		t.Fatalf("command failed: %v\noutput=%s", err, out)
	}

	normalized := normalizeOverviewJSON(t, []byte(out), fixture)
	assertGoldenJSON(t, normalized, filepath.Join("testdata", "overview_ok.golden.json"))
}
