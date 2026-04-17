package backup

import (
	"strings"
	"testing"
	"time"
)

func TestBuildManifestTrimsScopeAndUsesArtifactNames(t *testing.T) {
	manifest, err := BuildManifest(ManifestBuildRequest{
		Scope:           " prod ",
		CreatedAt:       time.Date(2026, 4, 15, 12, 0, 0, 0, time.FixedZone("test", 3*60*60)),
		DBBackupName:    "/tmp/db.sql.gz",
		FilesBackupName: "/tmp/files.tar.gz",
		DBChecksum:      strings.Repeat("a", 64),
		FilesChecksum:   strings.Repeat("b", 64),
	})
	if err != nil {
		t.Fatal(err)
	}

	if manifest.Scope != "prod" {
		t.Fatalf("unexpected scope: %s", manifest.Scope)
	}
	if manifest.CreatedAt != "2026-04-15T09:00:00Z" {
		t.Fatalf("unexpected created_at: %s", manifest.CreatedAt)
	}
	if manifest.Artifacts.DBBackup != "db.sql.gz" {
		t.Fatalf("unexpected db backup: %s", manifest.Artifacts.DBBackup)
	}
	if manifest.Artifacts.FilesBackup != "files.tar.gz" {
		t.Fatalf("unexpected files backup: %s", manifest.Artifacts.FilesBackup)
	}
}

func TestManifestValidateRejectsPathArtifact(t *testing.T) {
	manifest := validManifestForTest()
	manifest.Artifacts.DBBackup = "../db.sql.gz"

	if err := manifest.Validate(); err == nil {
		t.Fatal("expected validation error")
	}
}

func TestManifestValidateRejectsBadChecksum(t *testing.T) {
	manifest := validManifestForTest()
	manifest.Checksums.FilesBackup = "not-a-sha"

	if err := manifest.Validate(); err == nil {
		t.Fatal("expected validation error")
	}
}

func validManifestForTest() Manifest {
	return Manifest{
		Version:   1,
		Scope:     "prod",
		CreatedAt: "2026-04-15T12:00:00Z",
		Artifacts: ManifestArtifacts{
			DBBackup:    "db.sql.gz",
			FilesBackup: "files.tar.gz",
		},
		Checksums: ManifestChecksums{
			DBBackup:    "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			FilesBackup: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		},
	}
}
