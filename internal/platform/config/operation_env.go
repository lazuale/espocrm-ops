package config

import (
	"strings"

	domainenv "github.com/lazuale/espocrm-ops/internal/domain/env"
)

var requiredOperationEnvKeys = []string{
	"COMPOSE_PROJECT_NAME",
	"DB_STORAGE_DIR",
	"ESPO_STORAGE_DIR",
	"BACKUP_ROOT",
	"ESPO_HELPER_IMAGE",
	"ESPO_RUNTIME_UID",
	"ESPO_RUNTIME_GID",
}

func LoadOperationEnv(projectDir, scope, overridePath string) (domainenv.OperationEnv, error) {
	resolvedPath, resolvedScope, err := resolveOperationEnvPath(projectDir, scope, overridePath)
	if err != nil {
		return domainenv.OperationEnv{}, err
	}

	if err := validateEnvFileForLoading(resolvedPath); err != nil {
		return domainenv.OperationEnv{}, err
	}

	values, err := loadEnvAssignments(resolvedPath)
	if err != nil {
		return domainenv.OperationEnv{}, err
	}

	if err := validateRequiredOperationEnvValues(resolvedPath, values); err != nil {
		return domainenv.OperationEnv{}, err
	}

	resolvedContour, err := resolveLoadedEnvContour(resolvedPath, resolvedScope, values["ESPO_CONTOUR"])
	if err != nil {
		return domainenv.OperationEnv{}, err
	}

	return domainenv.OperationEnv{
		FilePath:        resolvedPath,
		ResolvedContour: resolvedContour,
		Values:          values,
	}, nil
}

func validateRequiredOperationEnvValues(path string, values map[string]string) error {
	for _, key := range requiredOperationEnvKeys {
		if strings.TrimSpace(values[key]) == "" {
			return MissingEnvValueError{Path: path, Name: key}
		}
	}

	return nil
}
