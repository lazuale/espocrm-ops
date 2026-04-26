package cli

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/lazuale/espocrm-ops/internal/ops"
	"github.com/spf13/cobra"
)

type backupVerifyResult struct {
	Manifest    string `json:"manifest,omitempty"`
	Scope       string `json:"scope,omitempty"`
	CreatedAt   string `json:"created_at,omitempty"`
	DBBackup    string `json:"db_backup,omitempty"`
	FilesBackup string `json:"files_backup,omitempty"`
	DBName      string `json:"db_name,omitempty"`
}

func newBackupVerifyCmd() *cobra.Command {
	var manifestPath string

	cmd := &cobra.Command{
		Use:   "verify",
		Short: "Verify backup set from manifest",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			path, err := normalizeManifestPath(manifestPath, "backup verify")
			if err != nil {
				return err
			}

			result, verifyErr := ops.VerifyBackup(cmd.Context(), path)
			if verifyErr != nil {
				return opsCommandError("backup verify", "backup verify failed", backupVerifyResult{Manifest: path}, verifyErr)
			}

			return writeJSON(cmd.OutOrStdout(), envelope{
				Command: "backup verify",
				OK:      true,
				Message: "backup verified",
				Error:   nil,
				Result:  backupVerifyResultFromOps(result),
			})
		},
	}

	cmd.Flags().StringVar(&manifestPath, "manifest", "", "path to backup manifest.json")
	return cmd
}

func normalizeManifestPath(value, command string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", commandFailure(command, ops.ErrorKindUsage, exitUsage, command+" failed", fmt.Errorf("--manifest is required"), nil)
	}
	abs, err := filepath.Abs(filepath.Clean(value))
	if err != nil {
		return "", commandFailure(command, ops.ErrorKindUsage, exitUsage, command+" failed", fmt.Errorf("resolve --manifest: %w", err), nil)
	}
	return abs, nil
}

func backupVerifyResultFromOps(result ops.VerifyResult) backupVerifyResult {
	return backupVerifyResult{
		Manifest:    result.Manifest,
		Scope:       result.Scope,
		CreatedAt:   result.CreatedAt,
		DBBackup:    result.DBBackup,
		FilesBackup: result.FilesBackup,
		DBName:      result.DBName,
	}
}
