package cli

import (
	"os"
	"testing"
)

func TestMaintenanceApplyRemovesExpiredArtifacts(t *testing.T) {
	fixture := prepareMaintenanceFixture(t, "dev")
	useJournalClockForTest(t, fixture.fixedNow)

	previewOut, previewErr := runRootCommandWithOptions(
		t,
		[]testAppOption{withFixedTestRuntime(fixture.fixedNow, "op-maintenance-preview-1")},
		"--journal-dir", fixture.journalDir,
		"maintenance",
		"--scope", "dev",
		"--project-dir", fixture.projectDir,
		"--journal-keep-days", "30",
		"--journal-keep-last", "2",
	)
	if previewErr != nil {
		t.Fatalf("preview failed: %v\noutput=%s", previewErr, previewOut)
	}

	for _, path := range []string{
		fixture.oldJournalPath,
		fixture.oldReportTXT,
		fixture.oldReportJSON,
		fixture.oldSupportBundle,
		fixture.oldRestoreEnvFile,
		fixture.restoreStorageDir,
		fixture.restoreBackupDir,
	} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected preview to keep %s: %v", path, err)
		}
	}

	applyOut, applyErr := runRootCommandWithOptions(
		t,
		[]testAppOption{withFixedTestRuntime(fixture.fixedNow, "op-maintenance-apply-1")},
		"--journal-dir", fixture.journalDir,
		"maintenance",
		"--scope", "dev",
		"--project-dir", fixture.projectDir,
		"--journal-keep-days", "30",
		"--journal-keep-last", "2",
		"--apply",
	)
	if applyErr != nil {
		t.Fatalf("apply failed: %v\noutput=%s", applyErr, applyOut)
	}

	for _, path := range []string{
		fixture.oldJournalPath,
		fixture.oldReportTXT,
		fixture.oldReportJSON,
		fixture.oldSupportBundle,
		fixture.oldRestoreEnvFile,
		fixture.restoreStorageDir,
		fixture.restoreBackupDir,
	} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("expected apply to remove %s, got err=%v", path, err)
		}
	}

	for _, path := range []string{
		fixture.recentReportTXT,
		fixture.recentReportJSON,
		fixture.recentSupportBundle,
		fixture.recentRestoreEnvFile,
	} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected apply to keep %s: %v", path, err)
		}
	}
}
