package backupstore

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	domainbackup "github.com/lazuale/espocrm-ops/internal/domain/backup"
	platformfs "github.com/lazuale/espocrm-ops/internal/platform/fs"
)

type VerifiedBackup struct {
	ManifestPath string
	Scope        string
	CreatedAt    string
	DBBackupPath string
	FilesPath    string
}

func VerifyManifest(manifestPath string) error {
	_, err := VerifyManifestDetailed(manifestPath)
	return err
}

func VerifyManifestDetailed(manifestPath string) (VerifiedBackup, error) {
	if strings.TrimSpace(manifestPath) == "" {
		return VerifiedBackup{}, ManifestError{Err: fmt.Errorf("manifest path is required")}
	}

	manifest, err := LoadManifest(manifestPath)
	if err != nil {
		return VerifiedBackup{}, err
	}

	dbPath := domainbackup.ResolveArtifactPath(manifestPath, "db", manifest.Artifacts.DBBackup)
	filesPath := domainbackup.ResolveArtifactPath(manifestPath, "files", manifest.Artifacts.FilesBackup)

	if err := verifyManifestCoherence(manifestPath, manifest); err != nil {
		return VerifiedBackup{}, ValidationError{Err: err}
	}

	if err := verifyManifestArtifact("db backup", dbPath, ".sql.gz", manifest.Checksums.DBBackup, verifyGzipReadable("db backup")); err != nil {
		return VerifiedBackup{}, err
	}
	if err := verifyManifestArtifact("files backup", filesPath, ".tar.gz", manifest.Checksums.FilesBackup, verifyFilesArchiveReadable("files backup")); err != nil {
		return VerifiedBackup{}, err
	}

	return VerifiedBackup{
		ManifestPath: manifestPath,
		Scope:        manifest.Scope,
		CreatedAt:    manifest.CreatedAt,
		DBBackupPath: dbPath,
		FilesPath:    filesPath,
	}, nil
}

func verifyManifestCoherence(manifestPath string, manifest domainbackup.Manifest) error {
	dbGroup, dbErr := domainbackup.ParseDBBackupName(manifest.Artifacts.DBBackup)
	filesGroup, filesErr := domainbackup.ParseFilesBackupName(manifest.Artifacts.FilesBackup)

	if dbErr == nil && filesErr == nil && dbGroup != filesGroup {
		return ManifestCoherenceError{
			Details: fmt.Sprintf(
				"database backup %q resolves to %s_%s, but files backup %q resolves to %s_%s",
				manifest.Artifacts.DBBackup,
				dbGroup.Prefix,
				dbGroup.Stamp,
				manifest.Artifacts.FilesBackup,
				filesGroup.Prefix,
				filesGroup.Stamp,
			),
		}
	}

	manifestGroup, _, manifestErr := domainbackup.ParseManifestName(manifestPath)
	if manifestErr != nil {
		return nil
	}

	if dbErr != nil {
		return ManifestCoherenceError{
			Details: fmt.Sprintf("database backup %q does not match the canonical manifest-backed backup naming contract", manifest.Artifacts.DBBackup),
		}
	}
	if filesErr != nil {
		return ManifestCoherenceError{
			Details: fmt.Sprintf("files backup %q does not match the canonical manifest-backed backup naming contract", manifest.Artifacts.FilesBackup),
		}
	}
	if dbGroup != manifestGroup {
		return ManifestCoherenceError{
			Details: fmt.Sprintf(
				"manifest %q resolves to %s_%s, but database backup %q resolves to %s_%s",
				filepath.Base(manifestPath),
				manifestGroup.Prefix,
				manifestGroup.Stamp,
				manifest.Artifacts.DBBackup,
				dbGroup.Prefix,
				dbGroup.Stamp,
			),
		}
	}
	if filesGroup != manifestGroup {
		return ManifestCoherenceError{
			Details: fmt.Sprintf(
				"manifest %q resolves to %s_%s, but files backup %q resolves to %s_%s",
				filepath.Base(manifestPath),
				manifestGroup.Prefix,
				manifestGroup.Stamp,
				manifest.Artifacts.FilesBackup,
				filesGroup.Prefix,
				filesGroup.Stamp,
			),
		}
	}

	return nil
}

func VerifyDirectDBBackup(dbPath string) error {
	return verifyDirectArtifact("db backup", dbPath, ".sql.gz", verifyGzipReadable("db backup"))
}

