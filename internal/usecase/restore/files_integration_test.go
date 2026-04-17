package restore

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

	domainbackup "github.com/lazuale/espocrm-ops/internal/domain/backup"
)

func TestRestoreFiles_Integration(t *testing.T) {
	tmp := t.TempDir()

	dbName := "db_20260415_123456.sql.gz"
	filesName := "files_20260415_123456.tar.gz"
	dbPath := filepath.Join(tmp, "db", dbName)
	filesPath := filepath.Join(tmp, "files", filesName)
	manifestPath := filepath.Join(tmp, "manifests", "manifest.json")
	targetDir := filepath.Join(tmp, "storage")

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
		"avatars/user1.txt": "hello",
		"documents/a.txt":   "world",
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

	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(targetDir, "stale.txt"), []byte("stale"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := RestoreFiles(RestoreFilesRequest{
		ManifestPath: manifestPath,
		TargetDir:    targetDir,
	})
	if err != nil {
		t.Fatalf("RestoreFiles failed: %v", err)
	}

	mustContain(t, filepath.Join(targetDir, "avatars", "user1.txt"), "hello")
	mustContain(t, filepath.Join(targetDir, "documents", "a.txt"), "world")

	if _, err := os.Stat(filepath.Join(targetDir, "stale.txt")); !os.IsNotExist(err) {
		t.Fatalf("expected stale file removed, got: %v", err)
	}
}

func TestRestoreFiles_RejectsSymlinkEntry(t *testing.T) {
	tmp := t.TempDir()

	dbName := "db_20260415_123456.sql.gz"
	filesName := "files_20260415_123456.tar.gz"
	dbPath := filepath.Join(tmp, dbName)
	filesPath := filepath.Join(tmp, filesName)
	manifestPath := filepath.Join(tmp, "manifest.json")
	targetDir := filepath.Join(tmp, "storage")

	writeGzipFile(t, dbPath, []byte("select 1;"))
	writeSymlinkTarGz(t, filesPath)
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

	_, err := RestoreFiles(RestoreFilesRequest{
		ManifestPath: manifestPath,
		TargetDir:    targetDir,
	})
	if err == nil {
		t.Fatal("expected symlink rejection")
	}
}

func TestRestoreFiles_DryRunDoesNotReplaceTarget(t *testing.T) {
	tmp := t.TempDir()

	dbName := "db_20260415_123456.sql.gz"
	filesName := "files_20260415_123456.tar.gz"
	dbPath := filepath.Join(tmp, dbName)
	filesPath := filepath.Join(tmp, filesName)
	manifestPath := filepath.Join(tmp, "manifest.json")
	targetDir := filepath.Join(tmp, "storage")

	writeGzipFile(t, dbPath, []byte("select 1;"))
	writeTarGz(t, filesPath, map[string]string{
		"avatars/user1.txt": "hello",
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

	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(targetDir, "stale.txt"), []byte("stale"), 0o644); err != nil {
		t.Fatal(err)
	}

	plan, err := RestoreFiles(RestoreFilesRequest{
		ManifestPath: manifestPath,
		TargetDir:    targetDir,
		DryRun:       true,
	})
	if err != nil {
		t.Fatalf("RestoreFiles dry-run failed: %v", err)
	}
	if plan.Plan.SourceKind != RestoreSourceManifest {
		t.Fatalf("unexpected source kind: %s", plan.Plan.SourceKind)
	}
	if plan.Plan.SourcePath != filesPath {
		t.Fatalf("unexpected source path: %s", plan.Plan.SourcePath)
	}

	mustContain(t, filepath.Join(targetDir, "stale.txt"), "stale")
	if _, err := os.Stat(filepath.Join(targetDir, "avatars", "user1.txt")); !os.IsNotExist(err) {
		t.Fatalf("dry-run should not restore files, got: %v", err)
	}
}

func TestRestoreFiles_DirectArchive(t *testing.T) {
	tmp := t.TempDir()

	filesPath := filepath.Join(tmp, "files.tar.gz")
	targetDir := filepath.Join(tmp, "storage")

	writeTarGz(t, filesPath, map[string]string{
		"storage/restored.txt": "hello",
	})
	writeSHA256Sidecar(t, filesPath)

	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(targetDir, "stale.txt"), []byte("stale"), 0o644); err != nil {
		t.Fatal(err)
	}

	plan, err := RestoreFiles(RestoreFilesRequest{
		FilesBackup: filesPath,
		TargetDir:   targetDir,
	})
	if err != nil {
		t.Fatalf("RestoreFiles direct archive failed: %v", err)
	}
	if plan.Plan.SourceKind != RestoreSourceDirectBackup {
		t.Fatalf("unexpected source kind: %s", plan.Plan.SourceKind)
	}
	if plan.Plan.SourcePath != filesPath {
		t.Fatalf("unexpected source path: %s", plan.Plan.SourcePath)
	}

	mustContain(t, filepath.Join(targetDir, "restored.txt"), "hello")
	if _, err := os.Stat(filepath.Join(targetDir, "stale.txt")); !os.IsNotExist(err) {
		t.Fatalf("expected stale file removed, got: %v", err)
	}
}

func TestRestoreFiles_DirectArchiveRequiresSidecar(t *testing.T) {
	tmp := t.TempDir()

	filesPath := filepath.Join(tmp, "files.tar.gz")
	targetDir := filepath.Join(tmp, "storage")

	writeTarGz(t, filesPath, map[string]string{
		"storage/restored.txt": "hello",
	})

	_, err := RestoreFiles(RestoreFilesRequest{
		FilesBackup: filesPath,
		TargetDir:   targetDir,
		DryRun:      true,
	})
	if err == nil {
		t.Fatal("expected missing checksum sidecar to fail")
	}
}

func TestRestoreFiles_RejectsUnsafeTargetDirBeforePreflight(t *testing.T) {
	for _, targetDir := range []string{"   ", ".", string(os.PathSeparator)} {
		_, err := RestoreFiles(RestoreFilesRequest{
			ManifestPath: "missing-manifest.json",
			TargetDir:    targetDir,
			DryRun:       true,
		})
		if err == nil {
			t.Fatalf("expected unsafe target dir %q to be rejected", targetDir)
		}
	}
}

func mustContain(t *testing.T, path, want string) {
	t.Helper()

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != want {
		t.Fatalf("unexpected content in %s: got %q want %q", path, string(raw), want)
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
	defer f.Close()

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
	defer f.Close()

	gz := gzip.NewWriter(f)
	defer gz.Close()

	tw := tar.NewWriter(gz)
	defer tw.Close()

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

func writeSymlinkTarGz(t *testing.T, path string) {
	t.Helper()

	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	gz := gzip.NewWriter(f)
	defer gz.Close()

	tw := tar.NewWriter(gz)
	defer tw.Close()

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

func writeSHA256Sidecar(t *testing.T, path string) {
	t.Helper()

	body := sha256OfFile(t, path) + "  " + filepath.Base(path) + "\n"
	if err := os.WriteFile(path+".sha256", []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}
