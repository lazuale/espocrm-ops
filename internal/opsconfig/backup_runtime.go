package opsconfig

import (
	"fmt"
	"strconv"
	"strings"
)

type BackupExecutionConfig struct {
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

type MissingValueError struct {
	Path string
	Name string
}

func (e MissingValueError) Error() string {
	return fmt.Sprintf("%s is not set in %s", e.Name, e.Path)
}

type InvalidValueError struct {
	Path    string
	Message string
}

func (e InvalidValueError) Error() string {
	if strings.TrimSpace(e.Path) == "" {
		return e.Message
	}
	return fmt.Sprintf("%s: %s", e.Message, e.Path)
}

func LoadBackupExecutionConfig(projectDir, envFilePath string, values map[string]string, requireDB bool) (BackupExecutionConfig, error) {
	composeProject, err := requiredValue(envFilePath, values, "COMPOSE_PROJECT_NAME")
	if err != nil {
		return BackupExecutionConfig{}, err
	}

	backupRootRaw, err := requiredValue(envFilePath, values, "BACKUP_ROOT")
	if err != nil {
		return BackupExecutionConfig{}, err
	}
	storageDirRaw, err := requiredValue(envFilePath, values, "ESPO_STORAGE_DIR")
	if err != nil {
		return BackupExecutionConfig{}, err
	}

	retentionDays := 7
	if raw := strings.TrimSpace(values["BACKUP_RETENTION_DAYS"]); raw != "" {
		parsed, parseErr := strconv.Atoi(raw)
		if parseErr != nil {
			return BackupExecutionConfig{}, InvalidValueError{
				Path:    envFilePath,
				Message: fmt.Sprintf("BACKUP_RETENTION_DAYS must be an integer: %v", parseErr),
			}
		}
		if parsed < 0 {
			return BackupExecutionConfig{}, InvalidValueError{
				Path:    envFilePath,
				Message: "BACKUP_RETENTION_DAYS must be non-negative",
			}
		}
		retentionDays = parsed
	}

	namePrefix := strings.TrimSpace(values["BACKUP_NAME_PREFIX"])
	if namePrefix == "" {
		namePrefix = composeProject
	}

	cfg := BackupExecutionConfig{
		BackupRoot:     ResolveProjectPath(projectDir, backupRootRaw),
		StorageDir:     ResolveProjectPath(projectDir, storageDirRaw),
		NamePrefix:     namePrefix,
		RetentionDays:  retentionDays,
		ComposeProject: composeProject,
		DBUser:         strings.TrimSpace(values["DB_USER"]),
		DBPassword:     values["DB_PASSWORD"],
		DBName:         strings.TrimSpace(values["DB_NAME"]),
		EspoCRMImage:   strings.TrimSpace(values["ESPOCRM_IMAGE"]),
		MariaDBTag:     strings.TrimSpace(values["MARIADB_TAG"]),
	}

	if requireDB {
		if strings.TrimSpace(cfg.DBUser) == "" {
			return BackupExecutionConfig{}, MissingValueError{Path: envFilePath, Name: "DB_USER"}
		}
		if strings.TrimSpace(cfg.DBPassword) == "" {
			return BackupExecutionConfig{}, MissingValueError{Path: envFilePath, Name: "DB_PASSWORD"}
		}
		if strings.TrimSpace(cfg.DBName) == "" {
			return BackupExecutionConfig{}, MissingValueError{Path: envFilePath, Name: "DB_NAME"}
		}
	}

	return cfg, nil
}

func requiredValue(envFilePath string, values map[string]string, name string) (string, error) {
	value := strings.TrimSpace(values[name])
	if value == "" {
		return "", MissingValueError{Path: envFilePath, Name: name}
	}

	return value, nil
}
