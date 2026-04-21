package backup

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
	"time"

	backupverifyapp "github.com/lazuale/espocrm-ops/internal/app/backupverify"
	domainbackup "github.com/lazuale/espocrm-ops/internal/domain/backup"
	appadapter "github.com/lazuale/espocrm-ops/internal/platform/appadapter"
)

func TestVerify_OK(t *testing.T) {
	tmp := t.TempDir()

	dbName := "db_20260415_123456.sql.gz"
	filesName := "files_20260415_123456.tar.gz"

	dbPath := filepath.Join(tmp, dbName)
	filesPath := filepath.Join(tmp, filesName)
	manifestPath := filepath.Join(tmp, "manifest.json")

	writeGzipFile(t, dbPath, []byte("select 1;"))
	writeTarGz(t, filesPath, map[string]string{
		"storage/test.txt": "hello",
	})

	manifest := domainbackup.Manifest{
		Version:   1,
		Scope:     "prod",
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
		Artifacts: domainbackup.ManifestArtifacts{
			DBBackup:    dbName,
			FilesBackup: filesName,
		},
		Checksums: domainbackup.ManifestChecksums{
			DBBackup:    sha256OfFile(t, dbPath),
			FilesBackup: sha256OfFile(t, filesPath),
		},
	}
	writeManifest(t, manifestPath, manifest)

	_, err := testBackupVerifyService().Diagnose(backupverifyapp.Request{ManifestPath: manifestPath})
	if err != nil {
		t.Fatalf("expected verify to succeed, got: %v", err)
	}
}

func TestVerify_OK_FromManifestsDirLayout(t *testing.T) {
	tmp := t.TempDir()

	dbName := "db_20260415_123456.sql.gz"
	filesName := "files_20260415_123456.tar.gz"

	dbPath := filepath.Join(tmp, "db", dbName)
	filesPath := filepath.Join(tmp, "files", filesName)
	manifestPath := filepath.Join(tmp, "manifests", "manifest.json")

	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(filesPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(manifestPath), 0o755); err != nil {
		t.Fatal(err)
	}

	writeGzipFile(t, dbPath, []byte("select 1;"))
	writeTarGz(t, filesPath, map[string]string{
		"storage/test.txt": "hello",
	})

	manifest := domainbackup.Manifest{
		Version:   1,
		Scope:     "prod",
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
		Artifacts: domainbackup.ManifestArtifacts{
			DBBackup:    dbName,
			FilesBackup: filesName,
		},
		Checksums: domainbackup.ManifestChecksums{
			DBBackup:    sha256OfFile(t, dbPath),
			FilesBackup: sha256OfFile(t, filesPath),
		},
	}
	writeManifest(t, manifestPath, manifest)

	_, err := testBackupVerifyService().Diagnose(backupverifyapp.Request{ManifestPath: manifestPath})
	if err != nil {
		t.Fatalf("expected manifest-dir verify to succeed, got: %v", err)
	}
}

func TestVerify_FailsOnChecksumMismatch(t *testing.T) {
	tmp := t.TempDir()

	dbName := "db.sql.gz"
	filesName := "files.tar.gz"

	dbPath := filepath.Join(tmp, dbName)
	filesPath := filepath.Join(tmp, filesName)
	manifestPath := filepath.Join(tmp, "manifest.json")

	writeGzipFile(t, dbPath, []byte("select 1;"))
	writeTarGz(t, filesPath, map[string]string{
		"a.txt": "x",
	})

	manifest := domainbackup.Manifest{
		Version:   1,
		Scope:     "prod",
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
		Artifacts: domainbackup.ManifestArtifacts{
			DBBackup:    dbName,
			FilesBackup: filesName,
		},
		Checksums: domainbackup.ManifestChecksums{
			DBBackup:    stringsOf("deadbeef", 8),
			FilesBackup: sha256OfFile(t, filesPath),
		},
	}
	writeManifest(t, manifestPath, manifest)

	_, err := testBackupVerifyService().Diagnose(backupverifyapp.Request{ManifestPath: manifestPath})
	if err == nil {
		t.Fatal("expected checksum error")
	}
}

