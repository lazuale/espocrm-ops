package cli

import (
	"fmt"
	"time"

	config "github.com/lazuale/espocrm-ops/internal/config"
	"github.com/lazuale/espocrm-ops/internal/ops"
	runtime "github.com/lazuale/espocrm-ops/internal/runtime"
	"github.com/spf13/cobra"
)

var backupNow = func() time.Time {
	return time.Now().UTC()
}

type backupResult struct {
	Manifest    string `json:"manifest,omitempty"`
	DBBackup    string `json:"db_backup,omitempty"`
	FilesBackup string `json:"files_backup,omitempty"`
}

func newBackupCmd() *cobra.Command {
	var scope string
	var projectDir string

	cmd := &cobra.Command{
		Use:   "backup",
		Short: "Create full verified backup set",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadBackupConfig(scope, projectDir)
			if err != nil {
				return &commandError{
					command:  "backup",
					kind:     ops.ErrorKindUsage,
					exitCode: exitUsage,
					message:  "backup failed",
					err:      err,
				}
			}

			result, backupErr := ops.Backup(cmd.Context(), cfg, runtime.DockerCompose{}, backupNow())
			if backupErr != nil {
				return backupCommandError(result, backupErr)
			}

			return writeJSON(cmd.OutOrStdout(), envelope{
				Command:  "backup",
				OK:       true,
				Message:  "backup completed",
				Error:    nil,
				Warnings: combineWarnings(cfg.Warnings, result.Warnings),
				Result: backupResult{
					Manifest:    result.Manifest,
					DBBackup:    result.DBBackup,
					FilesBackup: result.FilesBackup,
				},
			})
		},
	}

	cmd.Flags().StringVar(&scope, "scope", "", "backup contour")
	cmd.Flags().StringVar(&projectDir, "project-dir", ".", "project directory containing the compose file and env files")
	return cmd
}

func loadBackupConfig(scope, projectDir string) (config.BackupConfig, error) {
	if scope == "" {
		return config.BackupConfig{}, fmt.Errorf("--scope is required")
	}
	return config.LoadBackup(config.BackupRequest{
		Scope:      scope,
		ProjectDir: projectDir,
	})
}

func combineWarnings(groups ...[]string) []string {
	var warnings []string
	for _, group := range groups {
		warnings = append(warnings, group...)
	}
	if len(warnings) == 0 {
		return []string{}
	}
	return append([]string(nil), warnings...)
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
