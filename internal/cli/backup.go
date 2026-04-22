package cli

import (
	"fmt"

	v2app "github.com/lazuale/espocrm-ops/internal/app"
	resultbridge "github.com/lazuale/espocrm-ops/internal/cli/resultbridge"
	"github.com/lazuale/espocrm-ops/internal/contract/exitcode"
	"github.com/lazuale/espocrm-ops/internal/contract/result"
	"github.com/spf13/cobra"
)

func newBackupCmd() *cobra.Command {
	var scope string
	var projectDir string
	var composeFile string
	var envFile string
	var skipDB bool
	var skipFiles bool
	var noStop bool

	cmd := &cobra.Command{
		Use:   "backup",
		Short: "Create a coherent backup set",
		Args:  noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			in := backupInput{
				scope:       scope,
				projectDir:  projectDir,
				composeFile: composeFile,
				envFile:     envFile,
				skipDB:      skipDB,
				skipFiles:   skipFiles,
				noStop:      noStop,
			}
			if err := validateBackupInput(cmd, &in); err != nil {
				return err
			}

			return runBackup(cmd, in)
		},
	}

	cmd.Flags().StringVar(&scope, "scope", "", "backup contour")
	cmd.Flags().StringVar(&projectDir, "project-dir", ".", "project directory containing the compose file and env files")
	cmd.Flags().StringVar(&composeFile, "compose-file", "", "compose file path (defaults to project-dir/compose.yaml)")
	cmd.Flags().StringVar(&envFile, "env-file", "", "override env file path")
	cmd.Flags().BoolVar(&skipDB, "skip-db", false, "skip database backup")
	cmd.Flags().BoolVar(&skipFiles, "skip-files", false, "skip files backup")
	cmd.Flags().BoolVar(&noStop, "no-stop", false, "do not stop application services before backup")

	return cmd
}

type backupInput struct {
	scope       string
	projectDir  string
	composeFile string
	envFile     string
	skipDB      bool
	skipFiles   bool
	noStop      bool
}

func validateBackupInput(cmd *cobra.Command, in *backupInput) error {
	if err := normalizeContourFlag("--scope", &in.scope); err != nil {
		return err
	}
	if err := normalizeProjectContext(cmd, &in.projectDir, &in.composeFile, &in.envFile); err != nil {
		return err
	}

	if in.skipDB && in.skipFiles {
		return usageError(fmt.Errorf("nothing to back up: --skip-db and --skip-files cannot both be set"))
	}

	return nil
}

func runBackup(cmd *cobra.Command, in backupInput) error {
	return RunCommand(cmd, CommandSpec{
		Name:       "backup",
		ErrorCode:  "backup_failed",
		ExitCode:   exitcode.InternalError,
		RenderText: resultbridge.RenderBackupText,
	}, func() (res result.Result, err error) {
		info, err := appForCommand(cmd).backup.Execute(cmd.Context(), v2app.BackupCommandRequest{
			Scope:           in.scope,
			ProjectDir:      in.projectDir,
			ComposeFile:     in.composeFile,
			EnvFileOverride: in.envFile,
			SkipDB:          in.skipDB,
			SkipFiles:       in.skipFiles,
			NoStop:          in.noStop,
			Now:             appForCommand(cmd).runtime.Now,
		})
		if err != nil {
			res = resultbridge.BackupResult(info)
			return res, err
		}

		return resultbridge.BackupResult(info), nil
	})
}
