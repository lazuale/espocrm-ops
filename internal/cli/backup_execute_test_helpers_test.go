package cli

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

type backupCommandFixture struct {
	projectDir  string
	journalDir  string
	composeFile string
	storageDir  string
	envFile     string
	fixedNow    time.Time
	docker      *dockerHarness
}

func prepareBackupCommandFixture(t *testing.T) backupCommandFixture {
	t.Helper()

	tmp := t.TempDir()
	projectDir := filepath.Join(tmp, "project")
	storageDir := filepath.Join(projectDir, "runtime", "dev", "espo")
	fixedNow := time.Date(2026, 4, 15, 11, 0, 0, 0, time.UTC)

	mustMkdirAll(t, projectDir, storageDir)
	mustWriteFile(t, filepath.Join(storageDir, "file.txt"), "hello\n")
	mustSetTreeTimes(t, storageDir, fixedNow)

	composeFile := mustComposeFile(t, projectDir)
	envFile := writeDoctorEnvFile(t, projectDir, "dev", map[string]string{
		"COMPOSE_PROJECT_NAME": "espocrm-test",
		"ESPO_STORAGE_DIR":     "./runtime/dev/espo",
		"BACKUP_ROOT":          "./backups/dev",
		"BACKUP_NAME_PREFIX":   "espocrm-test-dev",
	})

	return backupCommandFixture{
		projectDir:  projectDir,
		journalDir:  filepath.Join(tmp, "journal"),
		composeFile: composeFile,
		storageDir:  storageDir,
		envFile:     envFile,
		fixedNow:    fixedNow,
		docker:      newDockerHarness(t),
	}
}

func normalizeBackupJSON(t *testing.T, raw []byte) []byte {
	t.Helper()

	obj := parseCLIJSONBytes(t, raw)
	replacements := normalizeArtifactPlaceholders(obj, map[string]string{
		"project_dir":    "REPLACE_PROJECT_DIR",
		"compose_file":   "REPLACE_COMPOSE_FILE",
		"env_file":       "REPLACE_ENV_FILE",
		"backup_root":    "REPLACE_BACKUP_ROOT",
		"manifest_txt":   "REPLACE_MANIFEST_TXT",
		"manifest_json":  "REPLACE_MANIFEST_JSON",
		"files_backup":   "REPLACE_FILES_BACKUP",
		"files_checksum": "REPLACE_FILES_CHECKSUM",
	})
	normalizeItemStringFields(obj, replacements, "details")
	return marshalCLIJSON(t, obj)
}

func mustMkdirAll(t *testing.T, paths ...string) {
	t.Helper()

	for _, path := range paths {
		if err := os.MkdirAll(path, 0o755); err != nil {
			t.Fatal(err)
		}
	}
}

func mustWriteFile(t *testing.T, path, body string) {
	t.Helper()

	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func mustSetTreeTimes(t *testing.T, root string, when time.Time) {
	t.Helper()

	if err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		return os.Chtimes(path, when, when)
	}); err != nil {
		t.Fatal(err)
	}
}
