package backup

import (
	"errors"

	"github.com/lazuale/espocrm-ops/internal/contract/apperr"
	"github.com/lazuale/espocrm-ops/internal/platform/backupstore"
)

type ManifestError struct {
	Err error
}

func (e ManifestError) Error() string {
	return e.Err.Error()
}

func (e ManifestError) Unwrap() error {
	return e.Err
}

func (e ManifestError) ErrorKind() apperr.Kind {
	return apperr.KindManifest
}

func (e ManifestError) ErrorCode() string {
	return "manifest_invalid"
}

type ValidationError struct {
	Err error
}

func (e ValidationError) Error() string {
	return e.Err.Error()
}

func (e ValidationError) Unwrap() error {
	return e.Err
}

func (e ValidationError) ErrorKind() apperr.Kind {
	return apperr.KindValidation
}

func (e ValidationError) ErrorCode() string {
	return "backup_verification_failed"
}

func classifyStoreError(err error) error {
	if err == nil {
		return nil
	}

	var manifestErr backupstore.ManifestError
	if errors.As(err, &manifestErr) {
		return ManifestError{Err: err}
	}

	var validationErr backupstore.ValidationError
	if errors.As(err, &validationErr) {
		return ValidationError{Err: err}
	}

	return err
}
