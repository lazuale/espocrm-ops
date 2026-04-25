package manifest

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

const (
	VersionOne     = 1
	VersionCurrent = 2

	StorageContractEspoCRMFullStorageV1 = "espocrm-full-storage-v1"
)

type Manifest struct {
	Version   int       `json:"version"`
	Scope     string    `json:"scope"`
	CreatedAt string    `json:"created_at"`
	Artifacts Artifacts `json:"artifacts"`
	Checksums Checksums `json:"checksums"`
	Runtime   Runtime   `json:"runtime,omitempty"`
}

type Artifacts struct {
	DBBackup    string `json:"db_backup"`
	FilesBackup string `json:"files_backup"`
}

type Checksums struct {
	DBBackup    string `json:"db_backup"`
	FilesBackup string `json:"files_backup"`
}

type Runtime struct {
	EspoCRMImage     string   `json:"espo_crm_image"`
	MariaDBImage     string   `json:"mariadb_image"`
	DBName           string   `json:"db_name"`
	DBService        string   `json:"db_service"`
	AppServices      []string `json:"app_services"`
	BackupNamePrefix string   `json:"backup_name_prefix"`
	StorageContract  string   `json:"storage_contract"`
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

func Validate(path string, manifest Manifest) error {
	if strings.TrimSpace(path) == "" {
		return fmt.Errorf("manifest path is required")
	}
	switch manifest.Version {
	case VersionOne, VersionCurrent:
	default:
		return fmt.Errorf("manifest version must be %d or %d", VersionOne, VersionCurrent)
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
	if manifest.Version == VersionCurrent {
		if err := validateRuntime(manifest.Runtime); err != nil {
			return err
		}
	}

	return nil
}

func RequireRestoreRuntimeContract(manifest Manifest) error {
	if manifest.Version != VersionCurrent {
		return fmt.Errorf(
			"manifest version %d is unsupported for restore/migrate; manifest version %d with runtime metadata is required",
			manifest.Version,
			VersionCurrent,
		)
	}
	if err := validateRuntime(manifest.Runtime); err != nil {
		return err
	}

	return nil
}

func ResolveArtifacts(path string, manifest Manifest) (ArtifactPaths, error) {
	manifestDir := filepath.Dir(path)
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

func validateRuntime(runtime Runtime) error {
	for _, field := range []struct {
		name  string
		value string
	}{
		{name: "runtime.espo_crm_image", value: runtime.EspoCRMImage},
		{name: "runtime.mariadb_image", value: runtime.MariaDBImage},
		{name: "runtime.db_name", value: runtime.DBName},
		{name: "runtime.db_service", value: runtime.DBService},
		{name: "runtime.backup_name_prefix", value: runtime.BackupNamePrefix},
		{name: "runtime.storage_contract", value: runtime.StorageContract},
	} {
		if strings.TrimSpace(field.value) == "" {
			return fmt.Errorf("%s is required", field.name)
		}
	}
	if runtime.StorageContract != StorageContractEspoCRMFullStorageV1 {
		return fmt.Errorf(
			"runtime.storage_contract must be %q",
			StorageContractEspoCRMFullStorageV1,
		)
	}
	if len(runtime.AppServices) == 0 {
		return fmt.Errorf("runtime.app_services is required")
	}
	if slices.ContainsFunc(runtime.AppServices, func(service string) bool {
		return strings.TrimSpace(service) == ""
	}) {
		return fmt.Errorf("runtime.app_services contains empty service name")
	}

	return nil
}
