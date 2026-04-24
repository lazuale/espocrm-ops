package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadBackupConfigValid(t *testing.T) {
	projectDir := t.TempDir()
	composePath := filepath.Join(projectDir, "compose.yaml")
	envPath := filepath.Join(projectDir, ".env.prod")
	if err := os.WriteFile(composePath, []byte("services: {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(envPath, []byte(strings.Join([]string{
		"ESPO_CONTOUR=prod",
		"BACKUP_ROOT=./backups/prod",
		"ESPO_STORAGE_DIR=./runtime/prod/espo",
		"APP_SERVICES=espocrm,espocrm-daemon,espocrm-websocket",
		"DB_SERVICE=db",
		"DB_USER=espocrm",
		"DB_PASSWORD=db-secret",
		"DB_NAME=espocrm",
		"",
	}, "\n")), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadBackup(BackupRequest{
		Scope:      "prod",
		ProjectDir: projectDir,
	})
	if err != nil {
		t.Fatalf("LoadBackup failed: %v", err)
	}
	if cfg.ComposeFile != composePath {
		t.Fatalf("unexpected compose file: %s", cfg.ComposeFile)
	}
	if cfg.EnvFile != envPath {
		t.Fatalf("unexpected env file: %s", cfg.EnvFile)
	}
	if cfg.BackupRoot != filepath.Join(projectDir, "backups", "prod") {
		t.Fatalf("unexpected backup root: %s", cfg.BackupRoot)
	}
	if cfg.StorageDir != filepath.Join(projectDir, "runtime", "prod", "espo") {
		t.Fatalf("unexpected storage dir: %s", cfg.StorageDir)
	}
	if strings.Join(cfg.AppServices, ",") != "espocrm,espocrm-daemon,espocrm-websocket" {
		t.Fatalf("unexpected app services: %v", cfg.AppServices)
	}
	if cfg.DBService != "db" {
		t.Fatalf("unexpected db service: %s", cfg.DBService)
	}
	if cfg.DBPassword != "db-secret" {
		t.Fatalf("unexpected db password: %q", cfg.DBPassword)
	}
}

func TestLoadBackupConfigReadsDBPasswordFile(t *testing.T) {
	projectDir := t.TempDir()
	passwordPath := filepath.Join(projectDir, "db.password")
	if err := os.WriteFile(filepath.Join(projectDir, "compose.yaml"), []byte("services: {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(passwordPath, []byte("file-secret\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, ".env.dev"), []byte(strings.Join([]string{
		"ESPO_CONTOUR=dev",
		"BACKUP_ROOT=./backups/dev",
		"ESPO_STORAGE_DIR=./runtime/dev/espo",
		"APP_SERVICES=espocrm,espocrm-daemon,espocrm-websocket",
		"DB_SERVICE=db",
		"DB_USER=espocrm",
		"DB_PASSWORD_FILE=./db.password",
		"DB_NAME=espocrm",
		"",
	}, "\n")), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadBackup(BackupRequest{
		Scope:      "dev",
		ProjectDir: projectDir,
	})
	if err != nil {
		t.Fatalf("LoadBackup failed: %v", err)
	}
	if cfg.DBPassword != "file-secret" {
		t.Fatalf("unexpected db password: %q", cfg.DBPassword)
	}
}

func TestLoadBackupConfigUsesComposeFileFromEnv(t *testing.T) {
	projectDir := t.TempDir()
	customCompose := filepath.Join(projectDir, "compose.prod.yaml")
	if err := os.WriteFile(customCompose, []byte("services: {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, ".env.prod"), []byte(strings.Join([]string{
		"ESPO_CONTOUR=prod",
		"COMPOSE_FILE=./compose.prod.yaml",
		"BACKUP_ROOT=./backups/prod",
		"ESPO_STORAGE_DIR=./runtime/prod/espo",
		"APP_SERVICES=espocrm,espocrm-daemon,espocrm-websocket",
		"DB_SERVICE=db",
		"DB_USER=espocrm",
		"DB_PASSWORD=db-secret",
		"DB_NAME=espocrm",
		"",
	}, "\n")), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadBackup(BackupRequest{
		Scope:      "prod",
		ProjectDir: projectDir,
	})
	if err != nil {
		t.Fatalf("LoadBackup failed: %v", err)
	}
	if cfg.ComposeFile != customCompose {
		t.Fatalf("unexpected compose file: %s", cfg.ComposeFile)
	}
}

