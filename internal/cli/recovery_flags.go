package cli

import (
	"fmt"
	"strings"

	journalusecase "github.com/lazuale/espocrm-ops/internal/usecase/journal"
	"github.com/spf13/cobra"
)

func normalizeRecoveryModeFlag(cmd *cobra.Command, flag string, value *string) error {
	trimmed := strings.TrimSpace(*value)
	if cmd.Flags().Changed(flag) && trimmed == "" {
		return usageError(fmt.Errorf("--%s must not be blank", flag))
	}
	if trimmed == "" {
		trimmed = journalusecase.RecoveryModeAuto
	}

	switch trimmed {
	case journalusecase.RecoveryModeAuto, journalusecase.RecoveryModeRetry, journalusecase.RecoveryModeResume:
	default:
		return usageError(fmt.Errorf("--%s must be auto, retry, or resume", flag))
	}

	*value = trimmed
	return nil
}

func rejectRecoveryOverrides(cmd *cobra.Command, recoveryFlag string, flags ...string) error {
	for _, flag := range flags {
		if cmd.Flags().Changed(flag) {
			return usageError(fmt.Errorf("--%s cannot be used with %s", flag, recoveryFlag))
		}
	}

	return nil
}
