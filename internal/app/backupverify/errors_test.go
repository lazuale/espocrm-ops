package backupverify

import (
	"errors"
	"testing"

	"github.com/lazuale/espocrm-ops/internal/contract/apperr"
	domainfailure "github.com/lazuale/espocrm-ops/internal/domain/failure"
)

type codeOnlyError struct{}

func (codeOnlyError) Error() string {
	return "typed failure"
}

func (codeOnlyError) ErrorCode() string {
	return "manifest_invalid"
}

func TestWrapAppErrorUsesDomainFailureCode(t *testing.T) {
	err := wrapAppError(domainfailure.Failure{
		Kind: domainfailure.KindManifest,
		Code: "manifest_invalid",
		Err:  errors.New("bad manifest"),
	}, "backup_verification_failed")

	kind, ok := apperr.KindOf(err)
	if !ok || kind != apperr.KindManifest {
		t.Fatalf("expected manifest apperr kind, got %v ok=%v", kind, ok)
	}
	code, ok := apperr.CodeOf(err)
	if !ok || code != "manifest_invalid" {
		t.Fatalf("expected manifest_invalid code, got %q ok=%v", code, ok)
	}
}

func TestWrapAppErrorIgnoresCodeOnlyLocalCarrier(t *testing.T) {
	err := wrapAppError(codeOnlyError{}, "backup_verification_failed")

	kind, ok := apperr.KindOf(err)
	if !ok || kind != apperr.KindInternal {
		t.Fatalf("expected internal apperr kind, got %v ok=%v", kind, ok)
	}
	code, ok := apperr.CodeOf(err)
	if !ok || code != "backup_verification_failed" {
		t.Fatalf("expected backup_verification_failed code, got %q ok=%v", code, ok)
	}
}
