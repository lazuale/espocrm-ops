package restore

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type RestoreDBRequest struct {
	ManifestPath       string
	DBBackup           string
	DBContainer        string
	DBName             string
	DBUser             string
	DBPassword         string
	DBPasswordFile     string
	DBRootPassword     string
	DBRootPasswordFile string
	DryRun             bool
}

func (r RestoreDBRequest) Validate() error {
	if err := validateRestoreDBSource(r.ManifestPath, r.DBBackup); err != nil {
		return err
	}
	if err := requireNonBlank("db container", r.DBContainer); err != nil {
		return err
	}
	if err := requireNonBlank("db name", r.DBName); err != nil {
		return err
	}
	if err := requireNonBlank("db user", r.DBUser); err != nil {
		return err
	}

	return nil
}

type RestoreFilesRequest struct {
	ManifestPath string
	FilesBackup  string
	TargetDir    string
	DryRun       bool
}

func (r RestoreFilesRequest) Validate() error {
	if err := validateRestoreFilesSource(r.ManifestPath, r.FilesBackup); err != nil {
		return err
	}

	return ValidateTargetDir(r.TargetDir)
}

type FilesPreflightRequest struct {
	ManifestPath string
	FilesBackup  string
	TargetDir    string
}

func (r FilesPreflightRequest) Validate() error {
	if err := validateRestoreFilesSource(r.ManifestPath, r.FilesBackup); err != nil {
		return err
	}

	return ValidateTargetDir(r.TargetDir)
}

type DBPreflightRequest struct {
	ManifestPath string
	DBBackup     string
	DBContainer  string
}

func (r DBPreflightRequest) Validate() error {
	if err := validateRestoreDBSource(r.ManifestPath, r.DBBackup); err != nil {
		return err
	}
	if err := requireNonBlank("db container", r.DBContainer); err != nil {
		return err
	}

	return nil
}

func ValidateTargetDir(targetDir string) error {
	if err := requireNonBlank("target dir", targetDir); err != nil {
		return err
	}

	cleanTarget := filepath.Clean(targetDir)
	if cleanTarget == "." {
		return fmt.Errorf("target dir must not be the current directory")
	}
	if cleanTarget == string(os.PathSeparator) {
		return fmt.Errorf("target dir must not be the filesystem root")
	}

	return nil
}

func requireNonBlank(name, value string) error {
	if strings.TrimSpace(value) == "" {
		return fmt.Errorf("%s is required", name)
	}

	return nil
}

func validateRestoreFilesSource(manifestPath, filesBackup string) error {
	hasManifest := strings.TrimSpace(manifestPath) != ""
	hasFilesBackup := strings.TrimSpace(filesBackup) != ""
	switch {
	case hasManifest && hasFilesBackup:
		return fmt.Errorf("use either manifest path or files backup, not both")
	case hasManifest || hasFilesBackup:
		return nil
	default:
		return fmt.Errorf("manifest path or files backup is required")
	}
}

func validateRestoreDBSource(manifestPath, dbBackup string) error {
	hasManifest := strings.TrimSpace(manifestPath) != ""
	hasDBBackup := strings.TrimSpace(dbBackup) != ""
	switch {
	case hasManifest && hasDBBackup:
		return fmt.Errorf("use either manifest path or db backup, not both")
	case hasManifest || hasDBBackup:
		return nil
	default:
		return fmt.Errorf("manifest path or db backup is required")
	}
}
