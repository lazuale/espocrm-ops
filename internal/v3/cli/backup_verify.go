package cli

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/lazuale/espocrm-ops/internal/v3/ops"
	"github.com/spf13/cobra"
)

type backupVerifyResult struct {
	Manifest    string `json:"manifest,omitempty"`
	Scope       string `json:"scope,omitempty"`
	CreatedAt   string `json:"created_at,omitempty"`
	DBBackup    string `json:"db_backup,omitempty"`
	FilesBackup string `json:"files_backup,omitempty"`
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

			return writeJSON(cmd.OutOrStdout(), envelope{
				Command:  "backup verify",
				OK:       true,
				Message:  "backup verified",
				Error:    nil,
				Warnings: []string{},
				Result: backupVerifyResult{
					Manifest:    result.Manifest,
					Scope:       result.Scope,
					CreatedAt:   result.CreatedAt,
					DBBackup:    result.DBBackup,
					FilesBackup: result.FilesBackup,
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
		exitCode: backupVerifyExitCode(verifyErr.Kind),
		message:  "backup verify failed",
		err:      verifyErr,
		result: backupVerifyResult{
			Manifest: manifestPath,
		},
	}
}

func backupVerifyExitCode(kind string) int {
	switch kind {
	case ops.ErrorKindUsage:
		return exitUsage
	case ops.ErrorKindManifest:
		return exitManifest
	case ops.ErrorKindArtifact, ops.ErrorKindChecksum, ops.ErrorKindArchive:
		return exitValidation
	case ops.ErrorKindRuntime:
		return exitRuntime
	case ops.ErrorKindIO:
		return exitIO
	default:
		return exitInternal
	}
}
