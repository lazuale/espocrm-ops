package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadConfigMissingEnv(t *testing.T) {
	_, err := LoadConfig(filepath.Join(t.TempDir(), ".env"))
	if err == nil || !strings.Contains(err.Error(), "missing .env") {
		t.Fatalf("expected missing .env error, got %v", err)
	}
}

func TestLoadConfigMissingRequiredKey(t *testing.T) {
	path := writeEnv(t, map[string]string{
		"BACKUP_ROOT":      filepath.Join(t.TempDir(), "backups"),
		"ESPO_STORAGE_DIR": filepath.Join(t.TempDir(), "storage"),
		"DB_USER":          "espocrm",
		"DB_PASSWORD":      "secret",
		"DB_ROOT_PASSWORD": "rootsecret",
	})
	_, err := LoadConfig(path)
	if err == nil || !strings.Contains(err.Error(), "DB_NAME") {
		t.Fatalf("expected missing DB_NAME error, got %v", err)
	}
}

func TestLoadConfigEmptyRequiredKey(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	data := "BACKUP_ROOT=" + filepath.Join(dir, "backups") + "\n" +
		"ESPO_STORAGE_DIR=" + filepath.Join(dir, "storage") + "\n" +
		"DB_USER=\n" +
		"DB_PASSWORD=secret\n" +
		"DB_ROOT_PASSWORD=rootsecret\n" +
		"DB_NAME=espocrm\n"
	if err := os.WriteFile(path, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	_, err := LoadConfig(path)
	if err == nil || !strings.Contains(err.Error(), "DB_USER") {
		t.Fatalf("expected empty DB_USER error, got %v", err)
	}
}

func TestLoadConfigDuplicateKey(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	data := "BACKUP_ROOT=" + filepath.Join(dir, "backups") + "\n" +
		"BACKUP_ROOT=" + filepath.Join(dir, "other") + "\n" +
		"ESPO_STORAGE_DIR=" + filepath.Join(dir, "storage") + "\n" +
		"DB_USER=espocrm\n" +
		"DB_PASSWORD=secret\n" +
		"DB_ROOT_PASSWORD=rootsecret\n" +
		"DB_NAME=espocrm\n"
	if err := os.WriteFile(path, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	_, err := LoadConfig(path)
	if err == nil || !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("expected duplicate key error, got %v", err)
	}
}

func TestLoadConfigValidExample(t *testing.T) {
	dir := t.TempDir()
	storage := filepath.Join(dir, "storage")
	backupRoot := filepath.Join(dir, "backups")
	path := writeEnv(t, map[string]string{
		"BACKUP_ROOT":      backupRoot,
		"ESPO_STORAGE_DIR": storage,
		"DB_USER":          "espocrm",
		"DB_PASSWORD":      "secret",
		"DB_ROOT_PASSWORD": "rootsecret",
		"DB_NAME":          "espocrm_1",
	})
	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig returned error: %v", err)
	}
	if cfg.DBName != "espocrm_1" || cfg.DBUser != "espocrm" || cfg.BackupRoot != backupRoot || cfg.EspoStorageDir != storage {
		t.Fatalf("unexpected config: %#v", cfg)
	}
}

func TestValidateDBName(t *testing.T) {
	valid := []string{"espocrm", "espo_1", "A1_"}
	for _, name := range valid {
		if err := validateDBName(name); err != nil {
			t.Fatalf("expected %q valid: %v", name, err)
		}
	}
	invalid := []string{"", "espo crm", "espo;crm", "espo`crm", "espo'crm", "espo-crm"}
	for _, name := range invalid {
		if err := validateDBName(name); err == nil {
			t.Fatalf("expected %q invalid", name)
		}
	}
}

func TestPathSafety(t *testing.T) {
	if _, err := validateStoragePath(""); err == nil {
		t.Fatal("expected empty storage path rejected")
	}
	if _, err := validateStoragePath("/"); err == nil {
		t.Fatal("expected root storage path rejected")
	}
	if _, err := validateStoragePath("."); err == nil {
		t.Fatal("expected dot storage path rejected")
	}
	if home, err := os.UserHomeDir(); err == nil {
		if _, err := validateStoragePath(home); err == nil {
			t.Fatal("expected home storage path rejected")
		}
	}

	dir := t.TempDir()
	storage, err := validateStoragePath(filepath.Join(dir, "storage"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := validateBackupRootPath(storage, storage); err == nil {
		t.Fatal("expected backup root equal to storage rejected")
	}
	if _, err := validateBackupRootPath(filepath.Join(storage, "backups"), storage); err == nil {
		t.Fatal("expected backup root inside storage rejected")
	}
	if _, err := validateBackupRootPath("/", storage); err == nil {
		t.Fatal("expected root backup path rejected")
	}
}

func writeEnv(t *testing.T, values map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	keys := []string{"BACKUP_ROOT", "ESPO_STORAGE_DIR", "DB_USER", "DB_PASSWORD", "DB_ROOT_PASSWORD", "DB_NAME"}
	var b strings.Builder
	b.WriteString("# test config\n\n")
	for _, key := range keys {
		if value, ok := values[key]; ok {
			b.WriteString(key)
			b.WriteString("=")
			b.WriteString(value)
			b.WriteString("\n")
		}
	}
	if err := os.WriteFile(path, []byte(b.String()), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}
