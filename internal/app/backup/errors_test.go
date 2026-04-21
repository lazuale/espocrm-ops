package backup

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

func TestWrapBackupBoundaryErrorUsesDomainFailureCode(t *testing.T) {
	err := wrapBackupBoundaryError(domainfailure.Failure{
		Kind: domainfailure.KindManifest,
		Code: "manifest_invalid",
		Err:  errors.New("bad manifest"),
	})

	kind, ok := apperr.KindOf(err)
	if !ok || kind != apperr.KindManifest {
		t.Fatalf("expected manifest apperr kind, got %v ok=%v", kind, ok)
	}
	code, ok := apperr.CodeOf(err)
	if !ok || code != "manifest_invalid" {
		t.Fatalf("expected manifest_invalid code, got %q ok=%v", code, ok)
	}
}

func TestWrapBackupBoundaryErrorIgnoresCodeOnlyLocalCarrier(t *testing.T) {
	err := wrapBackupBoundaryError(codeOnlyError{})

	kind, ok := apperr.KindOf(err)
	if !ok || kind != apperr.KindInternal {
		t.Fatalf("expected internal apperr kind, got %v ok=%v", kind, ok)
	}
	code, ok := apperr.CodeOf(err)
	if !ok || code != "backup_failed" {
		t.Fatalf("expected backup_failed code, got %q ok=%v", code, ok)
	}
}
