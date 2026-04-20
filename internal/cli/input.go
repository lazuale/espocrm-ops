package cli

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/lazuale/espocrm-ops/internal/opsconfig"
	"github.com/spf13/cobra"
)

func normalizeContourFlag(flag string, value *string) error {
	return normalizeChoiceFlag(flag, value, "--scope must be dev or prod", "dev", "prod")
}

func normalizeDoctorScopeFlag(value *string) error {
	return normalizeChoiceFlag("--scope", value, "--scope must be dev, prod, or all", "dev", "prod", "all")
}

func normalizeChoiceFlag(flag string, value *string, message string, allowed ...string) error {
	*value = strings.TrimSpace(*value)
	for _, candidate := range allowed {
		if *value == candidate {
			return nil
		}
	}

	return usageError(errors.New(message))
}

func normalizeProjectContext(cmd *cobra.Command, projectDir, composeFile, envFile *string) error {
	*projectDir = strings.TrimSpace(*projectDir)
	if err := requireNonBlankFlag("--project-dir", *projectDir); err != nil {
		return err
	}

	projectAbs, err := filepath.Abs(filepath.Clean(*projectDir))
	if err != nil {
		return usageError(fmt.Errorf("resolve --project-dir: %w", err))
	}
	*projectDir = projectAbs

	if composeFile != nil {
		if err := normalizeOptionalStringFlag(cmd, "compose-file", composeFile); err != nil {
			return err
		}
		if *composeFile == "" {
			*composeFile = opsconfig.ResolveProjectPath(*projectDir, "compose.yaml")
		} else {
			*composeFile = opsconfig.ResolveProjectPath(*projectDir, *composeFile)
		}
	}

	if envFile != nil {
		if err := normalizeOptionalStringFlag(cmd, "env-file", envFile); err != nil {
			return err
		}
		if *envFile != "" {
			*envFile = opsconfig.ResolveProjectPath(*projectDir, *envFile)
		}
	}

	return nil
}

func normalizeOptionalAbsolutePathFlag(cmd *cobra.Command, flag string, value *string) error {
	if err := normalizeOptionalStringFlag(cmd, flag, value); err != nil {
		return err
	}
	if *value == "" {
		return nil
	}

	absPath, err := filepath.Abs(filepath.Clean(*value))
	if err != nil {
		return usageError(fmt.Errorf("resolve --%s: %w", flag, err))
	}
	*value = absPath

	return nil
}
