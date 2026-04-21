package restoreflow

import (
	"errors"
	"fmt"
	"strings"

	domainfailure "github.com/lazuale/espocrm-ops/internal/domain/failure"
)

func failure(kind domainfailure.Kind, code string, err error) error {
	if err == nil {
		return nil
	}

	var existing domainfailure.Failure
	if errors.As(err, &existing) {
		if existing.Kind == "" {
			existing.Kind = kind
		}
		if strings.TrimSpace(existing.Code) == "" {
			existing.Code = code
		}
		return existing
	}

	return domainfailure.Failure{
		Kind: kind,
		Code: code,
		Err:  err,
	}
}

func releaseLockError(kind domainfailure.Kind, code, label string, err error) error {
	if err == nil {
		return nil
	}

	return failure(kind, code, fmt.Errorf("release %s: %w", label, err))
}
