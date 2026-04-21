package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lazuale/espocrm-ops/internal/testutil"
)

func TestConfigAdapterDoesNotReadProcessEnvironment(t *testing.T) {
	for _, forbidden := range []string{
		"os.Getenv(",
		"os.LookupEnv(",
		"os.Environ(",
		"os.Getwd(",
	} {
		assertConfigPackageTextAbsent(t, forbidden)
	}
}

func TestConfigAdapterDoesNotShellOutOrUseShellParsers(t *testing.T) {
	for _, forbidden := range []string{
		"exec.Command(",
		"godotenv",
		"\"sh\"",
		"\"-c\"",
	} {
		assertConfigPackageTextAbsent(t, forbidden)
	}
}

func TestConfigAdapterEnvFileChecksStayFailClosed(t *testing.T) {
	assertConfigPackageTextAbsent(t, "os.Stat(")
}

func assertConfigPackageTextAbsent(t *testing.T, needle string) {
	t.Helper()

	root := testutil.RepoRoot(t)
	configDir := filepath.Join(root, "internal", "platform", "config")

	err := filepath.WalkDir(configDir, func(path string, entry os.DirEntry, err error) error {
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
		if strings.Contains(string(raw), needle) {
			t.Fatalf("config adapter must not contain %s; found in %s", needle, path)
		}

		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
