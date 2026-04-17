package backup

import (
	"encoding/hex"
	"fmt"
	"path/filepath"
	"strings"
	"time"
)

type Manifest struct {
	Version   int               `json:"version"`
	Scope     string            `json:"scope"`
	CreatedAt string            `json:"created_at"`
	Artifacts ManifestArtifacts `json:"artifacts"`
	Checksums ManifestChecksums `json:"checksums"`
}

type ManifestArtifacts struct {
	DBBackup    string `json:"db_backup"`
	FilesBackup string `json:"files_backup"`
}

type ManifestChecksums struct {
	DBBackup    string `json:"db_backup"`
	FilesBackup string `json:"files_backup"`
}

type ManifestBuildRequest struct {
	Scope           string
	CreatedAt       time.Time
	DBBackupName    string
	FilesBackupName string
	DBChecksum      string
	FilesChecksum   string
}

func ResolveArtifactPath(manifestPath, kind, fileName string) string {
	return ResolveManifestArtifactPath(manifestPath, kind, fileName)
}

func BuildManifest(req ManifestBuildRequest) (Manifest, error) {
	manifest := Manifest{
		Version:   1,
		Scope:     strings.TrimSpace(req.Scope),
		CreatedAt: req.CreatedAt.UTC().Format(time.RFC3339),
		Artifacts: ManifestArtifacts{
			DBBackup:    filepath.Base(req.DBBackupName),
			FilesBackup: filepath.Base(req.FilesBackupName),
		},
		Checksums: ManifestChecksums{
			DBBackup:    req.DBChecksum,
			FilesBackup: req.FilesChecksum,
		},
	}
	if req.CreatedAt.IsZero() {
		return Manifest{}, fmt.Errorf("created_at is required")
	}
	if err := manifest.Validate(); err != nil {
		return Manifest{}, err
	}

	return manifest, nil
}

func (m Manifest) Validate() error {
	if m.Version != 1 {
		return fmt.Errorf("unsupported manifest version: %d", m.Version)
	}
	if strings.TrimSpace(m.Scope) == "" {
		return fmt.Errorf("scope is required")
	}
	if strings.TrimSpace(m.CreatedAt) == "" {
		return fmt.Errorf("created_at is required")
	}
	if _, err := time.Parse(time.RFC3339, m.CreatedAt); err != nil {
		return fmt.Errorf("invalid created_at: %w", err)
	}

	if err := validateArtifactName("artifacts.db_backup", m.Artifacts.DBBackup); err != nil {
		return err
	}
	if err := validateArtifactName("artifacts.files_backup", m.Artifacts.FilesBackup); err != nil {
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

func validateArtifactName(field, value string) error {
	value = strings.TrimSpace(value)
	if value == "" {
		return fmt.Errorf("%s is required", field)
	}
	if filepath.Base(value) != value || value == "." || value == ".." {
		return fmt.Errorf("%s must be a file name, not a path", field)
	}

	return nil
}

func ValidateChecksum(field, value string) error {
	value = strings.TrimSpace(value)
	if value == "" {
		return fmt.Errorf("%s is required", field)
	}
	if len(value) != 64 {
		return fmt.Errorf("%s must be a 64-char sha256 hex digest", field)
	}
	if _, err := hex.DecodeString(strings.ToLower(value)); err != nil {
		return fmt.Errorf("%s must be valid hex: %w", field, err)
	}

	return nil
}
