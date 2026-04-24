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
		"BACKUP_NAME_PREFIX=test-backup",
		"BACKUP_RETENTION_DAYS=7",
		"MIN_FREE_DISK_MB=1",
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
	if cfg.BackupNamePrefix != "test-backup" {
		t.Fatalf("unexpected backup name prefix: %s", cfg.BackupNamePrefix)
	}
	if cfg.BackupRetentionDays != 7 {
		t.Fatalf("unexpected backup retention: %d", cfg.BackupRetentionDays)
	}
	if cfg.MinFreeDiskMB != 1 {
		t.Fatalf("unexpected min free disk: %d", cfg.MinFreeDiskMB)
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
	if cfg.RuntimeOwnershipConfigured {
		t.Fatal("expected runtime ownership to be optional for backup config")
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
		"BACKUP_NAME_PREFIX=test-backup",
		"BACKUP_RETENTION_DAYS=7",
		"MIN_FREE_DISK_MB=1",
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

func TestLoadRestoreConfigRequiresDBRootPassword(t *testing.T) {
	projectDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(projectDir, "compose.yaml"), []byte("services: {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, ".env.prod"), []byte(strings.Join([]string{
		"ESPO_CONTOUR=prod",
		"BACKUP_ROOT=./backups/prod",
		"BACKUP_NAME_PREFIX=test-backup",
		"BACKUP_RETENTION_DAYS=7",
		"MIN_FREE_DISK_MB=1",
		"ESPO_STORAGE_DIR=./runtime/prod/espo",
		"ESPO_RUNTIME_UID=33",
		"ESPO_RUNTIME_GID=33",
		"APP_SERVICES=espocrm,espocrm-daemon,espocrm-websocket",
		"DB_SERVICE=db",
		"DB_USER=espocrm",
		"DB_PASSWORD=db-secret",
		"DB_NAME=espocrm",
		"",
	}, "\n")), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := LoadRestore(BackupRequest{
		Scope:      "prod",
		ProjectDir: projectDir,
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "DB_ROOT_PASSWORD or DB_ROOT_PASSWORD_FILE is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLoadRestoreConfigReadsDBRootPasswordFile(t *testing.T) {
	projectDir := t.TempDir()
	rootPasswordPath := filepath.Join(projectDir, "db.root.password")
	if err := os.WriteFile(filepath.Join(projectDir, "compose.yaml"), []byte("services: {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(rootPasswordPath, []byte("root-secret\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, ".env.prod"), []byte(strings.Join([]string{
		"ESPO_CONTOUR=prod",
		"BACKUP_ROOT=./backups/prod",
		"BACKUP_NAME_PREFIX=test-backup",
		"BACKUP_RETENTION_DAYS=7",
		"MIN_FREE_DISK_MB=1",
		"ESPO_STORAGE_DIR=./runtime/prod/espo",
		"ESPO_RUNTIME_UID=33",
		"ESPO_RUNTIME_GID=44",
		"APP_SERVICES=espocrm,espocrm-daemon,espocrm-websocket",
		"DB_SERVICE=db",
		"DB_USER=espocrm",
		"DB_PASSWORD=db-secret",
		"DB_ROOT_PASSWORD_FILE=./db.root.password",
		"DB_NAME=espocrm",
		"",
	}, "\n")), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadRestore(BackupRequest{
		Scope:      "prod",
		ProjectDir: projectDir,
	})
	if err != nil {
		t.Fatalf("LoadRestore failed: %v", err)
	}
	if cfg.DBRootPassword != "root-secret" {
		t.Fatalf("unexpected db root password: %q", cfg.DBRootPassword)
	}
	if !cfg.RuntimeOwnershipConfigured {
		t.Fatal("expected runtime ownership to be configured")
	}
	if cfg.RuntimeUID != 33 || cfg.RuntimeGID != 44 {
		t.Fatalf("unexpected runtime ownership: %d:%d", cfg.RuntimeUID, cfg.RuntimeGID)
	}
}

func TestLoadRestoreConfigRejectsInlineAndFileDBRootPassword(t *testing.T) {
	projectDir := t.TempDir()
	rootPasswordPath := filepath.Join(projectDir, "db.root.password")
	if err := os.WriteFile(filepath.Join(projectDir, "compose.yaml"), []byte("services: {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(rootPasswordPath, []byte("root-secret\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, ".env.prod"), []byte(strings.Join([]string{
		"ESPO_CONTOUR=prod",
		"BACKUP_ROOT=./backups/prod",
		"BACKUP_NAME_PREFIX=test-backup",
		"BACKUP_RETENTION_DAYS=7",
		"MIN_FREE_DISK_MB=1",
		"ESPO_STORAGE_DIR=./runtime/prod/espo",
		"ESPO_RUNTIME_UID=33",
		"ESPO_RUNTIME_GID=33",
		"APP_SERVICES=espocrm,espocrm-daemon,espocrm-websocket",
		"DB_SERVICE=db",
		"DB_USER=espocrm",
		"DB_PASSWORD=db-secret",
		"DB_ROOT_PASSWORD=root-secret",
		"DB_ROOT_PASSWORD_FILE=./db.root.password",
		"DB_NAME=espocrm",
		"",
	}, "\n")), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := LoadRestore(BackupRequest{
		Scope:      "prod",
		ProjectDir: projectDir,
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "only one of DB_ROOT_PASSWORD or DB_ROOT_PASSWORD_FILE may be set") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLoadRestoreConfigRequiresRuntimeOwnership(t *testing.T) {
	projectDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(projectDir, "compose.yaml"), []byte("services: {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, ".env.prod"), []byte(strings.Join([]string{
		"ESPO_CONTOUR=prod",
		"BACKUP_ROOT=./backups/prod",
		"BACKUP_NAME_PREFIX=test-backup",
		"BACKUP_RETENTION_DAYS=7",
		"MIN_FREE_DISK_MB=1",
		"ESPO_STORAGE_DIR=./runtime/prod/espo",
		"APP_SERVICES=espocrm,espocrm-daemon,espocrm-websocket",
		"DB_SERVICE=db",
		"DB_USER=espocrm",
		"DB_PASSWORD=db-secret",
		"DB_ROOT_PASSWORD=root-secret",
		"DB_NAME=espocrm",
		"",
	}, "\n")), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := LoadRestore(BackupRequest{
		Scope:      "prod",
		ProjectDir: projectDir,
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "ESPO_RUNTIME_UID and ESPO_RUNTIME_GID are required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLoadRestoreConfigRejectsNegativeRuntimeUID(t *testing.T) {
	projectDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(projectDir, "compose.yaml"), []byte("services: {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, ".env.prod"), []byte(strings.Join([]string{
		"ESPO_CONTOUR=prod",
		"BACKUP_ROOT=./backups/prod",
		"BACKUP_NAME_PREFIX=test-backup",
		"BACKUP_RETENTION_DAYS=7",
		"MIN_FREE_DISK_MB=1",
		"ESPO_STORAGE_DIR=./runtime/prod/espo",
		"ESPO_RUNTIME_UID=-1",
		"ESPO_RUNTIME_GID=33",
		"APP_SERVICES=espocrm,espocrm-daemon,espocrm-websocket",
		"DB_SERVICE=db",
		"DB_USER=espocrm",
		"DB_PASSWORD=db-secret",
		"DB_ROOT_PASSWORD=root-secret",
		"DB_NAME=espocrm",
		"",
	}, "\n")), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := LoadRestore(BackupRequest{
		Scope:      "prod",
		ProjectDir: projectDir,
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "ESPO_RUNTIME_UID") || !strings.Contains(err.Error(), "integer >= 0") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLoadRestoreConfigRejectsNonNumericRuntimeGID(t *testing.T) {
	projectDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(projectDir, "compose.yaml"), []byte("services: {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, ".env.prod"), []byte(strings.Join([]string{
		"ESPO_CONTOUR=prod",
		"BACKUP_ROOT=./backups/prod",
		"BACKUP_NAME_PREFIX=test-backup",
		"BACKUP_RETENTION_DAYS=7",
		"MIN_FREE_DISK_MB=1",
		"ESPO_STORAGE_DIR=./runtime/prod/espo",
		"ESPO_RUNTIME_UID=33",
		"ESPO_RUNTIME_GID=group",
		"APP_SERVICES=espocrm,espocrm-daemon,espocrm-websocket",
		"DB_SERVICE=db",
		"DB_USER=espocrm",
		"DB_PASSWORD=db-secret",
		"DB_ROOT_PASSWORD=root-secret",
		"DB_NAME=espocrm",
		"",
	}, "\n")), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := LoadRestore(BackupRequest{
		Scope:      "prod",
		ProjectDir: projectDir,
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "ESPO_RUNTIME_GID") || !strings.Contains(err.Error(), "integer >= 0") {
		t.Fatalf("unexpected error: %v", err)
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
		"BACKUP_NAME_PREFIX=test-backup",
		"BACKUP_RETENTION_DAYS=7",
		"MIN_FREE_DISK_MB=1",
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
		"BACKUP_NAME_PREFIX=test-backup",
		"BACKUP_RETENTION_DAYS=7",
		"MIN_FREE_DISK_MB=1",
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
		"BACKUP_NAME_PREFIX=test-backup",
		"BACKUP_RETENTION_DAYS=7",
		"MIN_FREE_DISK_MB=1",
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
		"BACKUP_NAME_PREFIX=test-backup",
		"BACKUP_RETENTION_DAYS=7",
		"MIN_FREE_DISK_MB=1",
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

func TestLoadBackupConfigParsesBackupNamePrefix(t *testing.T) {
	projectDir := t.TempDir()
	writeBackupConfigEnv(t, projectDir, ".env.prod", []string{
		"ESPO_CONTOUR=prod",
		"BACKUP_ROOT=./backups/prod",
		"BACKUP_NAME_PREFIX=prod.snapshot-01",
		"BACKUP_RETENTION_DAYS=7",
		"MIN_FREE_DISK_MB=128",
		"ESPO_STORAGE_DIR=./runtime/prod/espo",
		"APP_SERVICES=web,worker",
		"DB_SERVICE=db",
		"DB_USER=espocrm",
		"DB_PASSWORD=db-secret",
		"DB_NAME=espocrm",
	})

	cfg, err := LoadBackup(BackupRequest{
		Scope:      "prod",
		ProjectDir: projectDir,
	})
	if err != nil {
		t.Fatalf("LoadBackup failed: %v", err)
	}
	if cfg.BackupNamePrefix != "prod.snapshot-01" {
		t.Fatalf("unexpected backup name prefix: %s", cfg.BackupNamePrefix)
	}
}

func TestLoadBackupConfigMissingBackupNamePrefixFails(t *testing.T) {
	projectDir := t.TempDir()
	writeBackupConfigEnv(t, projectDir, ".env.prod", []string{
		"ESPO_CONTOUR=prod",
		"BACKUP_ROOT=./backups/prod",
		"BACKUP_RETENTION_DAYS=7",
		"MIN_FREE_DISK_MB=128",
		"ESPO_STORAGE_DIR=./runtime/prod/espo",
		"APP_SERVICES=web,worker",
		"DB_SERVICE=db",
		"DB_USER=espocrm",
		"DB_PASSWORD=db-secret",
		"DB_NAME=espocrm",
	})

	_, err := LoadBackup(BackupRequest{
		Scope:      "prod",
		ProjectDir: projectDir,
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "BACKUP_NAME_PREFIX is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLoadBackupConfigRejectsUnsafeBackupNamePrefix(t *testing.T) {
	testCases := []struct {
		name    string
		envLine string
	}{
		{name: "path_traversal", envLine: "BACKUP_NAME_PREFIX=../prod"},
		{name: "slash", envLine: "BACKUP_NAME_PREFIX=bad/name"},
		{name: "space", envLine: "BACKUP_NAME_PREFIX=\"bad name\""},
		{name: "dot", envLine: "BACKUP_NAME_PREFIX=."},
		{name: "dotdot", envLine: "BACKUP_NAME_PREFIX=.."},
		{name: "too_long", envLine: "BACKUP_NAME_PREFIX=" + strings.Repeat("a", 81)},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			projectDir := t.TempDir()
			writeBackupConfigEnv(t, projectDir, ".env.prod", []string{
				"ESPO_CONTOUR=prod",
				"BACKUP_ROOT=./backups/prod",
				tc.envLine,
				"BACKUP_RETENTION_DAYS=7",
				"MIN_FREE_DISK_MB=128",
				"ESPO_STORAGE_DIR=./runtime/prod/espo",
				"APP_SERVICES=web,worker",
				"DB_SERVICE=db",
				"DB_USER=espocrm",
				"DB_PASSWORD=db-secret",
				"DB_NAME=espocrm",
			})

			_, err := LoadBackup(BackupRequest{
				Scope:      "prod",
				ProjectDir: projectDir,
			})
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), "BACKUP_NAME_PREFIX") {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestLoadBackupConfigMissingMinFreeDiskFails(t *testing.T) {
	projectDir := t.TempDir()
	writeBackupConfigEnv(t, projectDir, ".env.prod", []string{
		"ESPO_CONTOUR=prod",
		"BACKUP_ROOT=./backups/prod",
		"BACKUP_NAME_PREFIX=test-backup",
		"BACKUP_RETENTION_DAYS=7",
		"ESPO_STORAGE_DIR=./runtime/prod/espo",
		"APP_SERVICES=web,worker",
		"DB_SERVICE=db",
		"DB_USER=espocrm",
		"DB_PASSWORD=db-secret",
		"DB_NAME=espocrm",
	})

	_, err := LoadBackup(BackupRequest{
		Scope:      "prod",
		ProjectDir: projectDir,
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "MIN_FREE_DISK_MB is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLoadBackupConfigRejectsInvalidMinFreeDisk(t *testing.T) {
	for _, value := range []string{"0", "-1", "abc"} {
		t.Run(value, func(t *testing.T) {
			projectDir := t.TempDir()
			writeBackupConfigEnv(t, projectDir, ".env.prod", []string{
				"ESPO_CONTOUR=prod",
				"BACKUP_ROOT=./backups/prod",
				"BACKUP_NAME_PREFIX=test-backup",
				"BACKUP_RETENTION_DAYS=7",
				"MIN_FREE_DISK_MB=" + value,
				"ESPO_STORAGE_DIR=./runtime/prod/espo",
				"APP_SERVICES=web,worker",
				"DB_SERVICE=db",
				"DB_USER=espocrm",
				"DB_PASSWORD=db-secret",
				"DB_NAME=espocrm",
			})

			_, err := LoadBackup(BackupRequest{
				Scope:      "prod",
				ProjectDir: projectDir,
			})
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), "MIN_FREE_DISK_MB") {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestLoadBackupConfigMissingBackupRetentionDaysFails(t *testing.T) {
	projectDir := t.TempDir()
	writeBackupConfigEnv(t, projectDir, ".env.prod", []string{
		"ESPO_CONTOUR=prod",
		"BACKUP_ROOT=./backups/prod",
		"BACKUP_NAME_PREFIX=test-backup",
		"MIN_FREE_DISK_MB=128",
		"ESPO_STORAGE_DIR=./runtime/prod/espo",
		"APP_SERVICES=web,worker",
		"DB_SERVICE=db",
		"DB_USER=espocrm",
		"DB_PASSWORD=db-secret",
		"DB_NAME=espocrm",
	})

	_, err := LoadBackup(BackupRequest{
		Scope:      "prod",
		ProjectDir: projectDir,
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "BACKUP_RETENTION_DAYS is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLoadBackupConfigRejectsInvalidBackupRetentionDays(t *testing.T) {
	for _, value := range []string{"-1", "abc"} {
		t.Run(value, func(t *testing.T) {
			projectDir := t.TempDir()
			writeBackupConfigEnv(t, projectDir, ".env.prod", []string{
				"ESPO_CONTOUR=prod",
				"BACKUP_ROOT=./backups/prod",
				"BACKUP_NAME_PREFIX=test-backup",
				"BACKUP_RETENTION_DAYS=" + value,
				"MIN_FREE_DISK_MB=128",
				"ESPO_STORAGE_DIR=./runtime/prod/espo",
				"APP_SERVICES=web,worker",
				"DB_SERVICE=db",
				"DB_USER=espocrm",
				"DB_PASSWORD=db-secret",
				"DB_NAME=espocrm",
			})

			_, err := LoadBackup(BackupRequest{
				Scope:      "prod",
				ProjectDir: projectDir,
			})
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), "BACKUP_RETENTION_DAYS") {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestLoadBackupConfigAllowsZeroBackupRetentionDays(t *testing.T) {
	projectDir := t.TempDir()
	writeBackupConfigEnv(t, projectDir, ".env.prod", []string{
		"ESPO_CONTOUR=prod",
		"BACKUP_ROOT=./backups/prod",
		"BACKUP_NAME_PREFIX=test-backup",
		"BACKUP_RETENTION_DAYS=0",
		"MIN_FREE_DISK_MB=128",
		"ESPO_STORAGE_DIR=./runtime/prod/espo",
		"APP_SERVICES=web,worker",
		"DB_SERVICE=db",
		"DB_USER=espocrm",
		"DB_PASSWORD=db-secret",
		"DB_NAME=espocrm",
	})

	cfg, err := LoadBackup(BackupRequest{
		Scope:      "prod",
		ProjectDir: projectDir,
	})
	if err != nil {
		t.Fatalf("LoadBackup failed: %v", err)
	}
	if cfg.BackupRetentionDays != 0 {
		t.Fatalf("unexpected backup retention: %d", cfg.BackupRetentionDays)
	}
}

func writeBackupConfigEnv(t *testing.T, projectDir, envName string, lines []string) {
	t.Helper()

	if err := os.WriteFile(filepath.Join(projectDir, "compose.yaml"), []byte("services: {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, envName), []byte(strings.Join(append(lines, ""), "\n")), 0o644); err != nil {
		t.Fatal(err)
	}
}
