package cli

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGolden_Doctor_JSON(t *testing.T) {
	tmp := t.TempDir()
	projectDir := filepath.Join(tmp, "project")
	journalDir := filepath.Join(tmp, "journal")

	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, "compose.yaml"), []byte("services: {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeDoctorEnvFile(t, projectDir, "prod", nil)
	newDockerHarness(t)

	out, err := runRootCommand(
		t,
		"--journal-dir", journalDir,
		"--json",
		"doctor",
		"--scope", "prod",
		"--project-dir", projectDir,
	)
	if err != nil {
		t.Fatalf("command failed: %v\noutput=%s", err, out)
	}

	normalized := normalizeDoctorJSON(t, []byte(out))
	assertGoldenJSON(t, normalized, filepath.Join("testdata", "doctor_ok.golden.json"))
}
