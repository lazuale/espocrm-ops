package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadConfigValid(t *testing.T) {
	projectDir := writeProject(t, []string{
		"BACKUP_ROOT=backups",
		"ESPO_STORAGE_DIR=storage",
		"APP_SERVICES=espocrm,espocrm-daemon",
		"DB_SERVICE=db",
		"DB_USER=espocrm",
		"DB_PASSWORD=dbpass",
		"DB_ROOT_PASSWORD=rootpass",
		"DB_NAME=espocrm",
	})

	cfg, err := Load(Request{Scope: "prod", ProjectDir: projectDir})
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if cfg.Scope != "prod" {
		t.Fatalf("unexpected scope: %s", cfg.Scope)
	}
	if cfg.BackupRoot != filepath.Join(projectDir, "backups") {
		t.Fatalf("unexpected backup root: %s", cfg.BackupRoot)
	}
	if strings.Join(cfg.AppServices, ",") != "espocrm,espocrm-daemon" {
		t.Fatalf("unexpected app services: %#v", cfg.AppServices)
	}
}

func TestLoadConfigRejectsUnknownKey(t *testing.T) {
	projectDir := writeProject(t, []string{
		"BACKUP_ROOT=backups",
		"ESPO_STORAGE_DIR=storage",
		"APP_SERVICES=espocrm",
		"DB_SERVICE=db",
		"DB_USER=espocrm",
		"DB_PASSWORD=dbpass",
		"DB_ROOT_PASSWORD=rootpass",
		"DB_NAME=espocrm",
		"BACKUP_RETENTION_DAYS=7",
	})

	_, err := Load(Request{Scope: "prod", ProjectDir: projectDir})
	if err == nil || !strings.Contains(err.Error(), "unknown key BACKUP_RETENTION_DAYS") {
		t.Fatalf("expected unknown key error, got %v", err)
	}
}

func TestLoadConfigRejectsShellLikeEnv(t *testing.T) {
	projectDir := writeProject(t, []string{
		"BACKUP_ROOT=backups",
		"ESPO_STORAGE_DIR=${STORAGE}",
		"APP_SERVICES=espocrm",
		"DB_SERVICE=db",
		"DB_USER=espocrm",
		"DB_PASSWORD=dbpass",
		"DB_ROOT_PASSWORD=rootpass",
		"DB_NAME=espocrm",
	})

	_, err := Load(Request{Scope: "prod", ProjectDir: projectDir})
	if err == nil || !strings.Contains(err.Error(), "shell expansion") {
		t.Fatalf("expected shell expansion error, got %v", err)
	}
}

func writeProject(t *testing.T, env []string) string {
	t.Helper()

	projectDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(projectDir, "compose.yaml"), []byte("services: {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(projectDir, "storage"), 0o755); err != nil {
		t.Fatal(err)
	}
	body := strings.Join(env, "\n") + "\n"
	if err := os.WriteFile(filepath.Join(projectDir, ".env.prod"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return projectDir
}
