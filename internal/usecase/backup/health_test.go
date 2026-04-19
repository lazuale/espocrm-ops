package backup

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestHealth_HealthyWhenLatestReadyBackupIsFreshAndVerified(t *testing.T) {
	root := filepath.Join(t.TempDir(), "backups")

	writeCatalogBackupSet(t, root, "espocrm-dev", "2026-04-07_01-00-00", "dev")
	touchCatalogSetFiles(t, root, "espocrm-dev", "2026-04-07_01-00-00", time.Date(2026, 4, 7, 1, 0, 0, 0, time.UTC))

	info, err := Health(HealthRequest{
		BackupRoot:     root,
		VerifyChecksum: true,
		MaxAgeHours:    48,
		Now:            time.Date(2026, 4, 7, 2, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}

	if info.Verdict != HealthVerdictHealthy {
		t.Fatalf("unexpected verdict: %s", info.Verdict)
	}
	if !info.RestoreReady || !info.FreshnessSatisfied || !info.VerificationSatisfied {
		t.Fatalf("unexpected health flags: %#v", info)
	}
	if info.LatestSet == nil || info.LatestReadySet == nil || info.LatestSet.ID != info.LatestReadySet.ID {
		t.Fatalf("expected latest set and latest ready set to match, got latest=%#v latestReady=%#v", info.LatestSet, info.LatestReadySet)
	}
	if len(info.Alerts) != 0 {
		t.Fatalf("expected no alerts, got %#v", info.Alerts)
	}
}

func TestHealth_DegradedWhenLatestObservedSetIsNotReady(t *testing.T) {
	root := filepath.Join(t.TempDir(), "backups")

	writeCatalogBackupSet(t, root, "espocrm-dev", "2026-04-07_01-00-00", "dev")
	writeCatalogDBOnly(t, root, "espocrm-dev", "2026-04-07_02-00-00")
	touchCatalogSetFiles(t, root, "espocrm-dev", "2026-04-07_01-00-00", time.Date(2026, 4, 7, 1, 0, 0, 0, time.UTC))
	if err := os.Chtimes(filepath.Join(root, "db", "espocrm-dev_2026-04-07_02-00-00.sql.gz"), time.Date(2026, 4, 7, 2, 0, 0, 0, time.UTC), time.Date(2026, 4, 7, 2, 0, 0, 0, time.UTC)); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(filepath.Join(root, "db", "espocrm-dev_2026-04-07_02-00-00.sql.gz.sha256"), time.Date(2026, 4, 7, 2, 0, 0, 0, time.UTC), time.Date(2026, 4, 7, 2, 0, 0, 0, time.UTC)); err != nil {
		t.Fatal(err)
	}

	info, err := Health(HealthRequest{
		BackupRoot:     root,
		VerifyChecksum: true,
		MaxAgeHours:    48,
		Now:            time.Date(2026, 4, 7, 3, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}

	if info.Verdict != HealthVerdictDegraded {
		t.Fatalf("unexpected verdict: %s", info.Verdict)
	}
	if info.BreachCount() != 0 || info.WarningCount() == 0 {
		t.Fatalf("unexpected alert counts: warnings=%d breaches=%d", info.WarningCount(), info.BreachCount())
	}
	if info.LatestSet == nil || info.LatestSet.Stamp != "2026-04-07_02-00-00" {
		t.Fatalf("unexpected latest set: %#v", info.LatestSet)
	}
	if info.LatestReadySet == nil || info.LatestReadySet.Stamp != "2026-04-07_01-00-00" {
		t.Fatalf("unexpected latest ready set: %#v", info.LatestReadySet)
	}
	if len(info.Alerts) == 0 || info.Alerts[0].Code != "latest_set_not_ready" {
		t.Fatalf("expected latest_set_not_ready warning, got %#v", info.Alerts)
	}
}

func TestHealth_BlockedWhenLatestReadyBackupBreachesFreshness(t *testing.T) {
	root := filepath.Join(t.TempDir(), "backups")

	writeCatalogBackupSet(t, root, "espocrm-dev", "2026-04-07_01-00-00", "dev")
	touchCatalogSetFiles(t, root, "espocrm-dev", "2026-04-07_01-00-00", time.Date(2026, 4, 7, 1, 0, 0, 0, time.UTC))

	info, err := Health(HealthRequest{
		BackupRoot:     root,
		VerifyChecksum: true,
		MaxAgeHours:    24,
		Now:            time.Date(2026, 4, 9, 2, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}

	if info.Verdict != HealthVerdictBlocked {
		t.Fatalf("unexpected verdict: %s", info.Verdict)
	}
	if info.BreachCount() == 0 {
		t.Fatalf("expected a breach alert, got %#v", info.Alerts)
	}
	if len(info.Alerts) == 0 || info.Alerts[0].Code != "freshness_breached" {
		t.Fatalf("expected freshness_breached alert, got %#v", info.Alerts)
	}
}

func TestHealth_DegradedWhenChecksumVerificationIsSkipped(t *testing.T) {
	root := filepath.Join(t.TempDir(), "backups")

	writeCatalogBackupSet(t, root, "espocrm-dev", "2026-04-07_01-00-00", "dev")
	touchCatalogSetFiles(t, root, "espocrm-dev", "2026-04-07_01-00-00", time.Date(2026, 4, 7, 1, 0, 0, 0, time.UTC))

	info, err := Health(HealthRequest{
		BackupRoot:     root,
		VerifyChecksum: false,
		MaxAgeHours:    48,
		Now:            time.Date(2026, 4, 7, 2, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}

	if info.Verdict != HealthVerdictDegraded {
		t.Fatalf("unexpected verdict: %s", info.Verdict)
	}
	if info.VerificationSatisfied {
		t.Fatalf("expected verification_satisfied=false, got %#v", info)
	}
	if len(info.Alerts) == 0 || info.Alerts[0].Code != "checksum_verification_skipped" {
		t.Fatalf("expected checksum_verification_skipped alert, got %#v", info.Alerts)
	}
}

func touchCatalogSetFiles(t *testing.T, root, prefix, stamp string, modTime time.Time) {
	t.Helper()

	paths := []string{
		filepath.Join(root, "db", prefix+"_"+stamp+".sql.gz"),
		filepath.Join(root, "db", prefix+"_"+stamp+".sql.gz.sha256"),
		filepath.Join(root, "files", prefix+"_files_"+stamp+".tar.gz"),
		filepath.Join(root, "files", prefix+"_files_"+stamp+".tar.gz.sha256"),
		filepath.Join(root, "manifests", prefix+"_"+stamp+".manifest.txt"),
		filepath.Join(root, "manifests", prefix+"_"+stamp+".manifest.json"),
	}

	for _, path := range paths {
		if err := os.Chtimes(path, modTime, modTime); err != nil {
			t.Fatal(err)
		}
	}
}
