package env

import (
	"path/filepath"
	"strings"
)

type OperationEnv struct {
	FilePath        string
	ResolvedContour string
	Values          map[string]string
}

func (e OperationEnv) Value(key string) string {
	if e.Values == nil {
		return ""
	}

	return e.Values[key]
}

func (e OperationEnv) ComposeProject() string {
	return e.Value("COMPOSE_PROJECT_NAME")
}

func (e OperationEnv) DBStorageDir() string {
	return e.Value("DB_STORAGE_DIR")
}

func (e OperationEnv) ESPOStorageDir() string {
	return e.Value("ESPO_STORAGE_DIR")
}

func (e OperationEnv) BackupRoot() string {
	return e.Value("BACKUP_ROOT")
}

func ResolveProjectPath(projectDir, value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if filepath.IsAbs(value) {
		return filepath.Clean(value)
	}

	return filepath.Clean(filepath.Join(projectDir, strings.TrimPrefix(value, "./")))
}