func VerifyDirectFilesBackup(filesPath string) error {
	return verifyDirectArtifact("files backup", filesPath, ".tar.gz", verifyFilesArchiveReadable("files backup"))
}

func verifyManifestArtifact(label, filePath, suffix, expectedChecksum string, verifyReadable func(string) error) error {
	if err := verifyNonEmptyFile(label, filePath); err != nil {
		return ValidationError{Err: err}
	}
	if err := verifyFilenameSuffix(label, filePath, suffix); err != nil {
		return ValidationError{Err: err}
	}
	if err := verifyExpectedChecksum(label, filePath, expectedChecksum); err != nil {
		return ValidationError{Err: err}
	}
	if err := verifyReadable(filePath); err != nil {
		return ValidationError{Err: err}
	}

	return nil
}

func verifyDirectArtifact(label, filePath, suffix string, verifyReadable func(string) error) error {
	if err := verifyNonEmptyFile(label, filePath); err != nil {
		return ValidationError{Err: err}
	}
	if err := verifyFilenameSuffix(label, filePath, suffix); err != nil {
		return ValidationError{Err: err}
	}
	sidecarPath := filePath + ".sha256"
	if err := verifyNonEmptyFile(label+" checksum", sidecarPath); err != nil {
		return ValidationError{Err: err}
	}
	ok, err := verifySHA256Sidecar(label, filePath, sidecarPath)
	if err != nil {
		return ValidationError{Err: err}
	}
	if !ok {
		return ValidationError{Err: ChecksumMismatchError{Label: label, Path: filePath}}
	}
	if err := verifyReadable(filePath); err != nil {
		return ValidationError{Err: err}
	}

	return nil
}

func verifyNonEmptyFile(label, filePath string) error {
	_, err := platformfs.EnsureNonEmptyFile(label, filePath)
	return err
}

func verifyFilenameSuffix(label, filePath, suffix string) error {
	if !strings.HasSuffix(filePath, suffix) {
		return FileNameSuffixError{Label: label, Path: filePath, Suffix: suffix}
	}

	return nil
}

func verifyExpectedChecksum(label, filePath, expected string) error {
	if err := domainbackup.ValidateChecksum("checksum", expected); err != nil {
		return ChecksumExpectedFormatError{Label: label, Err: err}
	}

	actual, err := platformfs.SHA256File(filePath)
	if err != nil {
		return ChecksumReadError{Label: label, Path: filePath, Err: err}
	}

	if !strings.EqualFold(actual, strings.TrimSpace(expected)) {
		return ChecksumMismatchError{Label: label, Path: filePath}
	}

	return nil
}

func VerifySHA256Sidecar(filePath, sidecarPath string) (bool, error) {
	return verifySHA256Sidecar("", filePath, sidecarPath)
}

func verifySHA256Sidecar(label, filePath, sidecarPath string) (bool, error) {
	raw, err := os.ReadFile(sidecarPath)
	if err != nil {
		return false, ChecksumReadError{Label: label, Path: sidecarPath, Err: err}
	}
	fields := strings.Fields(string(raw))
	if len(fields) == 0 {
		return false, ChecksumSidecarFormatError{Label: label, Path: sidecarPath, Err: fmt.Errorf("sidecar is empty")}
	}
	if err := domainbackup.ValidateChecksum("checksum", fields[0]); err != nil {
		return false, ChecksumSidecarFormatError{Label: label, Path: sidecarPath, Err: err}
	}

	actual, err := platformfs.SHA256File(filePath)
	if err != nil {
		return false, ChecksumReadError{Label: label, Path: filePath, Err: err}
	}

	return strings.EqualFold(actual, fields[0]), nil
}

func verifyGzipReadable(label string) func(string) error {
	return func(filePath string) error {
		if err := platformfs.VerifyGzipReadable(filePath); err != nil {
			return ArchiveValidationError{Label: label, Path: filePath, Format: "gzip", Err: err}
		}

		return nil
	}
}

func verifyFilesArchiveReadable(label string) func(string) error {
	return func(filePath string) error {
		if err := platformfs.VerifyTarGzReadable(filePath, domainbackup.ValidateFilesArchiveHeader); err != nil {
			return ArchiveValidationError{Label: label, Path: filePath, Format: "tar.gz", Err: err}
		}

		return nil
	}
}
