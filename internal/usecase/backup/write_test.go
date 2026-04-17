package backup

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	domainbackup "github.com/lazuale/espocrm-ops/internal/domain/backup"
)

func TestBuildManifest_HashesArtifacts(t *testing.T) {
	tmp := t.TempDir()
	dbPath := filepath.Join(tmp, "db", "espocrm-dev_2026-04-07_01-02-03.sql.gz")
	filesPath := filepath.Join(tmp, "files", "espocrm-dev_files_2026-04-07_01-02-03.tar.gz")

	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(filesPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dbPath, []byte("db"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filesPath, []byte("files"), 0o644); err != nil {
		t.Fatal(err)
	}

	createdAt := time.Date(2026, 4, 7, 1, 2, 3, 0, time.FixedZone("test", 3*60*60))
	manifest, err := BuildManifest(ManifestBuildRequest{
		Scope:           " dev ",
		CreatedAt:       createdAt,
		DBBackupPath:    dbPath,
		FilesBackupPath: filesPath,
	})
	if err != nil {
		t.Fatal(err)
	}

	if manifest.Scope != "dev" {
		t.Fatalf("unexpected scope: %s", manifest.Scope)
	}
	if manifest.CreatedAt != "2026-04-06T22:02:03Z" {
		t.Fatalf("unexpected created_at: %s", manifest.CreatedAt)
	}
	if manifest.Artifacts.DBBackup != filepath.Base(dbPath) {
		t.Fatalf("unexpected db artifact: %s", manifest.Artifacts.DBBackup)
	}
	if manifest.Artifacts.FilesBackup != filepath.Base(filesPath) {
		t.Fatalf("unexpected files artifact: %s", manifest.Artifacts.FilesBackup)
	}
	if manifest.Checksums.DBBackup != sha256OfFile(t, dbPath) {
		t.Fatalf("unexpected db checksum: %s", manifest.Checksums.DBBackup)
	}
	if manifest.Checksums.FilesBackup != sha256OfFile(t, filesPath) {
		t.Fatalf("unexpected files checksum: %s", manifest.Checksums.FilesBackup)
	}
}

func TestWriteManifest_WritesValidatedJSON(t *testing.T) {
	tmp := t.TempDir()
	manifestPath := filepath.Join(tmp, "manifests", "backup.manifest.json")
	manifest := domainbackup.Manifest{
		Version:   1,
		Scope:     "prod",
		CreatedAt: "2026-04-07T01:02:03Z",
		Artifacts: domainbackup.ManifestArtifacts{
			DBBackup:    "db.sql.gz",
			FilesBackup: "files.tar.gz",
		},
		Checksums: domainbackup.ManifestChecksums{
			DBBackup:    strings.Repeat("a", 64),
			FilesBackup: strings.Repeat("b", 64),
		},
	}

	if err := WriteManifest(manifestPath, manifest); err != nil {
		t.Fatal(err)
	}

	loaded, err := LoadManifest(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Artifacts.DBBackup != "db.sql.gz" {
		t.Fatalf("unexpected loaded manifest: %#v", loaded)
	}
}

func TestWriteSHA256Sidecar(t *testing.T) {
	tmp := t.TempDir()
	filePath := filepath.Join(tmp, "db.sql.gz")
	sidecarPath := filepath.Join(tmp, "sidecars", "db.sql.gz.sha256")
	checksum := strings.Repeat("A", 64)

	if err := os.WriteFile(filePath, []byte("db"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := WriteSHA256Sidecar(filePath, checksum, sidecarPath); err != nil {
		t.Fatal(err)
	}

	raw, err := os.ReadFile(sidecarPath)
	if err != nil {
		t.Fatal(err)
	}
	want := strings.ToLower(checksum) + "  db.sql.gz\n"
	if string(raw) != want {
		t.Fatalf("unexpected sidecar: got %q want %q", string(raw), want)
	}
}

func TestFinalizeBackup_WritesSidecarsAndManifest(t *testing.T) {
	tmp := t.TempDir()
	dbPath := filepath.Join(tmp, "db", "espocrm-dev_2026-04-07_01-02-03.sql.gz")
	filesPath := filepath.Join(tmp, "files", "espocrm-dev_files_2026-04-07_01-02-03.tar.gz")
	manifestPath := filepath.Join(tmp, "manifests", "espocrm-dev_2026-04-07_01-02-03.manifest.json")

	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(filesPath), 0o755); err != nil {
		t.Fatal(err)
	}
	writeGzipFile(t, dbPath, []byte("select 1;"))
	writeTarGz(t, filesPath, map[string]string{
		"storage/a.txt": "hello",
	})

	info, err := FinalizeBackup(FinalizeRequest{
		Scope:            "dev",
		CreatedAt:        time.Date(2026, 4, 7, 1, 2, 3, 0, time.UTC),
		DBBackupPath:     dbPath,
		FilesBackupPath:  filesPath,
		ManifestPath:     manifestPath,
		DBSidecarPath:    dbPath + ".tmp.sha256",
		FilesSidecarPath: filesPath + ".tmp.sha256",
	})
	if err != nil {
		t.Fatal(err)
	}

	for _, path := range []string{info.DBSidecarPath, info.FilesSidecarPath, info.ManifestPath} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected finalized artifact %s: %v", path, err)
		}
	}
	if info.DBSidecarPath != dbPath+".tmp.sha256" {
		t.Fatalf("unexpected db sidecar path: %s", info.DBSidecarPath)
	}
	if info.FilesSidecarPath != filesPath+".tmp.sha256" {
		t.Fatalf("unexpected files sidecar path: %s", info.FilesSidecarPath)
	}
	if err := Verify(VerifyRequest{ManifestPath: manifestPath}); err != nil {
		t.Fatalf("finalized backup should verify: %v", err)
	}
}
