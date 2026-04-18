package cli

import (
	"errors"
	"fmt"

	"github.com/lazuale/espocrm-ops/internal/contract/exitcode"
	"github.com/lazuale/espocrm-ops/internal/contract/result"
	"github.com/spf13/cobra"
)

func ExecuteRoot(root *cobra.Command) int {
	if root == nil {
		return exitcode.InternalError
	}

	if err := root.Execute(); err != nil {
		return renderExecutionError(root, err)
	}

	return exitcode.OK
}

func renderExecutionError(root *cobra.Command, err error) int {
	fallbackExitCode := exitcode.InternalError
	fallbackErrorCode := "internal_error"
	if IsUsageError(err) {
		fallbackExitCode = exitcode.UsageError
		fallbackErrorCode = "usage_error"
	}

	if appForCommand(root).JSONEnabled() {
		var carrier resultCarrier
		if errors.As(err, &carrier) {
			exitCode := codeForError(err, fallbackExitCode)
			res := carrier.CommandResult()
			if res.Command == "" {
				res.Command = root.Name()
			}
			if res.Error == nil {
				res.Error = &result.ErrorInfo{
					Code:     errorCodeForError(err, fallbackErrorCode),
					Kind:     errorKindForError(err),
					ExitCode: exitCode,
					Message:  err.Error(),
				}
			}
			_ = result.Render(root.OutOrStdout(), res, true)
			return exitCode
		}
	}

	errorResult, exitCode := ErrorResult(root.Name(), err, fallbackExitCode, fallbackErrorCode)
	if appForCommand(root).JSONEnabled() {
		_ = result.Render(root.OutOrStdout(), errorResult, true)
		return exitCode
	}

	var suppressor textErrorSuppressor
	if errors.As(err, &suppressor) && suppressor.SuppressTextError() {
		return exitCode
	}

	fmt.Fprintf(root.ErrOrStderr(), "ERROR: %v\n", err)
	return exitCode
}

type textErrorSuppressor interface {
	SuppressTextError() bool
}
