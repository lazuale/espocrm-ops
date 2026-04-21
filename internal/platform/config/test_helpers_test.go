package config

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/lazuale/espocrm-ops/internal/testutil"
)

func baseOperationEnvValues(scope string) map[string]string {
	values := testutil.BaseEnvValues(scope)
	cloned := make(map[string]string, len(values))
	for key, value := range values {
		cloned[key] = value
	}

	return cloned
}

func writeOperationEnvValuesFile(t *testing.T, dir, name string, values map[string]string, perm os.FileMode) string {
	t.Helper()

	path := filepath.Join(dir, name)
	writeOperationEnvValuesPath(t, path, values, perm)
	return path
}

func writeOperationEnvValuesPath(t *testing.T, path string, values map[string]string, perm os.FileMode) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}

	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	lines := make([]string, 0, len(keys))
	for _, key := range keys {
		lines = append(lines, key+"="+values[key])
	}

	writeOperationEnvLinesPath(t, path, lines, perm)
}

func writeOperationEnvLinesFile(t *testing.T, dir, name string, lines []string, perm os.FileMode) string {
	t.Helper()

	path := filepath.Join(dir, name)
	writeOperationEnvLinesPath(t, path, lines, perm)
	return path
}

func writeOperationEnvLinesPath(t *testing.T, path string, lines []string, perm os.FileMode) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}

	content := strings.Join(lines, "\n")
	if len(lines) != 0 {
		content += "\n"
	}

	if err := os.WriteFile(path, []byte(content), perm); err != nil {
		t.Fatal(err)
	}
}
