package opsconfig

import (
	"path/filepath"
	"strings"
)

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
