package cli

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

type maintenanceFixture struct {
	supportBundleFixture
	oldJournalPath       string
	oldReportTXT         string
	recentReportTXT      string
	oldReportJSON        string
	recentReportJSON     string
	oldSupportBundle     string
	recentSupportBundle  string
	oldRestoreEnvFile    string
	recentRestoreEnvFile string
	restoreStorageDir    string
	restoreBackupDir     string
}

func prepareMaintenanceFixture(t *testing.T, scope string) maintenanceFixture {
	t.Helper()

	base := prepareSupportBundleFixture(t, scope)
	fixture := maintenanceFixture{
		supportBundleFixture: base,
		oldJournalPath:       filepath.Join(base.journalDir, "update-old.json"),
		oldReportTXT:         filepath.Join(base.backupRoot, "reports", "espocrm-"+scope+"_restore-drill_2026-03-01_01-00-00.txt"),
		recentReportTXT:      filepath.Join(base.backupRoot, "reports", "espocrm-"+scope+"_restore-drill_2026-04-18_08-00-00.txt"),
		oldReportJSON:        filepath.Join(base.backupRoot, "reports", "espocrm-"+scope+"_restore-drill_2026-03-01_01-00-00.json"),
		recentReportJSON:     filepath.Join(base.backupRoot, "reports", "espocrm-"+scope+"_restore-drill_2026-04-18_08-00-00.json"),
		oldSupportBundle:     filepath.Join(base.backupRoot, "support", "espocrm-"+scope+"_support_2026-03-01_01-00-00.tar.gz"),
		recentSupportBundle:  filepath.Join(base.backupRoot, "support", "espocrm-"+scope+"_support_2026-04-18_08-00-00.tar.gz"),
		oldRestoreEnvFile:    filepath.Join(base.projectDir, ".cache", "env", "restore-drill."+scope+".old.env"),
		recentRestoreEnvFile: filepath.Join(base.projectDir, ".cache", "env", "restore-drill."+scope+".recent.env"),
		restoreStorageDir:    filepath.Join(base.projectDir, "storage", "restore-drill", scope),
		restoreBackupDir:     filepath.Join(base.projectDir, "backups", "restore-drill", scope),
	}

	writeJournalEntryFile(t, base.journalDir, filepath.Base(fixture.oldJournalPath), map[string]any{
		"operation_id": "op-update-old-1",
		"command":      "update",
		"started_at":   "2026-02-01T01:00:00Z",
		"finished_at":  "2026-02-01T01:03:00Z",
		"ok":           true,
		"message":      "update completed",
		"details": map[string]any{
			"scope": scope,
		},
	})

	writeFixtureFile(t, fixture.oldReportTXT, "old report txt\n")
	writeFixtureFile(t, fixture.recentReportTXT, "recent report txt\n")
	writeFixtureFile(t, fixture.oldReportJSON, "{\"report\":\"old\"}\n")
	writeFixtureFile(t, fixture.recentReportJSON, "{\"report\":\"recent\"}\n")
	writeFixtureFile(t, fixture.oldSupportBundle, "old support bundle\n")
	writeFixtureFile(t, fixture.recentSupportBundle, "recent support bundle\n")
	writeFixtureFile(t, fixture.oldRestoreEnvFile, "RESTORE_DRILL=old\n")
	writeFixtureFile(t, fixture.recentRestoreEnvFile, "RESTORE_DRILL=recent\n")
	writeFixtureFile(t, filepath.Join(fixture.restoreStorageDir, "db", "dump.sql"), "restore drill db\n")
	writeFixtureFile(t, filepath.Join(fixture.restoreStorageDir, "espo", "data", "drill.txt"), "restore drill files\n")
	writeFixtureFile(t, filepath.Join(fixture.restoreBackupDir, "manifest.txt"), "restore drill backup\n")

	oldTime := time.Date(2026, 3, 1, 1, 0, 0, 0, time.UTC)
	recentTime := time.Date(2026, 4, 18, 8, 0, 0, 0, time.UTC)
	for _, path := range []string{
		fixture.oldJournalPath,
		fixture.oldReportTXT,
		fixture.oldReportJSON,
		fixture.oldSupportBundle,
		fixture.oldRestoreEnvFile,
		fixture.restoreStorageDir,
		fixture.restoreBackupDir,
	} {
		setFixturePathModTime(t, path, oldTime)
	}
	for _, path := range []string{
		filepath.Join(base.journalDir, "backup.json"),
		filepath.Join(base.journalDir, "restore.json"),
		fixture.recentReportTXT,
		fixture.recentReportJSON,
		fixture.recentSupportBundle,
		fixture.recentRestoreEnvFile,
	} {
		setFixturePathModTime(t, path, recentTime)
	}

	return fixture
}

func writeFixtureFile(t *testing.T, path, body string) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func setFixturePathModTime(t *testing.T, path string, when time.Time) {
	t.Helper()

	err := filepath.Walk(path, func(current string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		return os.Chtimes(current, when, when)
	})
	if err != nil {
		t.Fatalf("set modtime for %s: %v", path, err)
	}
}

func normalizeMaintenanceJSON(t *testing.T, raw []byte, fixture maintenanceFixture) []byte {
	t.Helper()

	obj := decodeJSONMap(t, raw)

	replacements := map[string]string{
		fixture.projectDir:                   "REPLACE_PROJECT_DIR",
		fixture.projectDir + "/compose.yaml": "REPLACE_COMPOSE_FILE",
		fixture.projectDir + "/.env.dev":     "REPLACE_ENV_FILE",
		fixture.projectDir + "/.env.prod":    "REPLACE_ENV_FILE",
		fixture.journalDir:                   "REPLACE_JOURNAL_DIR",
		fixture.backupRoot:                   "REPLACE_BACKUP_ROOT",
	}

	obj = normalizeJSONValue(obj, replacements, nil).(map[string]any)

	return encodeJSONMap(t, obj)
}
