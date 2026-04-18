package cli

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/lazuale/espocrm-ops/internal/contract/apperr"
	"github.com/lazuale/espocrm-ops/internal/contract/exitcode"
	"github.com/lazuale/espocrm-ops/internal/contract/result"
	"github.com/spf13/cobra"
)

type CodeError struct {
	Code     int
	Err      error
	ErrCode  string
	Warnings []string
}

func (e CodeError) Error() string {
	return e.Err.Error()
}

func (e CodeError) Unwrap() error {
	return e.Err
}

func (e CodeError) ExitCode() int {
	return e.Code
}

func (e CodeError) ErrorCode() string {
	if e.ErrCode != "" {
		return e.ErrCode
	}
	return "internal_error"
}

func (e CodeError) WarningMessages() []string {
	if len(e.Warnings) == 0 {
		return nil
	}
	return append([]string(nil), e.Warnings...)
}

type silentCodeError struct {
	CodeError
}

func (e silentCodeError) SuppressTextError() bool {
	return true
}

func usageError(err error) error {
	return CodeError{
		Code:    exitcode.UsageError,
		Err:     err,
		ErrCode: "usage_error",
	}
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

func normalizeOptionalStringFlag(cmd *cobra.Command, flag string, value *string) error {
	trimmed := strings.TrimSpace(*value)
	if cmd.Flags().Changed(flag) && trimmed == "" {
		return usageError(fmt.Errorf("--%s must not be blank", flag))
	}

	*value = trimmed
	return nil
}

func noArgs(cmd *cobra.Command, args []string) error {
	if err := cobra.NoArgs(cmd, args); err != nil {
		return usageError(err)
	}

	return nil
}

func codeForError(err error, fallback int) int {
	if coded, ok := err.(exitCodedError); ok {
		return coded.ExitCode()
	}

	if kind, ok := apperr.KindOf(err); ok {
		return exitCodeForKind(kind, fallback)
	}

	var pathErr *os.PathError
	if errors.As(err, &pathErr) {
		return exitcode.FilesystemError
	}

	return fallback
}

func exitCodeForKind(kind apperr.Kind, fallback int) int {
	switch kind {
	case apperr.KindManifest:
		return exitcode.ManifestError
	case apperr.KindValidation:
		return exitcode.ValidationError
	case apperr.KindIO:
		return exitcode.FilesystemError
	case apperr.KindConflict, apperr.KindExternal, apperr.KindRestore:
		return exitcode.RestoreError
	case apperr.KindInternal:
		return exitcode.InternalError
	case apperr.KindNotFound, apperr.KindCorrupted:
		return exitcode.ValidationError
	default:
		return fallback
	}
}

func errorCodeForError(err error, fallback string) string {
	if code, ok := apperr.CodeOf(err); ok {
		return code
	}

	var pathErr *os.PathError
	if errors.As(err, &pathErr) {
		return "filesystem_error"
	}

	if fallback != "" {
		return fallback
	}
	return "internal_error"
}

func errorKindForError(err error) string {
	if kind, ok := apperr.KindOf(err); ok {
		return string(kind)
	}

	return ""
}

func ErrorResult(command string, err error, fallbackExitCode int, fallbackErrorCode string) (result.Result, int) {
	exitCode := codeForError(err, fallbackExitCode)
	errCode := errorCodeForError(err, fallbackErrorCode)
	warnings := warningMessages(err)

	return result.Result{
		Command:  command,
		OK:       false,
		Warnings: warnings,
		Error: &result.ErrorInfo{
			Code:     errCode,
			Kind:     errorKindForError(err),
			ExitCode: exitCode,
			Message:  err.Error(),
		},
	}, exitCode
}

type warningCarrier interface {
	WarningMessages() []string
}

func warningMessages(err error) []string {
	var carrier warningCarrier
	if errors.As(err, &carrier) {
		return carrier.WarningMessages()
	}
	return nil
}

func IsUsageError(err error) bool {
	if err == nil {
		return false
	}

	var codedExit exitCodedError
	if errors.As(err, &codedExit) && codedExit.ExitCode() == exitcode.UsageError {
		return true
	}

	if code, ok := apperr.CodeOf(err); ok && code == "usage_error" {
		return true
	}

	return false
}
