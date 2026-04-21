package docker

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDockerMySQLAdapterDoesNotUseProcessStdIOOrWholeEnv(t *testing.T) {
	root := repoRootForArchitectureTest(t)
	path := filepath.Join(root, "internal", "platform", "docker", "mysql.go")

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)

	for _, forbidden := range []string{"os.Stdout", "os.Stderr", "os.Environ("} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("docker mysql adapter must not use %s directly", forbidden)
		}
	}
}

func repoRootForArchitectureTest(t *testing.T) string {
	t.Helper()

	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.mod not found")
		}
		dir = parent
	}
}
