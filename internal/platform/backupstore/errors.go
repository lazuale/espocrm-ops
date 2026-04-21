package backupstore

import (
	"fmt"
	"path/filepath"
)

type VerificationError struct {
	Err error
}

func (e VerificationError) Error() string {
	return e.Err.Error()
}

func (e VerificationError) Unwrap() error {
	return e.Err
}

type fileNameSuffixError struct {
	Label  string
	Path   string
	Suffix string
}

func (e fileNameSuffixError) Error() string {
	return fmt.Sprintf("%s name invalid: expected suffix %q for %s", e.Label, e.Suffix, filepath.Base(e.Path))
}

type checksumExpectedFormatError struct {
	Label string
	Err   error
}

func (e checksumExpectedFormatError) Error() string {
	return fmt.Sprintf("%s checksum verification failed: %v", e.Label, e.Err)
}

func (e checksumExpectedFormatError) Unwrap() error {
	return e.Err
}

type checksumReadError struct {
	Label string
	Path  string
	Err   error
}

func (e checksumReadError) Error() string {
	if e.Label != "" {
		return fmt.Sprintf("%s checksum verification failed: %v", e.Label, e.Err)
	}

	return fmt.Sprintf("checksum verification failed for %s: %v", e.Path, e.Err)
}

func (e checksumReadError) Unwrap() error {
	return e.Err
}

type checksumMismatchError struct {
	Label string
	Path  string
}

func (e checksumMismatchError) Error() string {
	return fmt.Sprintf("%s checksum verification failed: sha256 mismatch", e.Label)
}

type checksumSidecarFormatError struct {
	Label string
	Path  string
	Err   error
}

func (e checksumSidecarFormatError) Error() string {
	if e.Label != "" {
		return fmt.Sprintf("%s checksum verification failed: invalid checksum sidecar %s: %v", e.Label, e.Path, e.Err)
	}

	return fmt.Sprintf("invalid checksum sidecar %s: %v", e.Path, e.Err)
}

func (e checksumSidecarFormatError) Unwrap() error {
	return e.Err
}

type archiveValidationError struct {
	Label  string
	Path   string
	Format string
	Err    error
}

func (e archiveValidationError) Error() string {
	return fmt.Sprintf("%s %s validation failed: %v", e.Label, e.Format, e.Err)
}

func (e archiveValidationError) Unwrap() error {
	return e.Err
}

type manifestCoherenceError struct {
	Details string
}

func (e manifestCoherenceError) Error() string {
	return fmt.Sprintf("manifest backup set is inconsistent: %s", e.Details)
}
