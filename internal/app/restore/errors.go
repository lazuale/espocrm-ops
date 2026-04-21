package restore

import (
	"errors"
	"strings"

	"github.com/lazuale/espocrm-ops/internal/contract/apperr"
	domainfailure "github.com/lazuale/espocrm-ops/internal/domain/failure"
)

func restoreFailure(kind domainfailure.Kind, code string, err error) error {
	if err == nil {
		return nil
	}

	var failure domainfailure.Failure
	if errors.As(err, &failure) {
		if failure.Kind == "" {
			failure.Kind = kind
		}
		if strings.TrimSpace(failure.Code) == "" {
			failure.Code = code
		}
		return failure
	}

	return domainfailure.Failure{
		Kind: kind,
		Code: code,
		Err:  err,
	}
}

func wrapRestoreExecuteError(err error) error {
	err = normalizeRestoreFailure(err, "restore_failed")

	var failure domainfailure.Failure
	if errors.As(err, &failure) && failure.Kind != "" {
		return apperr.Wrap(apperr.Kind(failure.Kind), "restore_failed", err)
	}
	var execute executeFailure
	if errors.As(err, &execute) && execute.Kind != "" {
		return apperr.Wrap(apperr.Kind(execute.Kind), "restore_failed", err)
	}
	if kind, ok := apperr.KindOf(err); ok {
		return apperr.Wrap(kind, "restore_failed", err)
	}

	return apperr.Wrap(apperr.KindInternal, "restore_failed", err)
}

func normalizeRestoreFailure(err error, defaultCode string) error {
	if err == nil {
		return nil
	}

	var failure domainfailure.Failure
	if errors.As(err, &failure) && failure.Kind != "" {
		failure.Code = normalizeRestoreErrorCode(failure.Code, defaultCode)
		return failure
	}

	return err
}

func normalizeRestoreErrorCode(code, defaultCode string) string {
	switch strings.TrimSpace(code) {
	case "", "operation_execute_failed":
		return defaultCode
	default:
		return code
	}
}
