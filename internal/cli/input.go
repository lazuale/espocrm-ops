package cli

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	errortransport "github.com/lazuale/espocrm-ops/internal/cli/errortransport"
	"github.com/lazuale/espocrm-ops/internal/opsconfig"
	"github.com/spf13/cobra"
)

func usageError(err error) error {
	return errortransport.UsageError(err)
}

func requiredFlagError(flag string) error {
	return usageError(fmt.Errorf("%s is required", flag))
}

func requireNonBlankFlag(flag, value string) error {
	if strings.TrimSpace(value) == "" {
		return requiredFlagError(flag)
	}

	return nil
}

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

func normalizeOptionalStringFlag(cmd *cobra.Command, flag string, value *string) error {
	trimmed := strings.TrimSpace(*value)
	if cmd.Flags().Changed(flag) && trimmed == "" {
		return usageError(fmt.Errorf("--%s must not be blank", flag))
	}

	*value = trimmed
	return nil
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

func normalizeConfirmProdFlag(cmd *cobra.Command, value *string) error {
	if err := normalizeOptionalStringFlag(cmd, "confirm-prod", value); err != nil {
		return err
	}
	if *value != "" && *value != "prod" {
		return usageError(fmt.Errorf("--confirm-prod accepts only the value prod"))
	}

	return nil
}

func noArgs(cmd *cobra.Command, args []string) error {
	if err := cobra.NoArgs(cmd, args); err != nil {
		return usageError(err)
	}

	return nil
}
