package model

import (
	"archive/tar"
	"fmt"
	"path/filepath"
	"strings"
	"time"
)

const (
	ManifestInvalidCode      = "manifest_invalid"
	legacyTarRegularTypeflag = byte(0)
)

type BackupVerifyManifest struct {
	Version            int               `json:"version"`
	Scope              string            `json:"scope"`
	CreatedAt          string            `json:"created_at"`
	Artifacts          ManifestArtifacts `json:"artifacts"`
	Checksums          ManifestChecksums `json:"checksums"`
	DBBackupCreated    bool              `json:"db_backup_created"`
	FilesBackupCreated bool              `json:"files_backup_created"`
}

func (m BackupVerifyManifest) Normalized() BackupVerifyManifest {
	if m.DBBackupCreated || m.FilesBackupCreated {
		return m
	}
	m.DBBackupCreated = manifestArtifactPresent(m.Artifacts.DBBackup, m.Checksums.DBBackup)
	m.FilesBackupCreated = manifestArtifactPresent(m.Artifacts.FilesBackup, m.Checksums.FilesBackup)
	return m
}

func (m BackupVerifyManifest) ValidateComplete() error {
	m = m.Normalized()
	if m.Version != 1 {
		return fmt.Errorf("неподдерживаемая версия manifest: %d", m.Version)
	}
	if strings.TrimSpace(m.Scope) == "" {
		return fmt.Errorf("scope обязателен")
	}
	if strings.TrimSpace(m.CreatedAt) == "" {
		return fmt.Errorf("created_at обязателен")
	}
	if _, err := time.Parse(time.RFC3339, m.CreatedAt); err != nil {
		return fmt.Errorf("created_at не является RFC3339: %w", err)
	}
	if !m.DBBackupCreated || !m.FilesBackupCreated {
		return fmt.Errorf("manifest допустим только для полного backup-set")
	}
	if err := validateManifestName("artifacts.db_backup", m.Artifacts.DBBackup); err != nil {
		return err
	}
	if err := validateManifestName("artifacts.files_backup", m.Artifacts.FilesBackup); err != nil {
		return err
	}
	if err := ValidateChecksum("checksums.db_backup", m.Checksums.DBBackup); err != nil {
		return err
	}
	if err := ValidateChecksum("checksums.files_backup", m.Checksums.FilesBackup); err != nil {
		return err
	}
	return nil
}

func (m BackupVerifyManifest) DBArtifactPath(manifestPath string) string {
	return ResolveManifestArtifactPath(manifestPath, "db", m.Artifacts.DBBackup)
}

func (m BackupVerifyManifest) FilesArtifactPath(manifestPath string) string {
	return ResolveManifestArtifactPath(manifestPath, "files", m.Artifacts.FilesBackup)
}

func ValidateBackupVerifyManifestCoherence(manifestPath string, manifest BackupVerifyManifest) error {
	dbGroup, dbOK := ParseBackupGroupName(manifest.Artifacts.DBBackup)
	filesGroup, filesOK := ParseBackupGroupName(manifest.Artifacts.FilesBackup)
	if dbOK && filesOK && dbGroup != filesGroup {
		return fmt.Errorf("manifest ссылается на artifacts из разных backup-set")
	}

	manifestGroup, manifestOK := ParseBackupGroupName(manifestPath)
	if !manifestOK {
		return nil
	}
	if !dbOK {
		return fmt.Errorf("db artifact не соответствует canonical имени backup")
	}
	if !filesOK {
		return fmt.Errorf("files artifact не соответствует canonical имени backup")
	}
	if dbGroup != manifestGroup || filesGroup != manifestGroup {
		return fmt.Errorf("manifest и artifacts относятся к разным backup-set")
	}
	return nil
}

func ResolveManifestArtifactPath(manifestPath, artifactDir, artifactName string) string {
	root := filepath.Dir(filepath.Dir(manifestPath))
	return filepath.Join(root, artifactDir, filepath.Base(artifactName))
}

func ValidateFilesArchiveHeader(header *tar.Header) error {
	if header == nil {
		return fmt.Errorf("tar header обязателен")
	}
	name := strings.TrimSpace(header.Name)
	if name == "" {
		return fmt.Errorf("tar entry без имени")
	}
	clean := filepath.ToSlash(filepath.Clean(name))
	if clean == "." || strings.HasPrefix(clean, "../") || clean == ".." || filepath.IsAbs(name) {
		return fmt.Errorf("tar entry выходит за пределы archive root: %s", name)
	}
	switch header.Typeflag {
	case tar.TypeDir, tar.TypeReg, legacyTarRegularTypeflag:
		return nil
	default:
		return fmt.Errorf("tar entry type не разрешён для backup files archive: %d", header.Typeflag)
	}
}

type BackupVerifyManifestError struct {
	Err error
}

func (e BackupVerifyManifestError) Error() string {
	return e.Err.Error()
}

func (e BackupVerifyManifestError) Unwrap() error {
	return e.Err
}

type BackupVerifyArtifactError struct {
	Label string
	Err   error
}

func (e BackupVerifyArtifactError) Error() string {
	if strings.TrimSpace(e.Label) == "" {
		return e.Err.Error()
	}
	return e.Label + ": " + e.Err.Error()
}

func (e BackupVerifyArtifactError) Unwrap() error {
	return e.Err
}

func NewArtifactFromVerification(path, checksum string, sizeBytes int64) Artifact {
	return Artifact{
		Path:         path,
		Name:         filepath.Base(path),
		ChecksumPath: checksumPath(path),
		Checksum:     strings.ToLower(strings.TrimSpace(checksum)),
		SizeBytes:    sizeBytes,
	}
}

func manifestArtifactPresent(name, checksum string) bool {
	return strings.TrimSpace(name) != "" || strings.TrimSpace(checksum) != ""
}

func checksumPath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	return path + ".sha256"
}
