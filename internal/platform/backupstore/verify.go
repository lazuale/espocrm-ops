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

type artifactVerification struct {
	label          string
	path           string
	suffix         string
	expectedSHA256 string
	sidecarPath    string
	verifyReadable func(string) error
}

func VerifyManifestDetailed(manifestPath string) (VerifiedBackup, error) {
	if strings.TrimSpace(manifestPath) == "" {
		return VerifiedBackup{}, ManifestError{Err: fmt.Errorf("manifest path is required")}
	}

	manifest, err := LoadManifest(manifestPath)
	if err != nil {
		return VerifiedBackup{}, err
	}

	dbPath := domainbackup.ResolveManifestArtifactPath(manifestPath, "db", manifest.Artifacts.DBBackup)
	filesPath := domainbackup.ResolveManifestArtifactPath(manifestPath, "files", manifest.Artifacts.FilesBackup)

	if err := verifyManifestCoherence(manifestPath, manifest); err != nil {
		return VerifiedBackup{}, VerificationError{Err: err}
	}

	if err := verifyBackupArtifact(artifactVerification{
		label:          "db backup",
		path:           dbPath,
		suffix:         ".sql.gz",
		expectedSHA256: manifest.Checksums.DBBackup,
		verifyReadable: verifyGzipReadable("db backup"),
	}); err != nil {
		return VerifiedBackup{}, err
	}
	if err := verifyBackupArtifact(artifactVerification{
		label:          "files backup",
		path:           filesPath,
		suffix:         ".tar.gz",
		expectedSHA256: manifest.Checksums.FilesBackup,
		verifyReadable: verifyFilesArchiveReadable("files backup"),
	}); err != nil {
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
	dbGroup, dbOK := parsedBackupGroup(manifest.Artifacts.DBBackup, domainbackup.ParseDBBackupName)
	filesGroup, filesOK := parsedBackupGroup(manifest.Artifacts.FilesBackup, domainbackup.ParseFilesBackupName)

	if dbOK && filesOK && dbGroup != filesGroup {
		return manifestCoherenceError{
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

	manifestGroup, ok := canonicalManifestGroup(manifestPath)
	if !ok {
		return nil
	}

	if !dbOK {
		return invalidManifestArtifactNameError("database backup", manifest.Artifacts.DBBackup)
	}
	if !filesOK {
		return invalidManifestArtifactNameError("files backup", manifest.Artifacts.FilesBackup)
	}

	if dbGroup != manifestGroup {
		return manifestCoherenceError{
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
		return manifestCoherenceError{
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
	return verifyBackupArtifact(artifactVerification{
		label:          "db backup",
		path:           dbPath,
		suffix:         ".sql.gz",
		sidecarPath:    dbPath + ".sha256",
		verifyReadable: verifyGzipReadable("db backup"),
	})
}

func VerifyDirectFilesBackup(filesPath string) error {
	return verifyBackupArtifact(artifactVerification{
		label:          "files backup",
		path:           filesPath,
		suffix:         ".tar.gz",
		sidecarPath:    filesPath + ".sha256",
		verifyReadable: verifyFilesArchiveReadable("files backup"),
	})
}

func parsedBackupGroup(name string, parse func(string) (domainbackup.BackupGroup, error)) (domainbackup.BackupGroup, bool) {
	group, err := parse(name)
	if err != nil {
		return domainbackup.BackupGroup{}, false
	}

	return group, true
}

func invalidManifestArtifactNameError(label, name string) error {
	return manifestCoherenceError{
		Details: fmt.Sprintf("%s %q does not match the expected backup file naming pattern", label, name),
	}
}

func canonicalManifestGroup(manifestPath string) (domainbackup.BackupGroup, bool) {
	group, _, err := domainbackup.ParseManifestName(manifestPath)
	if err != nil {
		return domainbackup.BackupGroup{}, false
	}

	return group, true
}

func verifyBackupArtifact(spec artifactVerification) error {
	if err := verifyNonEmptyFile(spec.label, spec.path); err != nil {
		return VerificationError{Err: err}
	}
	if err := verifyFilenameSuffix(spec.label, spec.path, spec.suffix); err != nil {
		return VerificationError{Err: err}
	}
	if err := verifyArtifactChecksum(spec); err != nil {
		return VerificationError{Err: err}
	}
	if err := spec.verifyReadable(spec.path); err != nil {
		return VerificationError{Err: err}
	}

	return nil
}

func verifyArtifactChecksum(spec artifactVerification) error {
	if strings.TrimSpace(spec.expectedSHA256) != "" {
		return verifyExpectedChecksum(spec.label, spec.path, spec.expectedSHA256)
	}
	if strings.TrimSpace(spec.sidecarPath) == "" {
		return nil
	}

	if err := verifyNonEmptyFile(spec.label+" checksum", spec.sidecarPath); err != nil {
		return err
	}
	ok, err := verifySHA256Sidecar(spec.label, spec.path, spec.sidecarPath)
	if err != nil {
		return err
	}
	if !ok {
		return checksumMismatchError{Label: spec.label, Path: spec.path}
	}

	return nil
}

func verifyNonEmptyFile(label, filePath string) error {
	_, err := platformfs.EnsureNonEmptyFile(label, filePath)
	return err
}

func verifyFilenameSuffix(label, filePath, suffix string) error {
	if !strings.HasSuffix(filePath, suffix) {
		return fileNameSuffixError{Label: label, Path: filePath, Suffix: suffix}
	}

	return nil
}

func verifyExpectedChecksum(label, filePath, expected string) error {
	if err := domainbackup.ValidateChecksum("checksum", expected); err != nil {
		return checksumExpectedFormatError{Label: label, Err: err}
	}

	actual, err := platformfs.SHA256File(filePath)
	if err != nil {
		return checksumReadError{Label: label, Path: filePath, Err: err}
	}

	if !strings.EqualFold(actual, strings.TrimSpace(expected)) {
		return checksumMismatchError{Label: label, Path: filePath}
	}

	return nil
}

func verifySHA256Sidecar(label, filePath, sidecarPath string) (bool, error) {
	raw, err := os.ReadFile(sidecarPath)
	if err != nil {
		return false, checksumReadError{Label: label, Path: sidecarPath, Err: err}
	}
	fields := strings.Fields(string(raw))
	if len(fields) == 0 {
		return false, checksumSidecarFormatError{Label: label, Path: sidecarPath, Err: fmt.Errorf("sidecar is empty")}
	}
	if err := domainbackup.ValidateChecksum("checksum", fields[0]); err != nil {
		return false, checksumSidecarFormatError{Label: label, Path: sidecarPath, Err: err}
	}

	actual, err := platformfs.SHA256File(filePath)
	if err != nil {
		return false, checksumReadError{Label: label, Path: filePath, Err: err}
	}

	return strings.EqualFold(actual, fields[0]), nil
}

func verifyGzipReadable(label string) func(string) error {
	return func(filePath string) error {
		if err := platformfs.VerifyGzipReadable(filePath); err != nil {
			return archiveValidationError{Label: label, Path: filePath, Format: "gzip", Err: err}
		}

		return nil
	}
}

func verifyFilesArchiveReadable(label string) func(string) error {
	return func(filePath string) error {
		if err := platformfs.VerifyTarGzReadable(filePath, domainbackup.ValidateFilesArchiveHeader); err != nil {
			return archiveValidationError{Label: label, Path: filePath, Format: "tar.gz", Err: err}
		}

		return nil
	}
}
