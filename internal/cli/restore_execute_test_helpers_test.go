package cli

import (
	"path/filepath"
	"testing"
	"time"
)

type restoreCommandFixture struct {
	projectDir string
	journalDir string
	storageDir string
	backupRoot string
	backupSet  backupSetFixture
	fixedNow   time.Time
	docker     *dockerHarness
}

func prepareRestoreCommandFixture(t *testing.T, scope string, files map[string]string) restoreCommandFixture {
	t.Helper()

	tmp := t.TempDir()
	projectDir := filepath.Join(tmp, "project")
	storageDir := filepath.Join(projectDir, "runtime", scope, "espo")
	backupRoot := filepath.Join(projectDir, "backups", scope)
	fixedNow := time.Date(2026, 4, 19, 9, 0, 0, 0, time.UTC)

	mustMkdirAll(t, projectDir, storageDir)
	mustComposeFile(t, projectDir)
	mustWriteFile(t, filepath.Join(storageDir, "before.txt"), "before\n")

	writeDoctorEnvFile(t, projectDir, scope, nil)

	return restoreCommandFixture{
		projectDir: projectDir,
		journalDir: filepath.Join(tmp, "journal"),
		storageDir: storageDir,
		backupRoot: backupRoot,
		backupSet:  writeBackupSet(t, backupRoot, "espocrm-"+scope, "2026-04-19_08-00-00", scope, files),
		fixedNow:   fixedNow,
		docker:     newDockerHarness(t),
	}
}

func normalizeRestoreJSON(t *testing.T, raw []byte) []byte {
	t.Helper()

	obj := parseCLIJSONBytes(t, raw)
	replacements := normalizeArtifactPlaceholders(obj, map[string]string{
		"project_dir":             "REPLACE_PROJECT_DIR",
		"compose_file":            "REPLACE_COMPOSE_FILE",
		"env_file":                "REPLACE_ENV_FILE",
		"backup_root":             "REPLACE_BACKUP_ROOT",
		"manifest_txt":            "REPLACE_MANIFEST_TXT",
		"manifest_json":           "REPLACE_MANIFEST_JSON",
		"db_backup":               "REPLACE_DB_BACKUP",
		"files_backup":            "REPLACE_FILES_BACKUP",
		"snapshot_manifest_txt":   "REPLACE_SNAPSHOT_MANIFEST_TXT",
		"snapshot_manifest_json":  "REPLACE_SNAPSHOT_MANIFEST_JSON",
		"snapshot_db_backup":      "REPLACE_SNAPSHOT_DB_BACKUP",
		"snapshot_files_backup":   "REPLACE_SNAPSHOT_FILES_BACKUP",
		"snapshot_db_checksum":    "REPLACE_SNAPSHOT_DB_CHECKSUM",
		"snapshot_files_checksum": "REPLACE_SNAPSHOT_FILES_CHECKSUM",
	})
	delete(obj, "message")
	if errObj, ok := obj["error"].(map[string]any); ok {
		delete(errObj, "message")
	}
	if items, ok := obj["items"].([]any); ok {
		normalized := make([]any, 0, len(items))
		for _, rawItem := range items {
			item, ok := rawItem.(map[string]any)
			if !ok {
				continue
			}
			normalized = append(normalized, map[string]any{
				"code":   item["code"],
				"status": item["status"],
			})
		}
		obj["items"] = normalized
	}
	normalizeWarningsPaths(obj, replacements)
	return marshalCLIJSON(t, obj)
}
