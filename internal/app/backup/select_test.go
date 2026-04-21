package backup

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	backupverifyapp "github.com/lazuale/espocrm-ops/internal/app/backupverify"
	domainbackup "github.com/lazuale/espocrm-ops/internal/domain/backup"
)

func TestDiagnoseBackupRoot_SelectsNewestValidCompleteSet(t *testing.T) {
	tmp := t.TempDir()
	root := filepath.Join(tmp, "backups")

	validManifest := writeBackupSetForSelection(t, root, "espocrm-dev", "2026-04-07_01-00-00", "dev")
	writeIncompleteManifestForSelection(t, root, "espocrm-dev", "2026-04-07_02-00-00", "dev")

	report, err := testBackupVerifyService().Diagnose(backupverifyapp.Request{BackupRoot: root})
	if err != nil {
		t.Fatalf("Diagnose failed: %v", err)
	}
	if report.ManifestPath != validManifest {
		t.Fatalf("expected valid manifest %s, got %s", validManifest, report.ManifestPath)
	}
}

func TestDiagnoseBackupRoot_FailsWhenNoCompleteSetExists(t *testing.T) {
	tmp := t.TempDir()
	root := filepath.Join(tmp, "backups")

	writeIncompleteManifestForSelection(t, root, "espocrm-dev", "2026-04-07_02-00-00", "dev")

	if _, err := testBackupVerifyService().Diagnose(backupverifyapp.Request{BackupRoot: root}); err == nil {
		t.Fatal("expected no complete backup set error")
	}
}

func writeBackupSetForSelection(t *testing.T, root, prefix, stamp, scope string) string {
	t.Helper()

	dbName := prefix + "_" + stamp + ".sql.gz"
	filesName := prefix + "_files_" + stamp + ".tar.gz"
	dbPath := filepath.Join(root, "db", dbName)
	filesPath := filepath.Join(root, "files", filesName)
	manifestPath := filepath.Join(root, "manifests", prefix+"_"+stamp+".manifest.json")

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

	return manifestPath
}

func writeIncompleteManifestForSelection(t *testing.T, root, prefix, stamp, scope string) string {
	t.Helper()

	dbName := prefix + "_" + stamp + ".sql.gz"
	filesName := prefix + "_files_" + stamp + ".tar.gz"
	manifestPath := filepath.Join(root, "manifests", prefix+"_"+stamp+".manifest.json")
	if err := os.MkdirAll(filepath.Dir(manifestPath), 0o755); err != nil {
		t.Fatal(err)
	}
	writeManifest(t, manifestPath, domainbackup.Manifest{
		Version:   1,
		Scope:     scope,
		CreatedAt: time.Date(2026, 4, 7, 2, 0, 0, 0, time.UTC).Format(time.RFC3339),
		Artifacts: domainbackup.ManifestArtifacts{
			DBBackup:    dbName,
			FilesBackup: filesName,
		},
		Checksums: domainbackup.ManifestChecksums{
			DBBackup:    stringsOf("ab", 32),
			FilesBackup: stringsOf("cd", 32),
		},
	})

	return manifestPath
}
