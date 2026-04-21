package cli

import (
	"path/filepath"
	"testing"
	"time"
)

type migrateCommandFixture struct {
	projectDir   string
	journalDir   string
	storageDir   string
	sourceBackup backupSetFixture
	sourceRoot   string
	fixedNow     time.Time
	docker       *dockerHarness
}

func prepareMigrateCommandFixture(t *testing.T) migrateCommandFixture {
	t.Helper()

	tmp := t.TempDir()
	projectDir := filepath.Join(tmp, "project")
	storageDir := filepath.Join(projectDir, "runtime", "prod", "espo")
	sourceRoot := filepath.Join(projectDir, "backups", "dev")

	mustMkdirAll(t, projectDir, storageDir)
	mustComposeFile(t, projectDir)
	mustWriteFile(t, filepath.Join(storageDir, "before.txt"), "before\n")
	writeDoctorEnvFile(t, projectDir, "dev", nil)
	writeDoctorEnvFile(t, projectDir, "prod", nil)

	return migrateCommandFixture{
		projectDir:   projectDir,
		journalDir:   filepath.Join(tmp, "journal"),
		storageDir:   storageDir,
		sourceBackup: writeBackupSet(t, sourceRoot, "espocrm-dev", "2026-04-19_08-00-00", "dev", nil),
		sourceRoot:   sourceRoot,
		fixedNow:     time.Date(2026, 4, 19, 9, 0, 0, 0, time.UTC),
		docker:       newDockerHarness(t),
	}
}

func normalizeMigrateJSON(t *testing.T, raw []byte) []byte {
	t.Helper()

	obj := parseCLIJSONBytes(t, raw)
	replacements := normalizeArtifactPlaceholders(obj, map[string]string{
		"project_dir":            "REPLACE_PROJECT_DIR",
		"compose_file":           "REPLACE_COMPOSE_FILE",
		"source_env_file":        "REPLACE_SOURCE_ENV_FILE",
		"target_env_file":        "REPLACE_TARGET_ENV_FILE",
		"source_backup_root":     "REPLACE_SOURCE_BACKUP_ROOT",
		"target_backup_root":     "REPLACE_TARGET_BACKUP_ROOT",
		"manifest_txt":           "REPLACE_MANIFEST_TXT",
		"manifest_json":          "REPLACE_MANIFEST_JSON",
		"db_backup":              "REPLACE_DB_BACKUP",
		"files_backup":           "REPLACE_FILES_BACKUP",
		"requested_db_backup":    "REPLACE_REQUESTED_DB_BACKUP",
		"requested_files_backup": "REPLACE_REQUESTED_FILES_BACKUP",
	})
	normalizeWarningsPaths(obj, replacements)
	normalizeItemStringFields(obj, replacements, "details", "action")
	return marshalCLIJSON(t, obj)
}
