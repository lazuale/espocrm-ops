package cli

import (
	"path/filepath"
	"testing"
)

func normalizeStatusReportJSON(t *testing.T, raw []byte, fixture supportBundleFixture) []byte {
	t.Helper()

	obj := decodeJSONMap(t, raw)

	replacements := mergeReplacements(map[string]string{
		fixture.projectDir: "REPLACE_PROJECT_DIR",
		filepath.Join(fixture.projectDir, "compose.yaml"):           "REPLACE_COMPOSE_FILE",
		filepath.Join(fixture.projectDir, ".env.dev"):               "REPLACE_ENV_FILE",
		filepath.Join(fixture.projectDir, ".env.prod"):              "REPLACE_ENV_FILE",
		filepath.Join(fixture.projectDir, "runtime", "dev", "db"):   "REPLACE_DB_STORAGE_DIR",
		filepath.Join(fixture.projectDir, "runtime", "dev", "espo"): "REPLACE_ESPO_STORAGE_DIR",
		fixture.backupRoot:                           "REPLACE_BACKUP_ROOT",
		filepath.Join(fixture.backupRoot, "reports"): "REPLACE_REPORTS_DIR",
		filepath.Join(fixture.backupRoot, "support"): "REPLACE_SUPPORT_DIR",
		fixture.backupSet.DBBackup:                   "REPLACE_DB_BACKUP",
		fixture.backupSet.DBBackup + ".sha256":       "REPLACE_DB_CHECKSUM",
		fixture.backupSet.FilesBackup:                "REPLACE_FILES_BACKUP",
		fixture.backupSet.FilesBackup + ".sha256":    "REPLACE_FILES_CHECKSUM",
		fixture.backupSet.ManifestJSON:               "REPLACE_MANIFEST_JSON",
		fixture.backupSet.ManifestTXT:                "REPLACE_MANIFEST_TXT",
	}, operatorRuntimeLockReplacements(obj))

	obj = normalizeJSONValue(obj, replacements, map[string]jsonFieldTransform{
		"modified_at": func(any) any { return "REPLACE_MODIFIED_AT" },
		"age_hours":   func(any) any { return float64(0) },
	}).(map[string]any)

	return encodeJSONMap(t, obj)
}
