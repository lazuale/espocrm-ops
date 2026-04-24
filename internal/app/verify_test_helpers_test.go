package app_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func mustMkdirAll(t *testing.T, paths ...string) {
	t.Helper()

	for _, path := range paths {
		if err := os.MkdirAll(path, 0o755); err != nil {
			t.Fatal(err)
		}
	}
}

func mustWriteFile(t *testing.T, path, body string) {
	t.Helper()

	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func writeJSONFileForVerify(t *testing.T, path string, value any) {
	t.Helper()

	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(raw, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
}

func replaceStringValues(value any, old, new string) {
	switch typed := value.(type) {
	case map[string]any:
		for key, nested := range typed {
			if text, ok := nested.(string); ok {
				typed[key] = strings.ReplaceAll(text, old, new)
				continue
			}
			replaceStringValues(nested, old, new)
		}
	case []any:
		for idx, nested := range typed {
			if text, ok := nested.(string); ok {
				typed[idx] = strings.ReplaceAll(text, old, new)
				continue
			}
			replaceStringValues(nested, old, new)
		}
	}
}

func sha256OfPath(t *testing.T, path string) string {
	t.Helper()

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func normalizeJSONBytesForVerify(t *testing.T, raw []byte) []byte {
	t.Helper()

	var obj any
	if err := json.Unmarshal(raw, &obj); err != nil {
		t.Fatalf("invalid json: %v\n%s", err, string(raw))
	}
	out, err := json.MarshalIndent(obj, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	return out
}
