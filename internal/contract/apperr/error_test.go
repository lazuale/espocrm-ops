package apperr

import (
	"errors"
	"fmt"
	"testing"
)

type testCodeError struct{}

func (testCodeError) Error() string {
	return "typed failure"
}

func (testCodeError) ErrorCode() string {
	return "typed_failure"
}

type testCarrierError struct{}

func (testCarrierError) Error() string {
	return "typed failure"
}

func (testCarrierError) ErrorKind() Kind {
	return KindValidation
}

func (testCarrierError) ErrorCode() string {
	return "typed_failure"
}

func TestKindOfFindsWrappedErrorKind(t *testing.T) {
	cause := errors.New("bad input")
	err := Wrap(KindValidation, "input_invalid", cause)

	kind, ok := KindOf(err)
	if !ok {
		t.Fatal("expected KindOf to find typed error kind")
	}
	if kind != KindValidation {
		t.Fatalf("expected validation kind, got %s", kind)
	}
	if !errors.Is(err, cause) {
		t.Fatal("expected wrapped cause to be preserved")
	}
	if err.ErrorCode() != "input_invalid" {
		t.Fatalf("unexpected error code: %s", err.ErrorCode())
	}
}

func TestWrapToleratesNilCause(t *testing.T) {
	err := Wrap(KindInternal, "internal_error", nil)

	if got := err.Error(); got != string(KindInternal) {
		t.Fatalf("expected kind fallback message, got %q", got)
	}
	if err.Unwrap() != nil {
		t.Fatal("expected nil cause")
	}
}

func TestCodeOfFindsWrappedErrorCode(t *testing.T) {
	err := fmt.Errorf("outer: %w", testCarrierError{})

	code, ok := CodeOf(err)
	if !ok {
		t.Fatal("expected CodeOf to find typed error code")
	}
	if code != "typed_failure" {
		t.Fatalf("expected typed_failure code, got %q", code)
	}
}

func TestCodeOfIgnoresCodeOnlyErrors(t *testing.T) {
	err := fmt.Errorf("outer: %w", testCodeError{})

	if code, ok := CodeOf(err); ok {
		t.Fatalf("expected code-only error to be ignored, got %q", code)
	}
}
