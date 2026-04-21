package backup

import (
	"errors"
	"strings"

	"github.com/lazuale/espocrm-ops/internal/contract/apperr"
	domainfailure "github.com/lazuale/espocrm-ops/internal/domain/failure"
)

func wrapBackupBoundaryError(err error) error {
	return wrapBackupAppError(err, "backup_failed")
}

func normalizeBackupFailure(err error, defaultCode string) error {
	if err == nil {
		return nil
	}

	var failure domainfailure.Failure
	if errors.As(err, &failure) && failure.Kind != "" {
		failure.Code = normalizeBackupErrorCode(failure.Code, defaultCode)
		return failure
	}

	return err
}

func wrapBackupAppError(err error, defaultCode string) error {
	err = normalizeBackupFailure(err, defaultCode)

	var failure domainfailure.Failure
	if errors.As(err, &failure) && failure.Kind != "" {
		return apperr.Wrap(apperr.Kind(failure.Kind), normalizeBackupErrorCode(failure.Code, defaultCode), err)
	}
	if kind, ok := apperr.KindOf(err); ok {
		code := defaultCode
		if existing, ok := apperr.CodeOf(err); ok {
			code = normalizeBackupErrorCode(existing, defaultCode)
		}
		return apperr.Wrap(kind, code, err)
	}
	if code, ok := apperr.CodeOf(err); ok {
		return apperr.Wrap(backupErrorKindForCode(code), normalizeBackupErrorCode(code, defaultCode), err)
	}

	return apperr.Wrap(apperr.KindInternal, defaultCode, err)
}

func normalizeBackupErrorCode(code, defaultCode string) string {
	switch strings.TrimSpace(code) {
	case "", "operation_execute_failed":
		return defaultCode
	default:
		return code
	}
}

func backupErrorKindForCode(code string) apperr.Kind {
	switch normalizeBackupErrorCode(code, "") {
	case "manifest_invalid":
		return apperr.KindManifest
	case "backup_failed", "backup_verification_failed":
		return apperr.KindValidation
	default:
		return apperr.KindInternal
	}
}