func TestLoadBackupConfigUsesAppServicesFromEnv(t *testing.T) {
	projectDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(projectDir, "compose.yaml"), []byte("services: {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, ".env.prod"), []byte(strings.Join([]string{
		"ESPO_CONTOUR=prod",
		"BACKUP_ROOT=./backups/prod",
		"ESPO_STORAGE_DIR=./runtime/prod/espo",
		"APP_SERVICES=web,worker",
		"DB_SERVICE=database",
		"DB_USER=espocrm",
		"DB_PASSWORD=db-secret",
		"DB_NAME=espocrm",
		"",
	}, "\n")), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadBackup(BackupRequest{
		Scope:      "prod",
		ProjectDir: projectDir,
	})
	if err != nil {
		t.Fatalf("LoadBackup failed: %v", err)
	}
	if strings.Join(cfg.AppServices, ",") != "web,worker" {
		t.Fatalf("unexpected app services: %v", cfg.AppServices)
	}
	if cfg.DBService != "database" {
		t.Fatalf("unexpected db service: %s", cfg.DBService)
	}
}

func TestLoadBackupConfigMissingDBServiceFails(t *testing.T) {
	projectDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(projectDir, "compose.yaml"), []byte("services: {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	envPath := filepath.Join(projectDir, ".env.prod")
	if err := os.WriteFile(envPath, []byte(strings.Join([]string{
		"ESPO_CONTOUR=prod",
		"BACKUP_ROOT=./backups/prod",
		"ESPO_STORAGE_DIR=./runtime/prod/espo",
		"APP_SERVICES=web,worker",
		"DB_USER=espocrm",
		"DB_PASSWORD=db-secret",
		"DB_NAME=espocrm",
		"",
	}, "\n")), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := LoadBackup(BackupRequest{
		Scope:      "prod",
		ProjectDir: projectDir,
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "DB_SERVICE is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLoadBackupConfigMissingAppServicesFails(t *testing.T) {
	projectDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(projectDir, "compose.yaml"), []byte("services: {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	envPath := filepath.Join(projectDir, ".env.prod")
	if err := os.WriteFile(envPath, []byte(strings.Join([]string{
		"ESPO_CONTOUR=prod",
		"BACKUP_ROOT=./backups/prod",
		"ESPO_STORAGE_DIR=./runtime/prod/espo",
		"DB_SERVICE=db",
		"DB_USER=espocrm",
		"DB_PASSWORD=db-secret",
		"DB_NAME=espocrm",
		"",
	}, "\n")), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := LoadBackup(BackupRequest{
		Scope:      "prod",
		ProjectDir: projectDir,
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "APP_SERVICES is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLoadBackupConfigMissingRequiredEnvValue(t *testing.T) {
	projectDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(projectDir, "compose.yaml"), []byte("services: {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	envPath := filepath.Join(projectDir, ".env.dev")
	if err := os.WriteFile(envPath, []byte(strings.Join([]string{
		"ESPO_CONTOUR=dev",
		"ESPO_STORAGE_DIR=./runtime/dev/espo",
		"APP_SERVICES=espocrm,espocrm-daemon,espocrm-websocket",
		"DB_SERVICE=db",
		"DB_USER=espocrm",
		"DB_PASSWORD=db-secret",
		"DB_NAME=espocrm",
		"",
	}, "\n")), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := LoadBackup(BackupRequest{
		Scope:      "dev",
		ProjectDir: projectDir,
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "BACKUP_ROOT is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}
