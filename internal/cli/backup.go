package cli

import (
	"time"

	"github.com/lazuale/espocrm-ops/internal/config"
	"github.com/lazuale/espocrm-ops/internal/ops"
	"github.com/lazuale/espocrm-ops/internal/runtime"
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
		Short: "Create verified backup set",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(config.Request{Scope: scope, ProjectDir: projectDir})
			if err != nil {
				return commandFailure("backup", ops.ErrorKindUsage, exitUsage, "backup failed", err, nil)
			}

			result, backupErr := ops.Backup(cmd.Context(), cfg, runtime.DockerCompose{}, backupNow())
			if backupErr != nil {
				return opsCommandError("backup", "backup failed", backupResultFromOps(result), backupErr)
			}

			return writeJSON(cmd.OutOrStdout(), envelope{
				Command: "backup",
				OK:      true,
				Message: "backup completed",
				Error:   nil,
				Result:  backupResultFromOps(result),
			})
		},
	}

	cmd.Flags().StringVar(&scope, "scope", "", "backup scope")
	cmd.Flags().StringVar(&projectDir, "project-dir", ".", "project directory containing compose.yaml and .env.<scope>")
	return cmd
}

func backupResultFromOps(result ops.BackupResult) backupResult {
	return backupResult{
		Manifest:    result.Manifest,
		DBBackup:    result.DBBackup,
		FilesBackup: result.FilesBackup,
	}
}

func commandFailure(command, kind string, exitCode int, message string, err error, result any) error {
	return &commandError{
		command:  command,
		kind:     kind,
		exitCode: exitCode,
		message:  message,
		err:      err,
		result:   result,
	}
}

func opsCommandError(command, message string, result any, err error) error {
	verifyErr, ok := err.(*ops.VerifyError)
	if !ok {
		return commandFailure(command, ops.ErrorKindIO, exitIO, message, err, result)
	}
	return commandFailure(command, verifyErr.Kind, exitCodeForKind(verifyErr.Kind), message, verifyErr, result)
}

func exitCodeForKind(kind string) int {
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
