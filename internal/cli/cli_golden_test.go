package cli

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestGolden_VerifyBackup_JSON(t *testing.T) {
	tmp := t.TempDir()
	journalDir := filepath.Join(tmp, "journal")

	opts := []testAppOption{
		withFixedTestRuntime(time.Date(2026, 4, 15, 12, 0, 0, 0, time.UTC), "op-fixed-1"),
	}

	dbName := "db.sql.gz"
	filesName := "files.tar.gz"
	manifestPath := filepath.Join(tmp, "manifest.json")
	dbPath := filepath.Join(tmp, dbName)
	filesPath := filepath.Join(tmp, filesName)

	writeGzipFile(t, dbPath, []byte("select 1;"))
	writeTarGzFile(t, filesPath, map[string]string{
		"storage/a.txt": "hello",
	})

	writeJSON(t, manifestPath, map[string]any{
		"version":    1,
		"scope":      "prod",
		"created_at": "2026-04-15T11:00:00Z",
		"artifacts": map[string]any{
			"db_backup":    dbName,
			"files_backup": filesName,
		},
		"checksums": map[string]any{
			"db_backup":    sha256OfFile(t, dbPath),
			"files_backup": sha256OfFile(t, filesPath),
		},
	})

	out, err := runRootCommandWithOptions(t, opts,
		"--journal-dir", journalDir,
		"--json",
		"backup",
		"verify",
		"--manifest", manifestPath,
	)
	if err != nil {
		t.Fatalf("command failed: %v\noutput=%s", err, out)
	}

	normalized := normalizeVerifyBackupJSON(t, []byte(out))
	assertGoldenJSON(t, normalized, filepath.Join("testdata", "verify_backup_ok.golden.json"))
}

func normalizeVerifyBackupJSON(t *testing.T, raw []byte) []byte {
	t.Helper()

	var obj map[string]any
	if err := json.Unmarshal(raw, &obj); err != nil {
		t.Fatalf("invalid json output: %v\n%s", err, string(raw))
	}

	artifacts, _ := obj["artifacts"].(map[string]any)
	if artifacts != nil {
		for _, key := range []string{"manifest", "db_backup", "files_backup"} {
			if _, ok := artifacts[key]; ok {
				artifacts[key] = "REPLACE_AT_RUNTIME"
			}
		}
	}

	out, err := json.Marshal(obj)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func writeJSON(t *testing.T, path string, v any) {
	t.Helper()

	raw, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatal(err)
	}
}

func writeGzipFile(t *testing.T, path string, body []byte) {
	t.Helper()

	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer closeCLIArchiveResource(t, "golden gzip file", f)

	gz := gzip.NewWriter(f)
	if _, err := gz.Write(body); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
}

func writeTarGzFile(t *testing.T, path string, files map[string]string) {
	t.Helper()

	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer closeCLIArchiveResource(t, "golden tar archive file", f)

	gz := gzip.NewWriter(f)
	defer closeCLIArchiveResource(t, "golden tar archive gzip writer", gz)

	tw := tar.NewWriter(gz)
	defer closeCLIArchiveResource(t, "golden tar archive writer", tw)

	for name, body := range files {
		hdr := &tar.Header{
			Name: name,
			Mode: 0o644,
			Size: int64(len(body)),
		}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
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
