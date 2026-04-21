package operation

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

func TestPrepareOperationDoesNotPrecreateRuntimeDirs(t *testing.T) {
	projectDir := newOperationProject(t, "prod")

	ctx, err := PrepareOperation(OperationContextRequest{
		Scope:      "prod",
		Operation:  "backup",
		ProjectDir: projectDir,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = ctx.Release()
	}()

	for _, path := range []string{
		filepath.Join(projectDir, "runtime", "prod", "db"),
		filepath.Join(projectDir, "runtime", "prod", "espo"),
		filepath.Join(projectDir, "backups", "prod", "db"),
		filepath.Join(projectDir, "backups", "prod", "files"),
		filepath.Join(projectDir, "backups", "prod", "manifests"),
	} {
		if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
			t.Fatalf("expected %s to stay absent after preflight, got err=%v", path, statErr)
		}
	}

	locksDir := filepath.Join(projectDir, "backups", "prod", "locks")
	if _, err := os.Stat(locksDir); err != nil {
		t.Fatalf("expected maintenance lock directory %s: %v", locksDir, err)
	}
}

func newOperationProject(t *testing.T, scope string) string {
	t.Helper()

	projectDir := filepath.Join(t.TempDir(), "project")
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatal(err)
	}

	values := map[string]string{
		"ESPO_CONTOUR":               scope,
		"COMPOSE_PROJECT_NAME":       "espocrm-" + scope,
		"ESPOCRM_IMAGE":              "espocrm/espocrm:9.3.4-apache",
		"MARIADB_TAG":                "10.11",
		"DB_STORAGE_DIR":             "./runtime/" + scope + "/db",
		"ESPO_STORAGE_DIR":           "./runtime/" + scope + "/espo",
		"BACKUP_ROOT":                "./backups/" + scope,
		"BACKUP_NAME_PREFIX":         "espocrm-" + scope,
		"BACKUP_RETENTION_DAYS":      "7",
		"BACKUP_MAX_DB_AGE_HOURS":    "48",
		"BACKUP_MAX_FILES_AGE_HOURS": "48",
		"REPORT_RETENTION_DAYS":      "30",
		"SUPPORT_RETENTION_DAYS":     "14",
		"MIN_FREE_DISK_MB":           "1",
		"DOCKER_LOG_MAX_SIZE":        "10m",
		"DOCKER_LOG_MAX_FILE":        "5",
		"DB_MEM_LIMIT":               "512m",
		"DB_CPUS":                    "1.00",
		"DB_PIDS_LIMIT":              "256",
		"ESPO_MEM_LIMIT":             "512m",
		"ESPO_CPUS":                  "1.00",
		"ESPO_PIDS_LIMIT":            "256",
		"DAEMON_MEM_LIMIT":           "256m",
		"DAEMON_CPUS":                "0.50",
		"DAEMON_PIDS_LIMIT":          "128",
		"WS_MEM_LIMIT":               "256m",
		"WS_CPUS":                    "0.50",
		"WS_PIDS_LIMIT":              "128",
		"APP_PORT":                   "18080",
		"WS_PORT":                    "18081",
		"SITE_URL":                   "http://127.0.0.1:18080",
		"WS_PUBLIC_URL":              "ws://127.0.0.1:18081",
		"DB_ROOT_PASSWORD":           "root-secret",
		"DB_NAME":                    "espocrm",
		"DB_USER":                    "espocrm",
		"DB_PASSWORD":                "db-secret",
		"ADMIN_USERNAME":             "admin",
		"ADMIN_PASSWORD":             "admin-secret",
		"ESPO_DEFAULT_LANGUAGE":      "ru_RU",
		"ESPO_TIME_ZONE":             "Europe/Moscow",
		"ESPO_LOGGER_LEVEL":          "INFO",
	}

	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	lines := make([]string, 0, len(keys))
	for _, key := range keys {
		lines = append(lines, key+"="+values[key])
	}

	envPath := filepath.Join(projectDir, ".env."+scope)
	if err := os.WriteFile(envPath, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	return projectDir
}
