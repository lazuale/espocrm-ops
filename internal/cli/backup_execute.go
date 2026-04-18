package cli

import (
	"fmt"
	"io"
	"strings"

	"github.com/lazuale/espocrm-ops/internal/contract/exitcode"
	"github.com/lazuale/espocrm-ops/internal/contract/result"
	backupusecase "github.com/lazuale/espocrm-ops/internal/usecase/backup"
	"github.com/spf13/cobra"
)

func newBackupExecuteCmd() *cobra.Command {
	var scope string
	var projectDir string
	var composeFile string
	var envFile string
	var skipDB bool
	var skipFiles bool
	var noStop bool

	cmd := &cobra.Command{
		Use:    "backup-exec",
		Short:  "Execute the backup runtime flow",
		Args:   noArgs,
		Hidden: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			in := backupExecuteInput{
				scope:       scope,
				projectDir:  projectDir,
				composeFile: composeFile,
				envFile:     envFile,
				skipDB:      skipDB,
				skipFiles:   skipFiles,
				noStop:      noStop,
			}
			if err := validateBackupExecuteInput(&in); err != nil {
				return err
			}

			cfg, err := loadBackupExecutionConfig(in.projectDir)
			if err != nil {
				return err
			}
			if err := validateBackupExecutionConfig(cfg, !in.skipDB); err != nil {
				return err
			}

			jsonEnabled := appForCommand(cmd).JSONEnabled()

			return RunResultCommand(cmd, CommandSpec{
				Name:       "backup-exec",
				ErrorCode:  "backup_failed",
				ExitCode:   exitcode.InternalError,
				RenderText: renderBackupExecuteText,
			}, func() (result.Result, error) {
				res := result.Result{
					OK:        true,
					Artifacts: result.BackupExecuteArtifacts{},
					Details: result.BackupExecuteDetails{
						Scope:              in.scope,
						SkipDB:             in.skipDB,
						SkipFiles:          in.skipFiles,
						NoStop:             in.noStop,
						ConsistentSnapshot: !in.noStop,
						RetentionDays:      cfg.RetentionDays,
					},
				}

				req := backupusecase.ExecuteRequest{
					Scope:          in.scope,
					ProjectDir:     in.projectDir,
					ComposeFile:    in.composeFile,
					EnvFile:        in.envFile,
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
				res.Details = result.BackupExecuteDetails{
					Scope:                  info.Scope,
					CreatedAt:              info.CreatedAt,
					SkipDB:                 in.skipDB,
					SkipFiles:              in.skipFiles,
					NoStop:                 in.noStop,
					ConsistentSnapshot:     info.ConsistentSnapshot,
					AppServicesWereRunning: info.AppServicesWereRunning,
					RetentionDays:          cfg.RetentionDays,
				}
				res.Artifacts = result.BackupExecuteArtifacts{
					ManifestTXT:   info.ManifestTXTPath,
					ManifestJSON:  info.ManifestJSONPath,
					DBBackup:      info.DBBackupPath,
					FilesBackup:   info.FilesBackupPath,
					DBChecksum:    info.DBSidecarPath,
					FilesChecksum: info.FilesSidecarPath,
				}

				return res, nil
			})
		},
	}

	cmd.Flags().StringVar(&scope, "scope", "", "backup contour")
	cmd.Flags().StringVar(&projectDir, "project-dir", "", "docker compose project directory")
	cmd.Flags().StringVar(&composeFile, "compose-file", "", "docker compose file path")
	cmd.Flags().StringVar(&envFile, "env-file", "", "docker compose env file path")
	cmd.Flags().BoolVar(&skipDB, "skip-db", false, "skip database backup")
	cmd.Flags().BoolVar(&skipFiles, "skip-files", false, "skip files backup")
	cmd.Flags().BoolVar(&noStop, "no-stop", false, "do not stop application services first")

	return cmd
}

type backupExecuteInput struct {
	scope       string
	projectDir  string
	composeFile string
	envFile     string
	skipDB      bool
	skipFiles   bool
	noStop      bool
}

func validateBackupExecuteInput(in *backupExecuteInput) error {
	in.scope = strings.TrimSpace(in.scope)
	in.projectDir = strings.TrimSpace(in.projectDir)
	in.composeFile = strings.TrimSpace(in.composeFile)
	in.envFile = strings.TrimSpace(in.envFile)

	switch in.scope {
	case "dev", "prod":
	default:
		return usageError(fmt.Errorf("--scope must be dev or prod"))
	}
	if err := requireNonBlankFlag("--project-dir", in.projectDir); err != nil {
		return err
	}
	if err := requireNonBlankFlag("--compose-file", in.composeFile); err != nil {
		return err
	}
	if err := requireNonBlankFlag("--env-file", in.envFile); err != nil {
		return err
	}
	if in.skipDB && in.skipFiles {
		return usageError(fmt.Errorf("nothing to back up: --skip-db and --skip-files cannot both be set"))
	}

	return nil
}

func renderBackupExecuteText(w io.Writer, res result.Result) error {
	_ = w
	_ = res
	return nil
}
