package cli

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBackupVerifyCLIJSONSuccess(t *testing.T) {
	manifestPath, dbPath, filesPath := writeVerifiedBackupSet(t)
	stdout := &strings.Builder{}
	stderr := &strings.Builder{}

	exitCode := Execute([]string{"backup", "verify", "--manifest", manifestPath}, stdout, stderr)
	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d stdout=%s stderr=%s", exitCode, stdout.String(), stderr.String())
	}

	var obj map[string]any
	if err := json.Unmarshal([]byte(stdout.String()), &obj); err != nil {
		t.Fatal(err)
	}
	if command := requireJSONString(t, obj, "command"); command != "backup verify" {
		t.Fatalf("unexpected command: %s", command)
	}
	if !requireJSONBool(t, obj, "ok") {
		t.Fatal("expected ok=true")
	}
	if message := requireJSONString(t, obj, "message"); message != "backup verified" {
		t.Fatalf("unexpected message: %s", message)
	}
	if errValue, exists := obj["error"]; !exists || errValue != nil {
		t.Fatalf("expected error=null, got %#v", errValue)
	}
	if manifest := requireJSONString(t, obj, "result", "manifest"); manifest != manifestPath {
		t.Fatalf("unexpected manifest: %s", manifest)
	}
	if dbBackup := requireJSONString(t, obj, "result", "db_backup"); dbBackup != dbPath {
		t.Fatalf("unexpected db_backup: %s", dbBackup)
	}
	if filesBackup := requireJSONString(t, obj, "result", "files_backup"); filesBackup != filesPath {
		t.Fatalf("unexpected files_backup: %s", filesBackup)
	}
	if stderr.Len() != 0 {
		t.Fatalf("expected empty stderr, got %q", stderr.String())
	}
}

func TestBackupVerifyCLIJSONMissingManifest(t *testing.T) {
	stdout := &strings.Builder{}
	stderr := &strings.Builder{}

	exitCode := Execute([]string{"backup", "verify"}, stdout, stderr)
	if exitCode != exitUsage {
		t.Fatalf("expected exit code %d, got %d stdout=%s stderr=%s", exitUsage, exitCode, stdout.String(), stderr.String())
	}

	var obj map[string]any
	if err := json.Unmarshal([]byte(stdout.String()), &obj); err != nil {
		t.Fatal(err)
	}
	if command := requireJSONString(t, obj, "command"); command != "backup verify" {
		t.Fatalf("unexpected command: %s", command)
	}
	if requireJSONBool(t, obj, "ok") {
		t.Fatal("expected ok=false")
	}
	if message := requireJSONString(t, obj, "message"); message != "backup verify failed" {
		t.Fatalf("unexpected message: %s", message)
	}
	if kind := requireJSONString(t, obj, "error", "kind"); kind != "usage" {
		t.Fatalf("unexpected error kind: %s", kind)
	}
	if errMessage := requireJSONString(t, obj, "error", "message"); errMessage != "--manifest is required" {
		t.Fatalf("unexpected error message: %s", errMessage)
	}
	if stderr.Len() != 0 {
		t.Fatalf("expected empty stderr, got %q", stderr.String())
	}
}

func requireJSONString(t *testing.T, obj map[string]any, path ...string) string {
	t.Helper()
	raw := requireJSONPath(t, obj, path...)
	value, ok := raw.(string)
	if !ok {
		t.Fatalf("expected string at %v, got %#v", path, raw)
	}
	return value
}

func requireJSONBool(t *testing.T, obj map[string]any, path ...string) bool {
	t.Helper()
	raw := requireJSONPath(t, obj, path...)
	value, ok := raw.(bool)
	if !ok {
		t.Fatalf("expected bool at %v, got %#v", path, raw)
	}
	return value
}

func requireJSONPath(t *testing.T, obj map[string]any, path ...string) any {
	t.Helper()
	current := any(obj)
	for _, part := range path {
		m, ok := current.(map[string]any)
		if !ok {
			t.Fatalf("expected object before %q in %v, got %#v", part, path, current)
		}
		next, ok := m[part]
		if !ok {
			t.Fatalf("missing path %v", path)
		}
		current = next
	}
	return current
}

func writeVerifiedBackupSet(t *testing.T) (manifestPath, dbPath, filesPath string) {
	t.Helper()

	root := t.TempDir()
	dbPath = filepath.Join(root, "db", "espocrm-prod_2026-04-24_12-00-00.sql.gz")
	filesPath = filepath.Join(root, "files", "espocrm-prod_files_2026-04-24_12-00-00.tar.gz")
	manifestPath = filepath.Join(root, "manifests", "espocrm-prod_2026-04-24_12-00-00.manifest.json")

	for _, dir := range []string{filepath.Dir(dbPath), filepath.Dir(filesPath), filepath.Dir(manifestPath)} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}

	writeGzipFile(t, dbPath, []byte("select 1;\n"))
	writeTarGzFile(t, filesPath, map[string]string{"storage/a.txt": "hello\n"})
	rewriteSidecar(t, dbPath)
	rewriteSidecar(t, filesPath)

	raw, err := json.MarshalIndent(map[string]any{
		"version":    1,
		"scope":      "prod",
		"created_at": "2026-04-24T12:00:00Z",
		"artifacts": map[string]any{
			"db_backup":    filepath.Base(dbPath),
			"files_backup": filepath.Base(filesPath),
		},
		"checksums": map[string]any{
			"db_backup":    sha256OfFile(t, dbPath),
			"files_backup": sha256OfFile(t, filesPath),
		},
	}, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manifestPath, append(raw, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}

	return manifestPath, dbPath, filesPath
}

func writeGzipFile(t *testing.T, path string, body []byte) {
	t.Helper()
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer closeTestResource(t, file)

	writer := gzip.NewWriter(file)
	if _, err := writer.Write(body); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
}

func writeTarGzFile(t *testing.T, path string, files map[string]string) {
	t.Helper()
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer closeTestResource(t, file)

	gzipWriter := gzip.NewWriter(file)
	defer closeTestResource(t, gzipWriter)

	tarWriter := tar.NewWriter(gzipWriter)
	defer closeTestResource(t, tarWriter)

	for name, body := range files {
		header := &tar.Header{
			Name: name,
			Mode: 0o644,
			Size: int64(len(body)),
		}
		if err := tarWriter.WriteHeader(header); err != nil {
			t.Fatal(err)
		}
		if _, err := tarWriter.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
}

func rewriteSidecar(t *testing.T, path string) {
	t.Helper()
	body := sha256OfFile(t, path) + "  " + filepath.Base(path) + "\n"
	if err := os.WriteFile(path+".sha256", []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func sha256OfFile(t *testing.T, path string) string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func closeTestResource(t *testing.T, closer interface{ Close() error }) {
	t.Helper()
	if err := closer.Close(); err != nil {
		t.Fatalf("close resource: %v", err)
	}
}