func TestVerify_FailsOnUnsafeTarEntry(t *testing.T) {
	tmp := t.TempDir()

	dbName := "db.sql.gz"
	filesName := "files.tar.gz"

	dbPath := filepath.Join(tmp, dbName)
	filesPath := filepath.Join(tmp, filesName)
	manifestPath := filepath.Join(tmp, "manifest.json")

	writeGzipFile(t, dbPath, []byte("select 1;"))
	writeUnsafeTarGz(t, filesPath)

	manifest := domainbackup.Manifest{
		Version:   1,
		Scope:     "prod",
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
		Artifacts: domainbackup.ManifestArtifacts{
			DBBackup:    dbName,
			FilesBackup: filesName,
		},
		Checksums: domainbackup.ManifestChecksums{
			DBBackup:    sha256OfFile(t, dbPath),
			FilesBackup: sha256OfFile(t, filesPath),
		},
	}
	writeManifest(t, manifestPath, manifest)

	_, err := testBackupVerifyService().Diagnose(backupverifyapp.Request{ManifestPath: manifestPath})
	if err == nil {
		t.Fatal("expected unsafe tar entry error")
	}
}

func TestVerify_FailsOnSymlinkEntry(t *testing.T) {
	tmp := t.TempDir()

	dbName := "db.sql.gz"
	filesName := "files.tar.gz"

	dbPath := filepath.Join(tmp, dbName)
	filesPath := filepath.Join(tmp, filesName)
	manifestPath := filepath.Join(tmp, "manifest.json")

	writeGzipFile(t, dbPath, []byte("select 1;"))
	writeSymlinkTarGz(t, filesPath)

	manifest := domainbackup.Manifest{
		Version:   1,
		Scope:     "prod",
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
		Artifacts: domainbackup.ManifestArtifacts{
			DBBackup:    dbName,
			FilesBackup: filesName,
		},
		Checksums: domainbackup.ManifestChecksums{
			DBBackup:    sha256OfFile(t, dbPath),
			FilesBackup: sha256OfFile(t, filesPath),
		},
	}
	writeManifest(t, manifestPath, manifest)

	_, err := testBackupVerifyService().Diagnose(backupverifyapp.Request{ManifestPath: manifestPath})
	if err == nil {
		t.Fatal("expected symlink rejection")
	}
}

func TestVerify_FailsOnIncompleteBackupPair(t *testing.T) {
	tmp := t.TempDir()

	dbName := "espocrm-prod_2026-04-15_12-34-56.sql.gz"
	filesName := "espocrm-prod_files_2026-04-15_12-34-56.tar.gz"

	dbPath := filepath.Join(tmp, "db", dbName)
	manifestPath := filepath.Join(tmp, "manifests", "espocrm-prod_2026-04-15_12-34-56.manifest.json")

	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(manifestPath), 0o755); err != nil {
		t.Fatal(err)
	}

	writeGzipFile(t, dbPath, []byte("select 1;"))
	writeManifest(t, manifestPath, domainbackup.Manifest{
		Version:   1,
		Scope:     "prod",
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
		Artifacts: domainbackup.ManifestArtifacts{
			DBBackup:    dbName,
			FilesBackup: filesName,
		},
		Checksums: domainbackup.ManifestChecksums{
			DBBackup:    sha256OfFile(t, dbPath),
			FilesBackup: stringsOf("ab", 32),
		},
	})

	if _, err := testBackupVerifyService().Diagnose(backupverifyapp.Request{ManifestPath: manifestPath}); err == nil {
		t.Fatal("expected incomplete backup pair to fail verification")
	}
}

