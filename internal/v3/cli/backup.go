package cli

import (
	"fmt"
	"time"

	v3config "github.com/lazuale/espocrm-ops/internal/v3/config"
	"github.com/lazuale/espocrm-ops/internal/v3/ops"
	v3runtime "github.com/lazuale/espocrm-ops/internal/v3/runtime"
	"github.com/spf13/cobra"
)

var backupNow = func() time.Time {
	return time.Now().UTC()
}

type backupResult struct {
	Manifest    string `json:"manifest,omitempty"`
	Scope       string `json:"scope,omitempty"`
	CreatedAt   string `json:"created_at,omitempty"`
	DBBackup    string `json:"db_backup,omitempty"`
	FilesBackup string `json:"files_backup,omitempty"`
}

func newBackupCmd() *cobra.Command {
	var scope string
	var projectDir string
	var composeFile string
	var envFile string

	cmd := &cobra.Command{
		Use:   "backup",
		Short: "Create full verified backup set",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadBackupConfig(scope, projectDir, composeFile, envFile)
			if err != nil {
				return &commandError{
					command:  "backup",
					kind:     ops.ErrorKindUsage,
					exitCode: exitUsage,
					message:  "backup failed",
					err:      err,
				}
			}

			result, backupErr := ops.Backup(cmd.Context(), cfg, v3runtime.DockerCompose{}, backupNow())
			if backupErr != nil {
				return backupCommandError(result, backupErr)
			}

			return writeJSON(cmd.OutOrStdout(), envelope{
				Command:  "backup",
				OK:       true,
				Message:  "backup completed",
				Error:    nil,
				Warnings: []string{},
				Result: backupResult{
					Manifest:    result.Manifest,
					Scope:       result.Scope,
					CreatedAt:   result.CreatedAt,
					DBBackup:    result.DBBackup,
					FilesBackup: result.FilesBackup,
				},
			})
		},
	}

	cmd.Flags().StringVar(&scope, "scope", "", "backup contour")
	cmd.Flags().StringVar(&projectDir, "project-dir", ".", "project directory containing the compose file and env files")
	cmd.Flags().StringVar(&composeFile, "compose-file", "", "compose file path (defaults to project-dir/compose.yaml)")
	cmd.Flags().StringVar(&envFile, "env-file", "", "override env file path")
	return cmd
}

func loadBackupConfig(scope, projectDir, composeFile, envFile string) (v3config.BackupConfig, error) {
	if scope == "" {
		return v3config.BackupConfig{}, fmt.Errorf("--scope is required")
	}
	return v3config.LoadBackup(v3config.BackupRequest{
		Scope:       scope,
		ProjectDir:  projectDir,
		ComposeFile: composeFile,
		EnvFile:     envFile,
	})
}

func backupCommandError(result ops.BackupResult, err error) error {
	verifyErr, ok := err.(*ops.VerifyError)
	if !ok {
		return &commandError{
			command:  "backup",
			kind:     ops.ErrorKindIO,
			exitCode: exitIO,
			message:  "backup failed",
			err:      err,
			result: backupResult{
				Manifest:    result.Manifest,
				Scope:       result.Scope,
				CreatedAt:   result.CreatedAt,
				DBBackup:    result.DBBackup,
				FilesBackup: result.FilesBackup,
			},
		}
	}

	return &commandError{
		command:  "backup",
		kind:     verifyErr.Kind,
		exitCode: backupExitCode(verifyErr.Kind),
		message:  "backup failed",
		err:      verifyErr,
		result: backupResult{
			Manifest:    result.Manifest,
			Scope:       result.Scope,
			CreatedAt:   result.CreatedAt,
			DBBackup:    result.DBBackup,
			FilesBackup: result.FilesBackup,
		},
	}
}

func backupExitCode(kind string) int {
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
