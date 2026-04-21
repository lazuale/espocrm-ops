package docker

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lazuale/espocrm-ops/internal/testutil"
)

func TestDockerAdapterLowLevelExecStaysInDockerGo(t *testing.T) {
	assertDockerPackageTextOwnership(t, "exec.Command(", map[string]struct{}{
		"docker.go": {},
	})
}

func TestDockerAdapterEnvFilteringStaysInDockerGo(t *testing.T) {
	assertDockerPackageTextOwnership(t, "dockerCommandEnv(", map[string]struct{}{
		"docker.go": {},
	})
	assertDockerPackageTextOwnership(t, "os.Environ(", map[string]struct{}{
		"docker.go": {},
	})
}

func TestDockerAdapterRawRunCommandStaysInDockerGo(t *testing.T) {
	assertDockerPackageTextOwnership(t, "runCommand(", map[string]struct{}{
		"docker.go": {},
	})
}

func TestDockerAdapterShellSeamsStayInStoragePermissions(t *testing.T) {
	assertDockerPackageTextOwnership(t, `"--entrypoint", "sh"`, map[string]struct{}{
		"storage_permissions.go": {},
	})
	assertDockerPackageTextOwnership(t, `"-euc"`, map[string]struct{}{
		"storage_permissions.go": {},
	})
}

func TestDockerAdapterDoesNotUseProcessStdIODirectly(t *testing.T) {
	root := testutil.RepoRoot(t)
	dockerDir := filepath.Join(root, "internal", "platform", "docker")

	err := filepath.WalkDir(dockerDir, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}

		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		text := string(raw)
		for _, forbidden := range []string{"os.Stdout", "os.Stderr"} {
			if strings.Contains(text, forbidden) {
				t.Fatalf("docker adapter must not use %s directly in %s", forbidden, path)
			}
		}

		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func assertDockerPackageTextOwnership(t *testing.T, needle string, owners map[string]struct{}) {
	t.Helper()

	root := testutil.RepoRoot(t)
	dockerDir := filepath.Join(root, "internal", "platform", "docker")

	err := filepath.WalkDir(dockerDir, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}

		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if !strings.Contains(string(raw), needle) {
			return nil
		}
		if _, ok := owners[filepath.Base(path)]; !ok {
			t.Fatalf("%s must stay owner-local to %v; found in %s", needle, ownerNames(owners), path)
		}

		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func ownerNames(owners map[string]struct{}) []string {
	names := make([]string, 0, len(owners))
	for name := range owners {
		names = append(names, name)
	}
	return names
}
