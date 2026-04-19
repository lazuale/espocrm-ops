package backup

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	domainbackup "github.com/lazuale/espocrm-ops/internal/domain/backup"
)

func TestCatalog_ReadyOnlyVerifiedSkipsIncompleteAndCorrupted(t *testing.T) {
	root := filepath.Join(t.TempDir(), "backups")

	writeCatalogBackupSet(t, root, "espocrm-dev", "2026-04-07_01-00-00", "dev")
	writeCatalogDBOnly(t, root, "espocrm-dev", "2026-04-07_02-00-00")
	_, corruptedFiles := writeCatalogBackupSet(t, root, "espocrm-dev", "2026-04-07_03-00-00", "dev")
	if err := os.WriteFile(corruptedFiles, []byte("corrupted after sidecar"), 0o644); err != nil {
		t.Fatal(err)
	}

	info, err := Catalog(CatalogRequest{
		BackupRoot:     root,
		VerifyChecksum: true,
		ReadyOnly:      true,
		Now:            time.Date(2026, 4, 15, 12, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}

	if info.TotalSets != 3 {
		t.Fatalf("expected total_sets=3, got %d", info.TotalSets)
	}
	if info.ShownSets != 1 || len(info.Items) != 1 {
		t.Fatalf("expected one ready item, got shown=%d len=%d", info.ShownSets, len(info.Items))
	}
	item := info.Items[0]
	if item.Stamp != "2026-04-07_01-00-00" {
		t.Fatalf("unexpected selected stamp: %s", item.Stamp)
	}
	if item.RestoreReadiness != CatalogReadinessReadyVerified {
		t.Fatalf("unexpected readiness: %s", item.RestoreReadiness)
	}
	if item.DB.ChecksumStatus != CatalogChecksumVerified {
		t.Fatalf("unexpected db checksum status: %s", item.DB.ChecksumStatus)
	}
}

func TestCatalog_LimitKeepsNewestFirst(t *testing.T) {
	root := filepath.Join(t.TempDir(), "backups")

	writeCatalogBackupSet(t, root, "espocrm-dev", "2026-04-07_01-00-00", "dev")
	writeCatalogBackupSet(t, root, "espocrm-dev", "2026-04-07_02-00-00", "dev")

	info, err := Catalog(CatalogRequest{
		BackupRoot: root,
		Limit:      1,
		Now:        time.Date(2026, 4, 15, 12, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}

	if info.TotalSets != 2 {
		t.Fatalf("expected total_sets=2, got %d", info.TotalSets)
	}
	if info.ShownSets != 1 || info.Items[0].Stamp != "2026-04-07_02-00-00" {
		t.Fatalf("unexpected limited items: %#v", info.Items)
	}
	if info.Items[0].RestoreReadiness != CatalogReadinessReadyUnverified {
		t.Fatalf("unexpected readiness without checksum verification: %s", info.Items[0].RestoreReadiness)
	}
}

func TestCatalog_CorruptedSidecarBecomesCorruptedItem(t *testing.T) {
	root := filepath.Join(t.TempDir(), "backups")

	dbPath, _ := writeCatalogBackupSet(t, root, "espocrm-dev", "2026-04-07_01-00-00", "dev")
	if err := os.WriteFile(dbPath+".sha256", []byte("bad sidecar\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	info, err := Catalog(CatalogRequest{
		BackupRoot:     root,
		VerifyChecksum: true,
		Now:            time.Date(2026, 4, 15, 12, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}

	if len(info.Items) != 1 {
		t.Fatalf("expected one item, got %#v", info.Items)
	}
	if info.Items[0].DB.ChecksumStatus != CatalogChecksumMismatch {
		t.Fatalf("unexpected checksum status: %s", info.Items[0].DB.ChecksumStatus)
	}
	if info.Items[0].RestoreReadiness != CatalogReadinessCorrupted {
		t.Fatalf("unexpected readiness: %s", info.Items[0].RestoreReadiness)
	}
}

func TestCatalog_UsesManifestMetadataAndJournalOrigin(t *testing.T) {
	root := filepath.Join(t.TempDir(), "backups")
	journalDir := filepath.Join(t.TempDir(), "journal")
	if err := os.MkdirAll(journalDir, 0o755); err != nil {
		t.Fatal(err)
	}

	_, _ = writeCatalogBackupSet(t, root, "espocrm-prod", "2026-04-07_01-00-00", "prod")
	manifestPath := filepath.Join(root, "manifests", "espocrm-prod_2026-04-07_01-00-00.manifest.json")
	writeCatalogJournalEntry(t, journalDir, "update.json", map[string]any{
		"operation_id": "op-update-1",
		"command":      "update",
		"started_at":   "2026-04-07T01:05:00Z",
		"finished_at":  "2026-04-07T01:07:00Z",
		"ok":           true,
		"artifacts": map[string]any{
			"manifest_json": manifestPath,
		},
	})

	info, err := Catalog(CatalogRequest{
		BackupRoot:     root,
		JournalDir:     journalDir,
		VerifyChecksum: true,
		Now:            time.Date(2026, 4, 15, 12, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}

	if info.JournalRead.TotalFilesSeen != 1 {
		t.Fatalf("expected one journal file, got %#v", info.JournalRead)
	}
	item := info.Items[0]
	if item.ID != "espocrm-prod_2026-04-07_01-00-00" {
		t.Fatalf("unexpected id: %s", item.ID)
	}
	if item.Scope != "prod" || item.Contour != "prod" {
		t.Fatalf("unexpected scope summary: %#v", item)
	}
	if item.CreatedAt != "2026-04-07T01:00:00Z" {
		t.Fatalf("unexpected created_at: %s", item.CreatedAt)
	}
	if item.Origin.Kind != BackupOriginUpdateRecoveryPoint {
		t.Fatalf("unexpected origin: %#v", item.Origin)
	}
	if item.Origin.OperationID != "op-update-1" {
		t.Fatalf("unexpected origin operation: %#v", item.Origin)
	}
	if item.ManifestJSON.Status != CatalogManifestValid {
		t.Fatalf("unexpected manifest status: %#v", item.ManifestJSON)
	}
}

func TestCatalog_InvalidManifestBecomesCorrupted(t *testing.T) {
	root := filepath.Join(t.TempDir(), "backups")

	_, _ = writeCatalogBackupSet(t, root, "espocrm-dev", "2026-04-07_01-00-00", "dev")
	manifestPath := filepath.Join(root, "manifests", "espocrm-dev_2026-04-07_01-00-00.manifest.json")
	if err := os.WriteFile(manifestPath, []byte("{bad json"), 0o644); err != nil {
		t.Fatal(err)
	}

	info, err := Catalog(CatalogRequest{
		BackupRoot:     root,
		VerifyChecksum: true,
		Now:            time.Date(2026, 4, 15, 12, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}

	item := info.Items[0]
	if item.ManifestJSON.Status != CatalogManifestInvalid {
		t.Fatalf("unexpected manifest status: %#v", item.ManifestJSON)
	}
	if item.RestoreReadiness != CatalogReadinessCorrupted {
		t.Fatalf("unexpected readiness: %s", item.RestoreReadiness)
	}
}

func writeCatalogBackupSet(t *testing.T, root, prefix, stamp, scope string) (string, string) {
	t.Helper()

	dbName := prefix + "_" + stamp + ".sql.gz"
	filesName := prefix + "_files_" + stamp + ".tar.gz"
	dbPath := filepath.Join(root, "db", dbName)
	filesPath := filepath.Join(root, "files", filesName)
	manifestTXTPath := filepath.Join(root, "manifests", prefix+"_"+stamp+".manifest.txt")
	manifestJSONPath := filepath.Join(root, "manifests", prefix+"_"+stamp+".manifest.json")

	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(filesPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(manifestTXTPath), 0o755); err != nil {
		t.Fatal(err)
	}

	writeGzipFile(t, dbPath, []byte("select 1;"))
	writeTarGz(t, filesPath, map[string]string{
		"storage/test.txt": "hello",
	})
	writeCatalogSidecar(t, dbPath)
	writeCatalogSidecar(t, filesPath)
	writeCatalogTXTManifest(t, manifestTXTPath, stamp, scope)
	writeManifest(t, manifestJSONPath, domainbackup.Manifest{
		Version:   1,
		Scope:     scope,
		CreatedAt: time.Date(2026, 4, 7, 1, 0, 0, 0, time.UTC).Format(time.RFC3339),
		Artifacts: domainbackup.ManifestArtifacts{
			DBBackup:    dbName,
			FilesBackup: filesName,
		},
		Checksums: domainbackup.ManifestChecksums{
			DBBackup:    sha256OfFile(t, dbPath),
			FilesBackup: sha256OfFile(t, filesPath),
		},
	})

	return dbPath, filesPath
}

func writeCatalogDBOnly(t *testing.T, root, prefix, stamp string) string {
	t.Helper()

	dbPath := filepath.Join(root, "db", prefix+"_"+stamp+".sql.gz")
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		t.Fatal(err)
	}
	writeGzipFile(t, dbPath, []byte("select 1;"))
	writeCatalogSidecar(t, dbPath)

	return dbPath
}

func writeCatalogSidecar(t *testing.T, filePath string) {
	t.Helper()

	body := sha256OfFile(t, filePath) + "  " + filepath.Base(filePath) + "\n"
	if err := os.WriteFile(filePath+".sha256", []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func writeCatalogTXTManifest(t *testing.T, path, stamp, scope string) {
	t.Helper()

	body := "created_at=" + stamp + "\ncontour=" + scope + "\ncompose_project=espocrm-test\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func writeCatalogJournalEntry(t *testing.T, dir, name string, entry any) {
	t.Helper()

	raw, err := json.MarshalIndent(entry, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), raw, 0o644); err != nil {
		t.Fatal(err)
	}
}
