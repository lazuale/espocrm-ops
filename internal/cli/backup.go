package cli

import (
	"errors"
	"fmt"

	"github.com/lazuale/espocrm-ops/internal/contract/apperr"
	"github.com/lazuale/espocrm-ops/internal/contract/exitcode"
	"github.com/lazuale/espocrm-ops/internal/contract/result"
	"github.com/lazuale/espocrm-ops/internal/opsconfig"
	backupusecase "github.com/lazuale/espocrm-ops/internal/usecase/backup"
	maintenanceusecase "github.com/lazuale/espocrm-ops/internal/usecase/maintenance"
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
	jsonEnabled := appForCommand(cmd).JSONEnabled()

	return RunCommand(cmd, CommandSpec{
		Name:      "backup",
		ErrorCode: "backup_failed",
		ExitCode:  exitcode.InternalError,
	}, func() (res result.Result, err error) {
		res = result.Result{
			OK: true,
			Details: result.BackupDetails{
				Scope:     in.scope,
				SkipDB:    in.skipDB,
				SkipFiles: in.skipFiles,
				NoStop:    in.noStop,
			},
		}

		ctx, err := maintenanceusecase.PrepareOperation(maintenanceusecase.OperationContextRequest{
			Scope:           in.scope,
			Operation:       "backup",
			ProjectDir:      in.projectDir,
			EnvFileOverride: in.envFile,
			EnvContourHint:  envFileContourHint(),
		})
		if err != nil {
			return res, wrapBackupCommandError(err)
		}
		defer func() {
			if releaseErr := ctx.Release(); releaseErr != nil {
				if err == nil {
					err = releaseErr
					return
				}
				err = errors.Join(err, releaseErr)
			}
		}()

		cfg, err := opsconfig.LoadBackupExecutionConfig(in.projectDir, ctx.Env.FilePath, ctx.Env.Values, !in.skipDB)
		if err != nil {
			return res, apperr.Wrap(apperr.KindValidation, "backup_failed", err)
		}

		req := backupusecase.ExecuteRequest{
			Scope:          in.scope,
			ProjectDir:     in.projectDir,
			ComposeFile:    in.composeFile,
			EnvFile:        ctx.Env.FilePath,
			BackupRoot:     cfg.BackupRoot,
			StorageDir:     cfg.StorageDir,
			NamePrefix:     cfg.NamePrefix,
			RetentionDays:  cfg.RetentionDays,
			ComposeProject: cfg.ComposeProject,
			DBUser:         cfg.DBUser,
			DBPassword:     cfg.DBPassword,
			DBName:         cfg.DBName,
			EspoCRMImage:   cfg.EspoCRMImage,
			MariaDBTag:     cfg.MariaDBTag,
			SkipDB:         in.skipDB,
			SkipFiles:      in.skipFiles,
			NoStop:         in.noStop,
		}
		if !jsonEnabled {
			req.LogWriter = cmd.OutOrStdout()
			req.ErrWriter = cmd.ErrOrStderr()
		}

		info, err := backupusecase.ExecuteBackup(req)
		if err != nil {
			return res, err
		}

		res.Message = "backup completed"
		res.Details = result.BackupDetails{
			Scope:                  info.Scope,
			CreatedAt:              info.CreatedAt,
			SkipDB:                 in.skipDB,
			SkipFiles:              in.skipFiles,
			NoStop:                 in.noStop,
			ConsistentSnapshot:     info.ConsistentSnapshot,
			AppServicesWereRunning: info.AppServicesWereRunning,
			RetentionDays:          cfg.RetentionDays,
		}
		res.Artifacts = result.BackupArtifacts{
			ManifestTXT:   info.ManifestTXTPath,
			ManifestJSON:  info.ManifestJSONPath,
			DBBackup:      info.DBBackupPath,
			FilesBackup:   info.FilesBackupPath,
			DBChecksum:    info.DBSidecarPath,
			FilesChecksum: info.FilesSidecarPath,
		}

		return res, nil
	})
}

func wrapBackupCommandError(err error) error {
	if kind, ok := apperr.KindOf(err); ok {
		return apperr.Wrap(kind, "backup_failed", err)
	}

	return apperr.Wrap(apperr.KindInternal, "backup_failed", err)
}
