package cli

import (
	"errors"
	"testing"

	"github.com/lazuale/espocrm-ops/internal/contract/apperr"
	"github.com/lazuale/espocrm-ops/internal/contract/exitcode"
)

func TestCodeForError_MapsAppErrorKindsToExitCodes(t *testing.T) {
	tests := []struct {
		name string
		kind apperr.Kind
		want int
	}{
		{name: "manifest", kind: apperr.KindManifest, want: exitcode.ManifestError},
		{name: "validation", kind: apperr.KindValidation, want: exitcode.ValidationError},
		{name: "io", kind: apperr.KindIO, want: exitcode.FilesystemError},
		{name: "conflict", kind: apperr.KindConflict, want: exitcode.RestoreError},
		{name: "external", kind: apperr.KindExternal, want: exitcode.RestoreError},
		{name: "restore", kind: apperr.KindRestore, want: exitcode.RestoreError},
		{name: "internal", kind: apperr.KindInternal, want: exitcode.InternalError},
		{name: "not-found", kind: apperr.KindNotFound, want: exitcode.ValidationError},
		{name: "corrupted", kind: apperr.KindCorrupted, want: exitcode.ValidationError},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := apperr.Wrap(tc.kind, tc.name+"_failed", errors.New("boom"))
			got := codeForError(err, 99)
			if got != tc.want {
				t.Fatalf("expected exit code %d, got %d", tc.want, got)
			}
		})
	}
}

func TestCodeForError_UnknownAppErrorKindUsesFallback(t *testing.T) {
	err := apperr.Error{
		Kind:    apperr.Kind("future_kind"),
		Code:    "future_error",
		Message: "future failure",
	}

	got := codeForError(err, 99)
	if got != 99 {
		t.Fatalf("expected fallback exit code, got %d", got)
	}
}

func TestErrorCodeForError_UsesAppErrorMachineCode(t *testing.T) {
	err := apperr.Wrap(apperr.KindValidation, "input_invalid", errors.New("bad input"))

	got := errorCodeForError(err, "fallback_error")
	if got != "input_invalid" {
		t.Fatalf("expected app error code, got %q", got)
	}
}

func TestErrorResult_IncludesTypedMachineFields(t *testing.T) {
	err := apperr.Wrap(apperr.KindNotFound, "operation_not_found", errors.New("missing operation"))

	res, exitCode := ErrorResult("show-operation", err, exitcode.InternalError, "internal_error")

	if exitCode != exitcode.ValidationError {
		t.Fatalf("expected validation exit code, got %d", exitCode)
	}
	if res.Error == nil {
		t.Fatal("expected error info")
	}
	if res.Error.Code != "operation_not_found" {
		t.Fatalf("unexpected error code: %s", res.Error.Code)
	}
	if res.Error.Kind != string(apperr.KindNotFound) {
		t.Fatalf("unexpected error kind: %s", res.Error.Kind)
	}
	if res.Error.ExitCode != exitcode.ValidationError {
		t.Fatalf("unexpected json exit code: %d", res.Error.ExitCode)
	}
}

func TestErrorResult_PropagatesWarningsFromCodeError(t *testing.T) {
	err := CodeError{
		Code:     exitcode.RestoreError,
		Err:      apperr.Wrap(apperr.KindRestore, "restore_failed", errors.New("boom")),
		ErrCode:  "restore_failed",
		Warnings: []string{"failed to write journal entry: journal writer is not configured"},
	}

	res, exitCode := ErrorResult("restore-db", err, exitcode.InternalError, "internal_error")

	if exitCode != exitcode.RestoreError {
		t.Fatalf("expected restore exit code, got %d", exitCode)
	}
	if len(res.Warnings) != 1 {
		t.Fatalf("expected one warning, got %#v", res.Warnings)
	}
	if res.Warnings[0] != "failed to write journal entry: journal writer is not configured" {
		t.Fatalf("unexpected warning: %#v", res.Warnings)
	}
}

func TestIsUsageError_UsesTypedErrorInsteadOfMessageGuessing(t *testing.T) {
	if !IsUsageError(usageError(errors.New("boom"))) {
		t.Fatal("expected typed usage error to be recognized")
	}

	if IsUsageError(errors.New(`unknown command "oops" for "espops"`)) {
		t.Fatal("plain message-only error must not be treated as usage error")
	}
}
