package cli

import (
	"path/filepath"
	"testing"
)

func normalizeHealthSummaryJSON(t *testing.T, raw []byte, fixture supportBundleFixture) []byte {
	t.Helper()

	obj := decodeJSONMap(t, raw)

	replacements := map[string]string{
		fixture.projectDir: "REPLACE_PROJECT_DIR",
		filepath.Join(fixture.projectDir, "compose.yaml"):           "REPLACE_COMPOSE_FILE",
		filepath.Join(fixture.projectDir, ".env.dev"):               "REPLACE_ENV_FILE",
		filepath.Join(fixture.projectDir, ".env.prod"):              "REPLACE_ENV_FILE",
		filepath.Join(fixture.projectDir, "runtime", "dev", "db"):   "REPLACE_DB_STORAGE_DIR",
		filepath.Join(fixture.projectDir, "runtime", "dev", "espo"): "REPLACE_ESPO_STORAGE_DIR",
		fixture.backupRoot:                                             "REPLACE_BACKUP_ROOT",
		filepath.Join(fixture.backupRoot, "reports"):                   "REPLACE_REPORTS_DIR",
		filepath.Join(fixture.backupRoot, "support"):                   "REPLACE_SUPPORT_DIR",
		filepath.Join(fixture.backupRoot, "locks", "maintenance.lock"): "REPLACE_MAINTENANCE_LOCK",
		fixture.backupSet.DBBackup:                                     "REPLACE_DB_BACKUP",
		fixture.backupSet.DBBackup + ".sha256":                         "REPLACE_DB_CHECKSUM",
		fixture.backupSet.FilesBackup:                                  "REPLACE_FILES_BACKUP",
		fixture.backupSet.FilesBackup + ".sha256":                      "REPLACE_FILES_CHECKSUM",
		fixture.backupSet.ManifestJSON:                                 "REPLACE_MANIFEST_JSON",
		fixture.backupSet.ManifestTXT:                                  "REPLACE_MANIFEST_TXT",
	}

	obj = normalizeJSONValue(obj, replacements, nil).(map[string]any)

	return encodeJSONMap(t, obj)
}
