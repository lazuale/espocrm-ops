package backup

import (
	"testing"
	"time"
)

func TestCatalogReadiness(t *testing.T) {
	completeDB := CatalogArtifact{File: "/backup/db.sql.gz", Sidecar: "/backup/db.sql.gz.sha256"}
	completeFiles := CatalogArtifact{File: "/backup/files.tar.gz", Sidecar: "/backup/files.tar.gz.sha256"}
	manifestTXT := CatalogManifest{File: "/backup/set.manifest.txt"}
	manifestJSON := CatalogManifest{File: "/backup/set.manifest.json"}

	if got := CatalogReadiness(completeDB, completeFiles, manifestTXT, manifestJSON, true); got != CatalogReadinessReadyVerified {
		t.Fatalf("expected verified readiness, got %s", got)
	}
	if got := CatalogReadiness(completeDB, completeFiles, manifestTXT, manifestJSON, false); got != CatalogReadinessReadyUnverified {
		t.Fatalf("expected unverified readiness, got %s", got)
	}

	corruptedDB := completeDB
	corruptedDB.ChecksumStatus = CatalogChecksumMismatch
	if got := CatalogReadiness(corruptedDB, completeFiles, manifestTXT, manifestJSON, true); got != CatalogReadinessCorrupted {
		t.Fatalf("expected corrupted readiness, got %s", got)
	}

	missingSidecar := completeFiles
	missingSidecar.Sidecar = ""
	if got := CatalogReadiness(completeDB, missingSidecar, manifestTXT, manifestJSON, true); got != CatalogReadinessIncomplete {
		t.Fatalf("expected incomplete readiness, got %s", got)
	}
}

func TestNewCatalogItemSetsGroupKeyAndReadyFlag(t *testing.T) {
	item := NewCatalogItem(
		BackupGroup{Prefix: "espocrm-prod", Stamp: "2026-04-07_01-00-00"},
		CatalogArtifact{File: "db.sql.gz", Sidecar: "db.sql.gz.sha256"},
		CatalogArtifact{File: "files.tar.gz", Sidecar: "files.tar.gz.sha256"},
		CatalogManifest{File: "set.manifest.txt"},
		CatalogManifest{File: "set.manifest.json"},
		true,
	)

	if item.GroupKey != "espocrm-prod|2026-04-07_01-00-00" {
		t.Fatalf("unexpected group key: %s", item.GroupKey)
	}
	if !item.IsReady {
		t.Fatal("expected ready item")
	}
}

func TestAgeHoursClampsFutureToZero(t *testing.T) {
	now := time.Date(2026, 4, 15, 12, 0, 0, 0, time.UTC)

	if got := AgeHours(now, now.Add(2*time.Hour)); got != 0 {
		t.Fatalf("expected future age clamped to zero, got %d", got)
	}
	if got := AgeHours(now, now.Add(-3*time.Hour)); got != 3 {
		t.Fatalf("expected age 3h, got %d", got)
	}
}

func TestAuditFindingsAndFailureCount(t *testing.T) {
	info := AuditInfo{
		DBBackup: AuditComponent{
			Status:  AuditStatusOK,
			Message: "db ok",
		},
		FilesBackup: AuditComponent{
			Status:  AuditStatusFail,
			Message: "files missing",
		},
		ManifestJSON: AuditComponent{
			Status:  AuditStatusWarn,
			Message: "checksum skipped",
		},
		ManifestTXT: AuditComponent{
			Status:  AuditStatusSkipped,
			Message: "not checked",
		},
	}

	findings := AuditFindings(info)
	if len(findings) != 2 {
		t.Fatalf("expected fail+warn findings, got %#v", findings)
	}
	if findings[0].Subject != "files_backup" || findings[0].Level != AuditStatusFail {
		t.Fatalf("unexpected first finding: %#v", findings[0])
	}
	if got := FailedAuditFindingCount(findings); got != 1 {
		t.Fatalf("expected one failure finding, got %d", got)
	}
}

func TestAuditHelpers(t *testing.T) {
	if got := ManifestMaxAgeHours(false, false, 24, 48); got != 48 {
		t.Fatalf("expected max age 48, got %d", got)
	}
	if got := ManifestMaxAgeHours(true, false, 24, 48); got != 48 {
		t.Fatalf("expected files age when db skipped, got %d", got)
	}
	if got := ManifestMaxAgeHours(false, true, 24, 48); got != 24 {
		t.Fatalf("expected db age when files skipped, got %d", got)
	}
	if got := AppendAuditMessage("base", "extra"); got != "base; extra" {
		t.Fatalf("unexpected appended message: %s", got)
	}
	if got := MaxAuditStatus(AuditStatusOK, AuditStatusWarn); got != AuditStatusWarn {
		t.Fatalf("expected warning to outrank ok, got %s", got)
	}
}
