package restore

import (
	"errors"
	"testing"

	"github.com/lazuale/espocrm-ops/internal/contract/apperr"
	domainfailure "github.com/lazuale/espocrm-ops/internal/domain/failure"
)

func TestRestoreFailurePreservesTypedFailures(t *testing.T) {
	err := restoreFailure(domainfailure.KindRestore, "restore_files_failed", domainfailure.Failure{
		Kind: domainfailure.KindValidation,
		Code: "manifest_invalid",
		Err:  errors.New("bad manifest"),
	})

	var failure domainfailure.Failure
	if !errors.As(err, &failure) {
		t.Fatalf("expected domain failure, got %T", err)
	}
	if failure.Kind != domainfailure.KindValidation {
		t.Fatalf("expected validation kind, got %q", failure.Kind)
	}
	if failure.Code != "manifest_invalid" {
		t.Fatalf("expected manifest_invalid code, got %q", failure.Code)
	}
}

func TestWrapRestoreExecuteErrorUsesDomainFailure(t *testing.T) {
	err := wrapRestoreExecuteError(domainfailure.Failure{
		Kind: domainfailure.KindConflict,
		Code: "lock_acquire_failed",
		Err:  errors.New("lock busy"),
	})

	kind, ok := apperr.KindOf(err)
	if !ok || kind != apperr.KindConflict {
		t.Fatalf("expected conflict apperr kind, got %v ok=%v", kind, ok)
	}
	code, ok := apperr.CodeOf(err)
	if !ok || code != "restore_failed" {
		t.Fatalf("expected restore_failed code, got %q ok=%v", code, ok)
	}
}
