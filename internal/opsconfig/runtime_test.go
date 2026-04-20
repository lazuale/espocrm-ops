package opsconfig

import (
	"errors"
	"path/filepath"
	"testing"
)

func TestLoadBackupExecutionConfigResolvesFromValues(t *testing.T) {
	projectDir := filepath.Join(string(filepath.Separator), "srv", "espocrm")
	cfg, err := LoadBackupExecutionConfig(projectDir, filepath.Join(projectDir, ".env.prod"), map[string]string{
		"COMPOSE_PROJECT_NAME":  "espocrm-prod",
		"BACKUP_ROOT":           "./backups/prod",
		"ESPO_STORAGE_DIR":      "./runtime/prod/espo",
		"BACKUP_RETENTION_DAYS": "14",
		"DB_USER":               "espocrm",
		"DB_PASSWORD":           "secret",
		"DB_NAME":               "espocrm",
		"ESPOCRM_IMAGE":         "espocrm/espocrm:9.3.4-apache",
		"MARIADB_TAG":           "10.11",
	}, true)
	if err != nil {
		t.Fatal(err)
	}

	if cfg.BackupRoot != filepath.Join(projectDir, "backups", "prod") {
		t.Fatalf("unexpected backup root: %s", cfg.BackupRoot)
	}
	if cfg.StorageDir != filepath.Join(projectDir, "runtime", "prod", "espo") {
		t.Fatalf("unexpected storage dir: %s", cfg.StorageDir)
	}
	if cfg.NamePrefix != "espocrm-prod" {
		t.Fatalf("unexpected name prefix: %s", cfg.NamePrefix)
	}
	if cfg.RetentionDays != 14 {
		t.Fatalf("unexpected retention days: %d", cfg.RetentionDays)
	}
}

func TestLoadBackupExecutionConfigRejectsInvalidRetentionDays(t *testing.T) {
	_, err := LoadBackupExecutionConfig("/tmp/project", "/tmp/project/.env.dev", map[string]string{
		"COMPOSE_PROJECT_NAME":  "espocrm-dev",
		"BACKUP_ROOT":           "./backups/dev",
		"ESPO_STORAGE_DIR":      "./runtime/dev/espo",
		"BACKUP_RETENTION_DAYS": "not-a-number",
	}, false)
	if err == nil {
		t.Fatal("expected invalid retention days error")
	}

	var typedErr InvalidValueError
	if !errors.As(err, &typedErr) {
		t.Fatalf("expected InvalidValueError, got %T: %v", err, err)
	}
}
