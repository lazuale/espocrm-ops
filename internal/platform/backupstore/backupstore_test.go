package backupstore

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	domainbackup "github.com/lazuale/espocrm-ops/internal/domain/backup"
)

func TestVerifyDetailed_ValidBackupSet(t *testing.T) {
	root := t.TempDir()

	manifestPath, dbPath, filesPath := writeBackupstoreSet(t, root, tarEntries{
		"storage/a.txt": "hello",
	})

	info, err := VerifyManifestDetailed(manifestPath)
	if err != nil {
		t.Fatal(err)
	}

	if info.ManifestPath != manifestPath {
		t.Fatalf("unexpected manifest path: %s", info.ManifestPath)
	}
	if info.DBBackupPath != dbPath {
		t.Fatalf("unexpected db path: %s", info.DBBackupPath)
	}
	if info.FilesPath != filesPath {
		t.Fatalf("unexpected files path: %s", info.FilesPath)
	}
	if info.Scope != "prod" {
		t.Fatalf("unexpected scope: %s", info.Scope)
	}
}

func TestVerifyDetailed_RejectsUnsafeFilesArchive(t *testing.T) {
	root := t.TempDir()

	manifestPath, _, _ := writeBackupstoreSet(t, root, tarEntries{
		"storage/a.txt": "hello",
		"storage/link":  symlinkEntry("../escape"),
	})

	_, err := VerifyManifestDetailed(manifestPath)
	if err == nil {
		t.Fatal("expected unsafe archive to fail")
	}
	if !strings.Contains(err.Error(), "symlink entries are not allowed") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestGroups_SortsNewestFirstAcrossArtifactDirs(t *testing.T) {
	root := t.TempDir()
	mustWriteEmptyFile(t, filepath.Join(root, "db", "espocrm-dev_2026-04-07_01-00-00.sql.gz"))
	mustWriteEmptyFile(t, filepath.Join(root, "files", "espocrm-dev_files_2026-04-07_03-00-00.tar.gz"))
	mustWriteEmptyFile(t, filepath.Join(root, "manifests", "espocrm-prod_2026-04-07_02-00-00.manifest.json"))
	mustWriteEmptyFile(t, filepath.Join(root, "db", "not-a-backup.txt"))

	groups, err := Groups(root, GroupModeAny)
	if err != nil {
		t.Fatal(err)
	}

	if len(groups) != 3 {
		t.Fatalf("expected 3 groups, got %#v", groups)
	}
	if groups[0].Stamp != "2026-04-07_03-00-00" {
		t.Fatalf("expected newest files group first, got %#v", groups)
	}
	if groups[1].Prefix != "espocrm-prod" || groups[1].Stamp != "2026-04-07_02-00-00" {
		t.Fatalf("unexpected second group: %#v", groups[1])
	}
}

func TestManifestCandidates_JSONOnlyNewestFirst(t *testing.T) {
	root := t.TempDir()
	mustWriteEmptyFile(t, filepath.Join(root, "manifests", "espocrm-dev_2026-04-07_01-00-00.manifest.json"))
	mustWriteEmptyFile(t, filepath.Join(root, "manifests", "espocrm-dev_2026-04-07_02-00-00.manifest.txt"))
	mustWriteEmptyFile(t, filepath.Join(root, "manifests", "espocrm-prod_2026-04-07_03-00-00.manifest.json"))

	candidates, err := ManifestCandidates(root)
	if err != nil {
		t.Fatal(err)
	}

	if len(candidates) != 2 {
		t.Fatalf("expected only json candidates, got %#v", candidates)
	}
	if candidates[0].Prefix != "espocrm-prod" || candidates[0].Stamp != "2026-04-07_03-00-00" {
		t.Fatalf("unexpected newest candidate: %#v", candidates[0])
	}
}

type tarEntries map[string]any

type symlinkEntry string

func writeBackupstoreSet(t *testing.T, root string, entries tarEntries) (string, string, string) {
	t.Helper()

	prefix := "espocrm-prod"
	stamp := "2026-04-07_01-00-00"
	dbName := prefix + "_" + stamp + ".sql.gz"
	filesName := prefix + "_files_" + stamp + ".tar.gz"
	manifestName := prefix + "_" + stamp + ".manifest.json"

	dbPath := filepath.Join(root, "db", dbName)
	filesPath := filepath.Join(root, "files", filesName)
	manifestPath := filepath.Join(root, "manifests", manifestName)

	writeTestGzip(t, dbPath, []byte("select 1;"))
	writeTestTarGz(t, filesPath, entries)

	manifest := domainbackup.Manifest{
		Version:   1,
		Scope:     "prod",
		CreatedAt: time.Date(2026, 4, 7, 1, 0, 0, 0, time.UTC).Format(time.RFC3339),
		Artifacts: domainbackup.ManifestArtifacts{
			DBBackup:    dbName,
			FilesBackup: filesName,
		},
		Checksums: domainbackup.ManifestChecksums{
			DBBackup:    sha256OfFile(t, dbPath),
			FilesBackup: sha256OfFile(t, filesPath),
		},
	}
	if err := WriteManifest(manifestPath, manifest); err != nil {
		t.Fatal(err)
	}

	return manifestPath, dbPath, filesPath
}

func mustBuildManifest(t *testing.T, dbName, filesName, dbChecksum, filesChecksum string) domainbackup.Manifest {
	t.Helper()

	manifest, err := domainbackup.BuildManifest(domainbackup.ManifestBuildRequest{
		Scope:           "prod",
		CreatedAt:       time.Date(2026, 4, 7, 1, 0, 0, 0, time.UTC),
		DBBackupName:    dbName,
		FilesBackupName: filesName,
		DBChecksum:      dbChecksum,
		FilesChecksum:   filesChecksum,
	})
	if err != nil {
		t.Fatal(err)
	}

	return manifest
}

func writeTestGzip(t *testing.T, path string, body []byte) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
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

func writeTestSidecar(t *testing.T, filePath string) {
	t.Helper()

	body := sha256OfFile(t, filePath) + "  " + filepath.Base(filePath) + "\n"
	if err := os.WriteFile(filePath+".sha256", []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func writeTestTarGz(t *testing.T, path string, entries tarEntries) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	gz := gzip.NewWriter(f)
	defer gz.Close()

	tw := tar.NewWriter(gz)
	defer tw.Close()

	for name, value := range entries {
		switch entry := value.(type) {
		case string:
			hdr := &tar.Header{
				Name: name,
				Mode: 0o644,
				Size: int64(len(entry)),
			}
			if err := tw.WriteHeader(hdr); err != nil {
				t.Fatal(err)
			}
			if _, err := tw.Write([]byte(entry)); err != nil {
				t.Fatal(err)
			}
		case symlinkEntry:
			hdr := &tar.Header{
				Name:     name,
				Typeflag: tar.TypeSymlink,
				Linkname: string(entry),
			}
			if err := tw.WriteHeader(hdr); err != nil {
				t.Fatal(err)
			}
		default:
			t.Fatalf("unsupported test tar entry %T", value)
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

func mustWriteEmptyFile(t *testing.T, path string) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, nil, 0o644); err != nil {
		t.Fatal(err)
	}
}