func TestVerify_FailsOnCanonicalManifestArtifactMismatch(t *testing.T) {
	tmp := t.TempDir()

	dbName := "espocrm-prod_2026-04-15_12-34-56.sql.gz"
	filesName := "espocrm-prod_files_2026-04-16_12-34-56.tar.gz"
	dbPath := filepath.Join(tmp, "db", dbName)
	filesPath := filepath.Join(tmp, "files", filesName)
	manifestPath := filepath.Join(tmp, "manifests", "espocrm-prod_2026-04-15_12-34-56.manifest.json")

	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(filesPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(manifestPath), 0o755); err != nil {
		t.Fatal(err)
	}

	writeGzipFile(t, dbPath, []byte("select 1;"))
	writeTarGz(t, filesPath, map[string]string{
		"storage/test.txt": "hello",
	})
	writeManifest(t, manifestPath, domainbackup.Manifest{
		Version:   1,
		Scope:     "prod",
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
		Artifacts: domainbackup.ManifestArtifacts{
			DBBackup:    dbName,
			FilesBackup: filesName,
		},
		Checksums: domainbackup.ManifestChecksums{
			DBBackup:    sha256OfFile(t, dbPath),
			FilesBackup: sha256OfFile(t, filesPath),
		},
	})

	_, err := testBackupVerifyService().Diagnose(backupverifyapp.Request{ManifestPath: manifestPath})
	if err == nil {
		t.Fatal("expected canonical manifest mismatch to fail")
	}
	if !strings.Contains(err.Error(), "manifest backup set is inconsistent") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLoadManifest_FailsOnPathArtifact(t *testing.T) {
	tmp := t.TempDir()
	manifestPath := filepath.Join(tmp, "manifest.json")

	writeManifest(t, manifestPath, domainbackup.Manifest{
		Version:   1,
		Scope:     "prod",
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
		Artifacts: domainbackup.ManifestArtifacts{
			DBBackup:    "db/db.sql.gz",
			FilesBackup: "files.tar.gz",
		},
		Checksums: domainbackup.ManifestChecksums{
			DBBackup:    stringsOf("ab", 32),
			FilesBackup: stringsOf("cd", 32),
		},
	})

	if _, err := (appadapter.BackupStore{}).LoadManifest(manifestPath); err == nil {
		t.Fatal("expected path validation error")
	}
}

func writeManifest(t *testing.T, path string, m domainbackup.Manifest) {
	t.Helper()

	raw, err := json.MarshalIndent(m, "", "  ")
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
	defer closeTestArchiveResource(t, "gzip file", f)

	gz := gzip.NewWriter(f)
	if _, err := gz.Write(body); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
}

func writeTarGz(t *testing.T, path string, files map[string]string) {
	t.Helper()

	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer closeTestArchiveResource(t, "tar archive file", f)

	gz := gzip.NewWriter(f)
	defer closeTestArchiveResource(t, "tar archive gzip writer", gz)

	tw := tar.NewWriter(gz)
	defer closeTestArchiveResource(t, "tar archive writer", tw)

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

func writeUnsafeTarGz(t *testing.T, path string) {
	t.Helper()

	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer closeTestArchiveResource(t, "unsafe tar archive file", f)

	gz := gzip.NewWriter(f)
	defer closeTestArchiveResource(t, "unsafe tar archive gzip writer", gz)

	tw := tar.NewWriter(gz)
	defer closeTestArchiveResource(t, "unsafe tar archive writer", tw)

	hdr := &tar.Header{
		Name: "../escape.txt",
		Mode: 0o644,
		Size: int64(len("boom")),
	}
	if err := tw.WriteHeader(hdr); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write([]byte("boom")); err != nil {
		t.Fatal(err)
	}
}

func writeSymlinkTarGz(t *testing.T, path string) {
	t.Helper()

	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer closeTestArchiveResource(t, "symlink tar archive file", f)

	gz := gzip.NewWriter(f)
	defer closeTestArchiveResource(t, "symlink tar archive gzip writer", gz)

	tw := tar.NewWriter(gz)
	defer closeTestArchiveResource(t, "symlink tar archive writer", tw)

	hdr := &tar.Header{
		Name:     "link.txt",
		Typeflag: tar.TypeSymlink,
		Linkname: "/etc/passwd",
	}
	if err := tw.WriteHeader(hdr); err != nil {
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

func stringsOf(chunk string, count int) string {
	return strings.Repeat(chunk, count)
}

func closeTestArchiveResource(t *testing.T, label string, closer interface{ Close() error }) {
	t.Helper()

	if err := closer.Close(); err != nil {
		t.Fatalf("close %s: %v", label, err)
	}
}
