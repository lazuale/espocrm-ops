package envadapter

import (
	"errors"
	"fmt"
	"testing"

	domainfailure "github.com/lazuale/espocrm-ops/internal/domain/failure"
	platformconfig "github.com/lazuale/espocrm-ops/internal/platform/config"
)

func TestClassifyPasswordErrorMapsWrappedTypedCauseToDomainFailure(t *testing.T) {
	err := classifyPasswordError(fmt.Errorf("resolve password: %w", platformconfig.PasswordFileReadError{
		Path: "/tmp/db-password",
		Err:  errors.New("boom"),
	}))

	assertEnvDomainFailure(t, err, domainfailure.KindIO, "filesystem_error")
}

func assertEnvDomainFailure(t *testing.T, err error, wantKind domainfailure.Kind, wantCode string) {
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
