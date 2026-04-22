package errortransport

import (
	"errors"
	"os"
	"strings"

	"github.com/lazuale/espocrm-ops/internal/contract/apperr"
	"github.com/lazuale/espocrm-ops/internal/contract/exitcode"
	"github.com/lazuale/espocrm-ops/internal/contract/result"
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

type ResultCodeError struct {
	CodeError
	Result result.Result
}

func (e ResultCodeError) CommandResult() result.Result {
	return e.Result
}

func UsageError(err error) error {
	return CodeError{
		Code:    exitcode.UsageError,
		Err:     err,
		ErrCode: "usage_error",
	}
}

func CodeForError(err error, fallback int) int {
	type exitCodedError interface {
		error
		ExitCode() int
	}

	var coded exitCodedError
	if errors.As(err, &coded) {
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

func ErrorCodeForError(err error, fallback string) string {
	var failureCarrier interface{ FailureCode() string }
	if errors.As(err, &failureCarrier) {
		if code := strings.TrimSpace(failureCarrier.FailureCode()); code != "" {
			return code
		}
	}

	var carrier interface{ ErrorCode() string }
	if errors.As(err, &carrier) {
		if code := strings.TrimSpace(carrier.ErrorCode()); code != "" {
			return code
		}
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

func ErrorKindForError(err error) string {
	var carrier interface{ ErrorKind() string }
	if errors.As(err, &carrier) {
		if kind := strings.TrimSpace(carrier.ErrorKind()); kind != "" {
			return kind
		}
	}

	if kind, ok := apperr.KindOf(err); ok {
		return string(kind)
	}

	return ""
}

func ErrorResult(command string, err error, fallbackExitCode int, fallbackErrorCode string) (result.Result, int) {
	exitCode := CodeForError(err, fallbackExitCode)
	errCode := ErrorCodeForError(err, fallbackErrorCode)
	warnings := WarningMessages(err)

	return result.Result{
		Command:  command,
		OK:       false,
		Warnings: warnings,
		Error: &result.ErrorInfo{
			Code:     errCode,
			Kind:     ErrorKindForError(err),
			ExitCode: exitCode,
			Message:  err.Error(),
		},
	}, exitCode
}

func WarningMessages(err error) []string {
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

	type exitCodedError interface {
		error
		ExitCode() int
	}

	var codedExit exitCodedError
	if errors.As(err, &codedExit) && codedExit.ExitCode() == exitcode.UsageError {
		return true
	}

	var carrier interface{ ErrorCode() string }
	if errors.As(err, &carrier) && strings.TrimSpace(carrier.ErrorCode()) == "usage_error" {
		return true
	}

	return false
}

func Silent(codeErr CodeError) error {
	return silentCodeError{CodeError: codeErr}
}

type warningCarrier interface {
	WarningMessages() []string
}

type silentCodeError struct {
	CodeError
}

func (e silentCodeError) SuppressTextError() bool {
	return true
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
