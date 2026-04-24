package cli

import (
	"errors"
	"fmt"

	errortransport "github.com/lazuale/espocrm-ops/internal/cli/errortransport"
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
	if errortransport.IsUsageError(err) {
		fallbackExitCode = exitcode.UsageError
		fallbackErrorCode = "usage_error"
	}
	if rendered, ok := err.(interface{ AlreadyRendered() bool }); ok && rendered.AlreadyRendered() {
		return errortransport.CodeForError(err, fallbackExitCode)
	}

	if appForCommand(root).JSONEnabled() {
		var carrier resultCarrier
		if errors.As(err, &carrier) {
			exitCode := errortransport.CodeForError(err, fallbackExitCode)
			res := carrier.CommandResult()
			if res.Command == "" {
				res.Command = root.Name()
			}
			if res.Error == nil {
				res.Error = &result.ErrorInfo{
					Code:     errortransport.ErrorCodeForError(err, fallbackErrorCode),
					Kind:     errortransport.ErrorKindForError(err),
					ExitCode: exitCode,
					Message:  err.Error(),
				}
			}
			_ = result.Render(root.OutOrStdout(), res, true)
			return exitCode
		}
	}

	errorResult, exitCode := errortransport.ErrorResult(root.Name(), err, fallbackExitCode, fallbackErrorCode)
	if appForCommand(root).JSONEnabled() {
		_ = result.Render(root.OutOrStdout(), errorResult, true)
		return exitCode
	}

	var suppressor textErrorSuppressor
	if errors.As(err, &suppressor) && suppressor.SuppressTextError() {
		return exitCode
	}

	if _, writeErr := fmt.Fprintf(root.ErrOrStderr(), "ERROR: %v\n", err); writeErr != nil {
		return exitcode.InternalError
	}
	return exitCode
}

type resultCarrier interface {
	CommandResult() result.Result
}

type textErrorSuppressor interface {
	SuppressTextError() bool
}
