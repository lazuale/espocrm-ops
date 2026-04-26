package cli

import (
	"fmt"
	"path/filepath"
	"strings"

	manifestpkg "github.com/lazuale/espocrm-ops/internal/manifest"
	"github.com/lazuale/espocrm-ops/internal/ops"
	"github.com/spf13/cobra"
)

type backupVerifyRuntimeResult struct {
	EspoCRMImage     string   `json:"espo_crm_image,omitempty"`
	MariaDBImage     string   `json:"mariadb_image,omitempty"`
	DBName           string   `json:"db_name,omitempty"`
	DBService        string   `json:"db_service,omitempty"`
	AppServices      []string `json:"app_services,omitempty"`
	BackupNamePrefix string   `json:"backup_name_prefix,omitempty"`
	StorageContract  string   `json:"storage_contract,omitempty"`
}

type backupVerifyResult struct {
	Manifest        string                     `json:"manifest,omitempty"`
	ManifestVersion int                        `json:"manifest_version,omitempty"`
	Scope           string                     `json:"scope,omitempty"`
	CreatedAt       string                     `json:"created_at,omitempty"`
	DBBackup        string                     `json:"db_backup,omitempty"`
	FilesBackup     string                     `json:"files_backup,omitempty"`
	Runtime         *backupVerifyRuntimeResult `json:"runtime"`
}

func newBackupVerifyCmd() *cobra.Command {
	var manifestPath string

	cmd := &cobra.Command{
		Use:   "verify",
		Short: "Verify backup set from explicit manifest",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			path, err := normalizeManifestPath(manifestPath)
			if err != nil {
				return err
			}

			result, verifyErr := ops.VerifyBackup(cmd.Context(), path)
			if verifyErr != nil {
				return backupVerifyError(path, verifyErr)
			}

			warnings := combineWarnings(result.Warnings)
			if result.ManifestVersion == manifestpkg.VersionOne {
				warnings = append(warnings, "manifest version 1 is checksum-valid, but restore and migrate require manifest version 2 with runtime metadata")
			}

			return writeJSON(cmd.OutOrStdout(), envelope{
				Command:  "backup verify",
				OK:       true,
				Message:  "backup verified",
				Error:    nil,
				Warnings: warnings,
				Result: backupVerifyResult{
					Manifest:        result.Manifest,
					ManifestVersion: result.ManifestVersion,
					Scope:           result.Scope,
					CreatedAt:       result.CreatedAt,
					DBBackup:        result.DBBackup,
					FilesBackup:     result.FilesBackup,
					Runtime:         backupVerifyRuntimeResultPtr(result),
				},
			})
		},
	}

	cmd.Flags().StringVar(&manifestPath, "manifest", "", "path to backup manifest.json")
	return cmd
}

func normalizeManifestPath(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", backupVerifyUsageError("--manifest is required")
	}

	abs, err := filepath.Abs(filepath.Clean(value))
	if err != nil {
		return "", &commandError{
			command:  "backup verify",
			kind:     "usage",
			exitCode: exitUsage,
			message:  fmt.Sprintf("resolve --manifest: %v", err),
			err:      err,
		}
	}
	return abs, nil
}

func backupVerifyError(manifestPath string, err error) error {
	verifyErr, ok := err.(*ops.VerifyError)
	if !ok {
		return &commandError{
			command:  "backup verify",
			kind:     "io",
			exitCode: exitIO,
			message:  "backup verify failed",
			err:      err,
			result: backupVerifyResult{
				Manifest: manifestPath,
			},
		}
	}

	return &commandError{
		command:  "backup verify",
		kind:     verifyErr.Kind,
		exitCode: backupExitCode(verifyErr.Kind),
		message:  "backup verify failed",
		err:      verifyErr,
		result: backupVerifyResult{
			Manifest: manifestPath,
		},
	}
}

func backupVerifyRuntimeResultPtr(result ops.VerifyResult) *backupVerifyRuntimeResult {
	if result.Runtime.EspoCRMImage == "" &&
		result.Runtime.MariaDBImage == "" &&
		result.Runtime.DBName == "" &&
		result.Runtime.DBService == "" &&
		len(result.Runtime.AppServices) == 0 &&
		result.Runtime.BackupNamePrefix == "" &&
		result.Runtime.StorageContract == "" {
		return nil
	}

	return &backupVerifyRuntimeResult{
		EspoCRMImage:     result.Runtime.EspoCRMImage,
		MariaDBImage:     result.Runtime.MariaDBImage,
		DBName:           result.Runtime.DBName,
		DBService:        result.Runtime.DBService,
		AppServices:      append([]string(nil), result.Runtime.AppServices...),
		BackupNamePrefix: result.Runtime.BackupNamePrefix,
		StorageContract:  result.Runtime.StorageContract,
	}
}
