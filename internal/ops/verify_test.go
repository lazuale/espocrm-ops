package ops

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestVerifyBackupValid(t *testing.T) {
	manifestPath, dbPath, filesPath := writeVerifiedBackupSet(t)

	result, err := VerifyBackup(context.Background(), manifestPath)
	if err != nil {
		t.Fatalf("VerifyBackup failed: %v", err)
	}
	if result.Manifest != manifestPath {
		t.Fatalf("unexpected manifest path: %s", result.Manifest)
	}
	if result.DBBackup != dbPath {
		t.Fatalf("unexpected db path: %s", result.DBBackup)
	}
	if result.FilesBackup != filesPath {
		t.Fatalf("unexpected files path: %s", result.FilesBackup)
	}
}

func TestVerifyBackupInvalidManifest(t *testing.T) {
	root := t.TempDir()
	manifestPath := filepath.Join(root, "manifests", "bad.manifest.json")
	if err := os.MkdirAll(filepath.Dir(manifestPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manifestPath, []byte("{"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := VerifyBackup(context.Background(), manifestPath)
	assertVerifyErrorKind(t, err, ErrorKindManifest)
}

func TestVerifyBackupManifestOutsideManifestsDirectory(t *testing.T) {
	root := t.TempDir()
	manifestPath := filepath.Join(root, "set.manifest.json")
	dbPath := filepath.Join(root, "db", "espocrm-prod_2026-04-24_12-00-00.sql.gz")
	filesPath := filepath.Join(root, "files", "espocrm-prod_files_2026-04-24_12-00-00.tar.gz")
	for _, dir := range []string{filepath.Dir(dbPath), filepath.Dir(filesPath)} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	writeGzipFile(t, dbPath, []byte("select 1;\n"))
	writeTarGzFile(t, filesPath, map[string]string{"storage/a.txt": "hello\n"})
	rewriteSidecar(t, dbPath)
	rewriteSidecar(t, filesPath)
	writeManifest(t, manifestPath, map[string]any{
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
	})

	_, err := VerifyBackup(context.Background(), manifestPath)
	assertVerifyErrorKind(t, err, ErrorKindManifest)
}

func TestVerifyBackupMissingArtifact(t *testing.T) {
	manifestPath, dbPath, _ := writeVerifiedBackupSet(t)
	if err := os.Remove(dbPath); err != nil {
		t.Fatal(err)
	}

	_, err := VerifyBackup(context.Background(), manifestPath)
	assertVerifyErrorKind(t, err, ErrorKindArtifact)
}

func TestVerifyBackupChecksumMismatch(t *testing.T) {
	manifestPath, dbPath, filesPath := writeVerifiedBackupSet(t)
	writeManifest(t, manifestPath, map[string]any{
		"version":    1,
		"scope":      "prod",
		"created_at": "2026-04-24T12:00:00Z",
		"artifacts": map[string]any{
			"db_backup":    filepath.Base(dbPath),
			"files_backup": filepath.Base(filesPath),
		},
		"checksums": map[string]any{
			"db_backup":    strings.Repeat("0", 64),
			"files_backup": sha256OfFile(t, filesPath),
		},
	})

	_, err := VerifyBackup(context.Background(), manifestPath)
	assertVerifyErrorKind(t, err, ErrorKindChecksum)
}

func TestVerifyBackupBrokenDBGzip(t *testing.T) {
	manifestPath, dbPath, filesPath := writeVerifiedBackupSet(t)
	if err := os.WriteFile(dbPath, []byte("not gzip"), 0o644); err != nil {
		t.Fatal(err)
	}
	rewriteSidecar(t, dbPath)
	writeManifest(t, manifestPath, map[string]any{
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
	})

	_, err := VerifyBackup(context.Background(), manifestPath)
	assertVerifyErrorKind(t, err, ErrorKindArchive)
}

func TestVerifyBackupBrokenTarGz(t *testing.T) {
	manifestPath, dbPath, filesPath := writeVerifiedBackupSet(t)
	writeGzipFile(t, filesPath, []byte("not a tar stream"))
	rewriteSidecar(t, filesPath)
	writeManifest(t, manifestPath, map[string]any{
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
	})

	_, err := VerifyBackup(context.Background(), manifestPath)
	assertVerifyErrorKind(t, err, ErrorKindArchive)
}

func TestVerifyBackupTarTraversal(t *testing.T) {
	manifestPath, dbPath, filesPath := writeVerifiedBackupSet(t)
	writeTraversalTarGzFile(t, filesPath)
	rewriteSidecar(t, filesPath)
	writeManifest(t, manifestPath, map[string]any{
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
	})

	_, err := VerifyBackup(context.Background(), manifestPath)
	assertVerifyErrorKind(t, err, ErrorKindArchive)
}

func assertVerifyErrorKind(t *testing.T, err error, want string) {
	t.Helper()
	if err == nil {
		t.Fatal("expected error")
	}
	verifyErr, ok := err.(*VerifyError)
	if !ok {
		t.Fatalf("expected VerifyError, got %T", err)
	}
	if verifyErr.Kind != want {
		t.Fatalf("expected kind %s, got %s", want, verifyErr.Kind)
	}
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
	writeManifest(t, manifestPath, map[string]any{
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
	})

	return manifestPath, dbPath, filesPath
}

func writeManifest(t *testing.T, path string, value any) {
	t.Helper()
	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(raw, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
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

func writeTraversalTarGzFile(t *testing.T, path string) {
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

	header := &tar.Header{
		Name: "../escape",
		Mode: 0o644,
		Size: int64(len("bad\n")),
	}
	if err := tarWriter.WriteHeader(header); err != nil {
		t.Fatal(err)
	}
	if _, err := tarWriter.Write([]byte("bad\n")); err != nil {
		t.Fatal(err)
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
