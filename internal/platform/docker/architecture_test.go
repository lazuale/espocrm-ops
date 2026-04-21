package docker

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lazuale/espocrm-ops/internal/testutil"
)

func TestDockerMySQLAdapterDoesNotUseProcessStdIOOrWholeEnv(t *testing.T) {
	root := testutil.RepoRoot(t)
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
