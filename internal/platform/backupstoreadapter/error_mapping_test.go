package backupstoreadapter

import (
	"errors"
	"fmt"
	"testing"

	domainfailure "github.com/lazuale/espocrm-ops/internal/domain/failure"
	platformbackupstore "github.com/lazuale/espocrm-ops/internal/platform/backupstore"
)

func TestClassifyBackupStoreErrorMapsWrappedTypedCauseToDomainFailure(t *testing.T) {
	err := classifyBackupStoreError(fmt.Errorf("load manifest: %w", platformbackupstore.ManifestError{
		Err: errors.New("bad manifest"),
	}))

	assertBackupStoreDomainFailure(t, err, domainfailure.KindManifest, "manifest_invalid")
}

func assertBackupStoreDomainFailure(t *testing.T, err error, wantKind domainfailure.Kind, wantCode string) {
	t.Helper()

	var failure domainfailure.Failure
	if !errors.As(err, &failure) {
		t.Fatalf("expected domainfailure.Failure, got %T", err)
	}
	if failure.Kind != wantKind {
		t.Fatalf("expected kind %q, got %q", wantKind, failure.Kind)
	}
	if failure.Code != wantCode {
		t.Fatalf("expected code %q, got %q", wantCode, failure.Code)
	}
}
