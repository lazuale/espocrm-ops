package config

import (
	"path/filepath"
	"strings"
)

func resolveOperationEnvPath(projectDir, scope, overridePath string) (string, string, error) {
	scope = strings.TrimSpace(scope)
	if !supportedContour(scope) {
		return "", "", UnsupportedContourError{Contour: scope}
	}

	overridePath = strings.TrimSpace(overridePath)
	if overridePath != "" {
		return filepath.Clean(overridePath), scope, nil
	}

	projectDir = strings.TrimSpace(projectDir)
	if projectDir == "" {
		return "", "", InvalidEnvFileError{Message: "project dir is required when env-file override is not provided"}
	}

	return filepath.Join(filepath.Clean(projectDir), ".env."+scope), scope, nil
}
