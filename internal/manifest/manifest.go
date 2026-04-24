package manifest

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type Manifest struct {
	Version   int       `json:"version"`
	Scope     string    `json:"scope"`
	CreatedAt string    `json:"created_at"`
	Artifacts Artifacts `json:"artifacts"`
	Checksums Checksums `json:"checksums"`
}

type Artifacts struct {
	DBBackup    string `json:"db_backup"`
	FilesBackup string `json:"files_backup"`
}

type Checksums struct {
	DBBackup    string `json:"db_backup"`
	FilesBackup string `json:"files_backup"`
}

type ArtifactPaths struct {
	DBPath           string
	FilesPath        string
	DBSidecarPath    string
	FilesSidecarPath string
}

func Load(path string) (Manifest, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return Manifest{}, fmt.Errorf("read manifest: %w", err)
	}

	var manifest Manifest
	if err := json.Unmarshal(raw, &manifest); err != nil {
		return Manifest{}, fmt.Errorf("decode manifest json: %w", err)
	}

	return manifest, nil
}

func Validate(manifestPath string, manifest Manifest) error {
	if manifest.Version != 1 {
		return fmt.Errorf("manifest version must be 1")
	}
	if strings.TrimSpace(manifest.Scope) == "" {
		return fmt.Errorf("manifest scope is required")
	}
	if strings.TrimSpace(manifest.CreatedAt) == "" {
		return fmt.Errorf("manifest created_at is required")
	}
	if _, err := time.Parse(time.RFC3339, manifest.CreatedAt); err != nil {
		return fmt.Errorf("manifest created_at must be RFC3339: %w", err)
	}
	if err := validateArtifactName("artifacts.db_backup", manifest.Artifacts.DBBackup); err != nil {
		return err
	}
	if err := validateArtifactName("artifacts.files_backup", manifest.Artifacts.FilesBackup); err != nil {
		return err
	}
	if err := validateChecksum("checksums.db_backup", manifest.Checksums.DBBackup); err != nil {
		return err
	}
	if err := validateChecksum("checksums.files_backup", manifest.Checksums.FilesBackup); err != nil {
		return err
	}
	if strings.TrimSpace(manifestPath) == "" {
		return fmt.Errorf("manifest path is required")
	}

	return nil
}

func ResolveArtifacts(manifestPath string, manifest Manifest) (ArtifactPaths, error) {
	manifestDir := filepath.Dir(manifestPath)
	if filepath.Base(manifestDir) != "manifests" {
		return ArtifactPaths{}, fmt.Errorf("manifest must be located in manifests directory")
	}
	root := filepath.Dir(manifestDir)

	dbPath := filepath.Join(root, "db", filepath.Base(manifest.Artifacts.DBBackup))
	filesPath := filepath.Join(root, "files", filepath.Base(manifest.Artifacts.FilesBackup))

	return ArtifactPaths{
		DBPath:           dbPath,
		FilesPath:        filesPath,
		DBSidecarPath:    dbPath + ".sha256",
		FilesSidecarPath: filesPath + ".sha256",
	}, nil
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

func validateChecksum(field, value string) error {
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
