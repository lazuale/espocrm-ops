package appadapter

import (
	"errors"
	"fmt"
	"testing"

	domainfailure "github.com/lazuale/espocrm-ops/internal/domain/failure"
	platformbackupstore "github.com/lazuale/espocrm-ops/internal/platform/backupstore"
	platformconfig "github.com/lazuale/espocrm-ops/internal/platform/config"
	platformdocker "github.com/lazuale/espocrm-ops/internal/platform/docker"
)

func TestClassifyPasswordErrorMapsWrappedTypedCauseToDomainFailure(t *testing.T) {
	err := classifyPasswordError(fmt.Errorf("resolve password: %w", platformconfig.PasswordFileReadError{
		Path: "/tmp/db-password",
		Err:  errors.New("boom"),
	}))

	assertDomainFailure(t, err, domainfailure.KindIO, "filesystem_error")
}

func TestClassifyBackupStoreErrorMapsWrappedTypedCauseToDomainFailure(t *testing.T) {
	err := classifyBackupStoreError(fmt.Errorf("load manifest: %w", platformbackupstore.ManifestError{
		Err: errors.New("bad manifest"),
	}))

	assertDomainFailure(t, err, domainfailure.KindManifest, "manifest_invalid")
}

func TestClassifyRuntimeErrorMapsWrappedTypedCausesToDomainFailures(t *testing.T) {
	tests := []struct {
		name string
		err  error
		code string
	}{
		{
			name: "docker unavailable",
			err: fmt.Errorf("docker call failed: %w", platformdocker.UnavailableError{
				Err: errors.New("missing docker"),
			}),
			code: "docker_unavailable",
		},
		{
			name: "container inspect",
			err: fmt.Errorf("inspect failed: %w", platformdocker.ContainerInspectError{
				Container: "db",
				Err:       errors.New("boom"),
			}),
			code: "container_inspect_failed",
		},
		{
			name: "db execution",
			err: fmt.Errorf("restore failed: %w", platformdocker.DBExecutionError{
				Action:    "restore mysql dump",
				Container: "db",
				Err:       errors.New("boom"),
			}),
			code: "restore_db_failed",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assertDomainFailure(t, classifyRuntimeError(tc.err), domainfailure.KindExternal, tc.code)
		})
	}
}

func assertDomainFailure(t *testing.T, err error, wantKind domainfailure.Kind, wantCode string) {
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
