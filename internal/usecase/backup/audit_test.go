package backup

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestAudit_DetectsIncoherentLatestSet(t *testing.T) {
	root := filepath.Join(t.TempDir(), "backups")

	writeCatalogBackupSet(t, root, "espocrm-dev", "2026-04-07_00-00-00", "dev")
	writeCatalogDBOnly(t, root, "espocrm-dev", "2026-04-07_01-00-00")

	info, err := Audit(AuditRequest{
		BackupRoot:       root,
		VerifyChecksum:   true,
		DBMaxAgeHours:    999,
		FilesMaxAgeHours: 999,
		Now:              time.Date(2026, 4, 7, 2, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}

	if info.Success {
		t.Fatal("expected audit failure")
	}
	if info.SelectedSet.Stamp != "2026-04-07_01-00-00" {
		t.Fatalf("unexpected selected stamp: %s", info.SelectedSet.Stamp)
	}
	if info.FilesBackup.Status != AuditStatusFail {
		t.Fatalf("expected files failure, got %#v", info.FilesBackup)
	}
	if len(info.Findings) == 0 {
		t.Fatal("expected failure findings")
	}
}

func TestAudit_SkipDBSelectsFilesSet(t *testing.T) {
	root := filepath.Join(t.TempDir(), "backups")

	writeCatalogBackupSet(t, root, "espocrm-dev", "2026-04-07_01-00-00", "dev")
	writeCatalogDBOnly(t, root, "espocrm-dev", "2026-04-07_02-00-00")

	info, err := Audit(AuditRequest{
		BackupRoot:       root,
		SkipDB:           true,
		VerifyChecksum:   true,
		DBMaxAgeHours:    999,
		FilesMaxAgeHours: 999,
		Now:              time.Date(2026, 4, 7, 2, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}

	if !info.Success {
		t.Fatalf("expected audit success, findings=%#v", info.Findings)
	}
	if info.SelectedSet.Stamp != "2026-04-07_01-00-00" {
		t.Fatalf("unexpected selected stamp: %s", info.SelectedSet.Stamp)
	}
	if info.DBBackup.Status != AuditStatusSkipped {
		t.Fatalf("expected skipped DB audit, got %#v", info.DBBackup)
	}
	if !info.FilesBackup.ChecksumVerified {
		t.Fatal("expected files checksum verification")
	}
}

func TestAudit_CorruptedSidecarBecomesStructuredFailure(t *testing.T) {
	root := filepath.Join(t.TempDir(), "backups")

	dbPath, _ := writeCatalogBackupSet(t, root, "espocrm-dev", "2026-04-07_01-00-00", "dev")
	if err := os.WriteFile(dbPath+".sha256", []byte("bad sidecar\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	info, err := Audit(AuditRequest{
		BackupRoot:       root,
		VerifyChecksum:   true,
		DBMaxAgeHours:    999,
		FilesMaxAgeHours: 999,
		Now:              time.Date(2026, 4, 7, 2, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}

	if info.DBBackup.Status != AuditStatusFail {
		t.Fatalf("expected db backup failure, got %#v", info.DBBackup)
	}
}
