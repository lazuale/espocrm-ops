package cli

import (
	"fmt"
	"io"
	"strings"

	"github.com/lazuale/espocrm-ops/internal/contract/exitcode"
	"github.com/lazuale/espocrm-ops/internal/contract/result"
	backupusecase "github.com/lazuale/espocrm-ops/internal/usecase/backup"
	updateusecase "github.com/lazuale/espocrm-ops/internal/usecase/update"
	"github.com/spf13/cobra"
)

func newUpdateBackupCmd() *cobra.Command {
	var scope string
	var projectDir string
	var composeFile string
	var envFile string
	var timeoutSeconds int

	cmd := &cobra.Command{
		Use:    "update-backup",
		Short:  "Prepare the pre-update recovery point",
		Args:   noArgs,
		Hidden: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			in := updateBackupInput{
				scope:          scope,
				projectDir:     projectDir,
				composeFile:    composeFile,
				envFile:        envFile,
				timeoutSeconds: timeoutSeconds,
			}
			if err := validateUpdateBackupInput(&in); err != nil {
				return err
			}

			cfg, err := loadBackupExecutionConfig(in.projectDir)
			if err != nil {
				return err
			}
			if err := validateBackupExecutionConfig(cfg, true); err != nil {
				return err
			}

			jsonEnabled := appForCommand(cmd).JSONEnabled()

			return RunResultCommand(cmd, CommandSpec{
				Name:       "update-backup",
				ErrorCode:  "update_backup_failed",
				ExitCode:   exitcode.InternalError,
				RenderText: renderUpdateBackupText,
			}, func() (result.Result, error) {
				res := result.Result{
					OK: true,
					Artifacts: result.UpdateBackupArtifacts{
						Scope: in.scope,
					},
					Details: result.UpdateBackupDetails{
						TimeoutSeconds: in.timeoutSeconds,
					},
				}

				req := updateusecase.BackupApplyRequest{
					TimeoutSeconds: in.timeoutSeconds,
					Backup: backupusecase.ExecuteRequest{
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
					},
				}

				if !jsonEnabled {
					if _, err := fmt.Fprintln(cmd.OutOrStdout(), "[3/6] Creating the pre-update recovery point"); err != nil {
						return res, err
					}
					req.LogWriter = cmd.OutOrStdout()
					req.Backup.LogWriter = cmd.OutOrStdout()
					req.Backup.ErrWriter = cmd.ErrOrStderr()
				}

				info, err := updateusecase.ApplyBackup(req)
				if err != nil {
					return res, err
				}

				res.Message = "update backup completed"
				res.Details = result.UpdateBackupDetails{
					TimeoutSeconds:         info.TimeoutSeconds,
					StartedDBTemporarily:   info.StartedDBTemporarily,
					CreatedAt:              info.CreatedAt,
					ConsistentSnapshot:     info.ConsistentSnapshot,
					AppServicesWereRunning: info.AppServicesWereRunning,
				}
				res.Artifacts = result.UpdateBackupArtifacts{
					Scope:         in.scope,
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

	cmd.Flags().StringVar(&scope, "scope", "", "update contour")
	cmd.Flags().StringVar(&projectDir, "project-dir", "", "docker compose project directory")
	cmd.Flags().StringVar(&composeFile, "compose-file", "", "docker compose file path")
	cmd.Flags().StringVar(&envFile, "env-file", "", "docker compose env file path")
	cmd.Flags().IntVar(&timeoutSeconds, "timeout", 300, "shared db-readiness timeout in seconds")

	return cmd
}

type updateBackupInput struct {
	scope          string
	projectDir     string
	composeFile    string
	envFile        string
	timeoutSeconds int
}

func validateUpdateBackupInput(in *updateBackupInput) error {
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
	if in.timeoutSeconds < 0 {
		return usageError(fmt.Errorf("--timeout must be non-negative"))
	}

	return nil
}

func renderUpdateBackupText(w io.Writer, res result.Result) error {
	_ = w
	_ = res
	return nil
}
