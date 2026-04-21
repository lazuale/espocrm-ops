package backupverify

import (
	"errors"
	"strings"

	"github.com/lazuale/espocrm-ops/internal/contract/apperr"
	domainfailure "github.com/lazuale/espocrm-ops/internal/domain/failure"
)

func wrapAppError(err error, defaultCode string) error {
	err = normalizeFailure(err, defaultCode)

	var failure domainfailure.Failure
	if errors.As(err, &failure) && failure.Kind != "" {
		return apperr.Wrap(apperr.Kind(failure.Kind), normalizeErrorCode(failure.Code, defaultCode), err)
	}
	if kind, ok := apperr.KindOf(err); ok {
		code := defaultCode
		if existing, ok := apperr.CodeOf(err); ok {
			code = normalizeErrorCode(existing, defaultCode)
		}
		return apperr.Wrap(kind, code, err)
	}
	if code, ok := apperr.CodeOf(err); ok {
		return apperr.Wrap(errorKindForCode(code), normalizeErrorCode(code, defaultCode), err)
	}

	return apperr.Wrap(apperr.KindInternal, defaultCode, err)
}

func normalizeFailure(err error, defaultCode string) error {
	if err == nil {
		return nil
	}

	var failure domainfailure.Failure
	if errors.As(err, &failure) && failure.Kind != "" {
		failure.Code = normalizeErrorCode(failure.Code, defaultCode)
		return failure
	}

	return err
}

func normalizeErrorCode(code, defaultCode string) string {
	switch strings.TrimSpace(code) {
	case "", "operation_execute_failed":
		return defaultCode
	default:
		return code
	}
}

func errorKindForCode(code string) apperr.Kind {
	switch normalizeErrorCode(code, "") {
	case "manifest_invalid":
		return apperr.KindManifest
	case "backup_verification_failed":
		return apperr.KindValidation
	default:
		return apperr.KindInternal
	}
}
