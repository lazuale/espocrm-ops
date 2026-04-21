package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func decodeCLIJSON(t *testing.T, raw string) map[string]any {
	t.Helper()

	var obj map[string]any
	if err := json.Unmarshal([]byte(raw), &obj); err != nil {
		t.Fatalf("invalid json output: %v\n%s", err, raw)
	}

	return obj
}

func requireJSONPath(t *testing.T, obj map[string]any, path ...string) any {
	t.Helper()

	var cur any = obj
	for _, key := range path {
		m, ok := cur.(map[string]any)
		if !ok {
			t.Fatalf("path %s: %T is not an object", formatJSONPath(path), cur)
		}

		next, ok := m[key]
		if !ok {
			t.Fatalf("missing json path: %s", formatJSONPath(path))
		}
		cur = next
	}

	return cur
}

func requireJSONObject(t *testing.T, obj map[string]any, path ...string) map[string]any {
	t.Helper()

	value := requireJSONPath(t, obj, path...)
	nested, ok := value.(map[string]any)
	if !ok {
		t.Fatalf("json path %s: expected object, got %T", formatJSONPath(path), value)
	}
	return nested
}

func requireJSONArray(t *testing.T, obj map[string]any, path ...string) []any {
	t.Helper()

	value := requireJSONPath(t, obj, path...)
	array, ok := value.([]any)
	if !ok {
		t.Fatalf("json path %s: expected array, got %T", formatJSONPath(path), value)
	}
	return array
}

func requireJSONString(t *testing.T, obj map[string]any, path ...string) string {
	t.Helper()

	value := requireJSONPath(t, obj, path...)
	text, ok := value.(string)
	if !ok {
		t.Fatalf("json path %s: expected string, got %T", formatJSONPath(path), value)
	}
	return text
}

func requireJSONBool(t *testing.T, obj map[string]any, path ...string) bool {
	t.Helper()

	value := requireJSONPath(t, obj, path...)
	flag, ok := value.(bool)
	if !ok {
		t.Fatalf("json path %s: expected bool, got %T", formatJSONPath(path), value)
	}
	return flag
}

func requireJSONInt(t *testing.T, obj map[string]any, path ...string) int {
	t.Helper()

	value := requireJSONPath(t, obj, path...)
	number, ok := value.(float64)
	if !ok {
		t.Fatalf("json path %s: expected number, got %T", formatJSONPath(path), value)
	}
	return int(number)
}

func requireArtifactPathsExist(t *testing.T, obj map[string]any, keys ...string) {
	t.Helper()

	for _, key := range keys {
		path := requireJSONString(t, obj, "artifacts", key)
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("artifact %s missing at %s: %v", key, path, err)
		}
	}
}

func requireSameJSONValue(t *testing.T, first, second map[string]any, path ...string) any {
	t.Helper()

	left := requireJSONPath(t, first, path...)
	right := requireJSONPath(t, second, path...)
	if fmt.Sprint(left) != fmt.Sprint(right) {
		t.Fatalf("json path %s drifted: first=%v second=%v", formatJSONPath(path), left, right)
	}
	return right
}

func formatJSONPath(path []string) string {
	if len(path) == 0 {
		return "$"
	}
	return "$." + strings.Join(path, ".")
}

func mustFileMode(t *testing.T, path string) string {
	t.Helper()

	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	return fmt.Sprintf("%o", fi.Mode().Perm())
}

func stringsContainsAny(text string, parts ...string) bool {
	for _, part := range parts {
		if strings.Contains(text, part) {
			return true
		}
	}
	return false
}

func mustComposeFile(t *testing.T, projectDir string) string {
	t.Helper()

	path := filepath.Join(projectDir, "compose.yaml")
	if err := os.WriteFile(path, []byte("services: {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}
