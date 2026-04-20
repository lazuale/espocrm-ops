package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

func envDefault(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}

	return fallback
}

func currentProcessEnv() []string {
	return append([]string(nil), os.Environ()...)
}

func envFileContourHint() string {
	return strings.TrimSpace(os.Getenv("ESPO_ENV_FILE_CONTOUR"))
}

type backupExecutionConfig struct {
	BackupRoot     string
	StorageDir     string
	NamePrefix     string
	RetentionDays  int
	ComposeProject string
	DBUser         string
	DBPassword     string
	DBName         string
	EspoCRMImage   string
	MariaDBTag     string
}

func loadBackupExecutionConfigFromValues(projectDir string, values map[string]string) (backupExecutionConfig, error) {
	composeProject, err := requireMapValue(values, "COMPOSE_PROJECT_NAME")
	if err != nil {
		return backupExecutionConfig{}, err
	}

	backupRootRaw, err := requireMapValue(values, "BACKUP_ROOT")
	if err != nil {
		return backupExecutionConfig{}, err
	}
	storageDirRaw, err := requireMapValue(values, "ESPO_STORAGE_DIR")
	if err != nil {
		return backupExecutionConfig{}, err
	}

	retentionDays := 7
	if raw := strings.TrimSpace(values["BACKUP_RETENTION_DAYS"]); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil {
			return backupExecutionConfig{}, usageError(fmt.Errorf("BACKUP_RETENTION_DAYS must be an integer: %w", err))
		}
		if parsed < 0 {
			return backupExecutionConfig{}, usageError(fmt.Errorf("BACKUP_RETENTION_DAYS must be non-negative"))
		}
		retentionDays = parsed
	}

	namePrefix := strings.TrimSpace(values["BACKUP_NAME_PREFIX"])
	if namePrefix == "" {
		namePrefix = composeProject
	}

	return backupExecutionConfig{
		BackupRoot:     resolveProjectPath(projectDir, backupRootRaw),
		StorageDir:     resolveProjectPath(projectDir, storageDirRaw),
		NamePrefix:     namePrefix,
		RetentionDays:  retentionDays,
		ComposeProject: composeProject,
		DBUser:         strings.TrimSpace(values["DB_USER"]),
		DBPassword:     values["DB_PASSWORD"],
		DBName:         strings.TrimSpace(values["DB_NAME"]),
		EspoCRMImage:   strings.TrimSpace(values["ESPOCRM_IMAGE"]),
		MariaDBTag:     strings.TrimSpace(values["MARIADB_TAG"]),
	}, nil
}

func loadBackupExecutionConfig(projectDir string) (backupExecutionConfig, error) {
	values := map[string]string{}
	for _, entry := range os.Environ() {
		key, value, ok := strings.Cut(entry, "=")
		if !ok {
			continue
		}
		values[key] = value
	}

	return loadBackupExecutionConfigFromValues(projectDir, values)
}

func validateBackupExecutionConfig(cfg backupExecutionConfig, requireDB bool) error {
	if strings.TrimSpace(cfg.BackupRoot) == "" {
		return usageError(fmt.Errorf("BACKUP_ROOT must not be blank"))
	}
	if strings.TrimSpace(cfg.StorageDir) == "" {
		return usageError(fmt.Errorf("ESPO_STORAGE_DIR must not be blank"))
	}
	if strings.TrimSpace(cfg.NamePrefix) == "" {
		return usageError(fmt.Errorf("BACKUP_NAME_PREFIX resolved to blank"))
	}
	if strings.TrimSpace(cfg.ComposeProject) == "" {
		return usageError(fmt.Errorf("COMPOSE_PROJECT_NAME must not be blank"))
	}
	if cfg.RetentionDays < 0 {
		return usageError(fmt.Errorf("BACKUP_RETENTION_DAYS must be non-negative"))
	}
	if !requireDB {
		return nil
	}
	if strings.TrimSpace(cfg.DBUser) == "" {
		return usageError(fmt.Errorf("DB_USER is required"))
	}
	if cfg.DBPassword == "" {
		return usageError(fmt.Errorf("DB_PASSWORD is required"))
	}
	if strings.TrimSpace(cfg.DBName) == "" {
		return usageError(fmt.Errorf("DB_NAME is required"))
	}

	return nil
}

func requireEnvValue(name string) (string, error) {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return "", usageError(fmt.Errorf("%s is required", name))
	}

	return value, nil
}

func requireMapValue(values map[string]string, name string) (string, error) {
	value := strings.TrimSpace(values[name])
	if value == "" {
		return "", usageError(fmt.Errorf("%s is required", name))
	}

	return value, nil
}

func resolveProjectPath(projectDir, value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if filepath.IsAbs(value) {
		return filepath.Clean(value)
	}

	return filepath.Clean(filepath.Join(projectDir, strings.TrimPrefix(value, "./")))
}
