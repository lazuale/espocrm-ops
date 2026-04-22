package backup

import (
	"encoding/hex"
	"fmt"
	"path/filepath"
	"strings"
	"time"
)

type Manifest struct {
	Version            int               `json:"version"`
	Scope              string            `json:"scope"`
	CreatedAt          string            `json:"created_at"`
	Artifacts          ManifestArtifacts `json:"artifacts"`
	Checksums          ManifestChecksums `json:"checksums"`
	DBBackupCreated    bool              `json:"db_backup_created"`
	FilesBackupCreated bool              `json:"files_backup_created"`
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
		DBBackupCreated:    true,
		FilesBackupCreated: true,
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
	m = m.Normalized()

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

	if !m.DBBackupCreated && !m.FilesBackupCreated {
		return fmt.Errorf("at least one backup artifact must be created")
	}

	if err := validateManifestArtifact("db", m.DBBackupCreated, m.Artifacts.DBBackup, m.Checksums.DBBackup); err != nil {
		return err
	}
	if err := validateManifestArtifact("files", m.FilesBackupCreated, m.Artifacts.FilesBackup, m.Checksums.FilesBackup); err != nil {
		return err
	}

	return nil
}

func (m Manifest) Normalized() Manifest {
	if m.DBBackupCreated || m.FilesBackupCreated {
		return m
	}

	m.DBBackupCreated = manifestArtifactPresent(m.Artifacts.DBBackup, m.Checksums.DBBackup)
	m.FilesBackupCreated = manifestArtifactPresent(m.Artifacts.FilesBackup, m.Checksums.FilesBackup)
	return m
}

func validateManifestArtifact(kind string, created bool, name, checksum string) error {
	artifactField := "artifacts." + kind + "_backup"
	checksumField := "checksums." + kind + "_backup"

	if !created {
		if strings.TrimSpace(name) != "" {
			return fmt.Errorf("%s must be empty when %s_created is false", artifactField, kind+"_backup")
		}
		if strings.TrimSpace(checksum) != "" {
			return fmt.Errorf("%s must be empty when %s_created is false", checksumField, kind+"_backup")
		}
		return nil
	}

	if err := validateArtifactName(artifactField, name); err != nil {
		return err
	}
	if err := ValidateChecksum(checksumField, checksum); err != nil {
		return err
	}

	return nil
}

func manifestArtifactPresent(name, checksum string) bool {
	return strings.TrimSpace(name) != "" || strings.TrimSpace(checksum) != ""
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
