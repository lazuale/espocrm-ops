package backupstore

import (
	"fmt"
	"path/filepath"
)

type ValidationError struct {
	Err error
}

func (e ValidationError) Error() string {
	return e.Err.Error()
}

func (e ValidationError) Unwrap() error {
	return e.Err
}

func (e ValidationError) ErrorCode() string {
	return "backup_verification_failed"
}

type FileNameSuffixError struct {
	Label  string
	Path   string
	Suffix string
}

func (e FileNameSuffixError) Error() string {
	return fmt.Sprintf("%s name invalid: expected suffix %q for %s", e.Label, e.Suffix, filepath.Base(e.Path))
}

func (e FileNameSuffixError) ErrorCode() string {
	return "backup_verification_failed"
}

type ChecksumExpectedFormatError struct {
	Label string
	Err   error
}

func (e ChecksumExpectedFormatError) Error() string {
	return fmt.Sprintf("%s checksum verification failed: %v", e.Label, e.Err)
}

func (e ChecksumExpectedFormatError) Unwrap() error {
	return e.Err
}

func (e ChecksumExpectedFormatError) ErrorCode() string {
	return "backup_verification_failed"
}

type ChecksumReadError struct {
	Label string
	Path  string
	Err   error
}

func (e ChecksumReadError) Error() string {
	if e.Label != "" {
		return fmt.Sprintf("%s checksum verification failed: %v", e.Label, e.Err)
	}

	return fmt.Sprintf("checksum verification failed for %s: %v", e.Path, e.Err)
}

func (e ChecksumReadError) Unwrap() error {
	return e.Err
}

func (e ChecksumReadError) ErrorCode() string {
	return "backup_verification_failed"
}

type ChecksumMismatchError struct {
	Label string
	Path  string
}

func (e ChecksumMismatchError) Error() string {
	return fmt.Sprintf("%s checksum verification failed: sha256 mismatch", e.Label)
}

func (e ChecksumMismatchError) ErrorCode() string {
	return "backup_verification_failed"
}

type ChecksumSidecarFormatError struct {
	Label string
	Path  string
	Err   error
}

func (e ChecksumSidecarFormatError) Error() string {
	if e.Label != "" {
		return fmt.Sprintf("%s checksum verification failed: invalid checksum sidecar %s: %v", e.Label, e.Path, e.Err)
	}

	return fmt.Sprintf("invalid checksum sidecar %s: %v", e.Path, e.Err)
}

func (e ChecksumSidecarFormatError) Unwrap() error {
	return e.Err
}

func (e ChecksumSidecarFormatError) ErrorCode() string {
	return "backup_verification_failed"
}

type ArchiveValidationError struct {
	Label  string
	Path   string
	Format string
	Err    error
}

func (e ArchiveValidationError) Error() string {
	return fmt.Sprintf("%s %s validation failed: %v", e.Label, e.Format, e.Err)
}

func (e ArchiveValidationError) Unwrap() error {
	return e.Err
}

func (e ArchiveValidationError) ErrorCode() string {
	return "backup_verification_failed"
}

type ManifestCoherenceError struct {
	Details string
}

func (e ManifestCoherenceError) Error() string {
	return fmt.Sprintf("manifest backup set is inconsistent: %s", e.Details)
}

func (e ManifestCoherenceError) ErrorCode() string {
	return "backup_verification_failed"
}
