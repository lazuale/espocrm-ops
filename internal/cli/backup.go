package cli

import (
	"fmt"
	"strings"
	"time"

	"github.com/lazuale/espocrm-ops/internal/contract/exitcode"
	"github.com/lazuale/espocrm-ops/internal/contract/result"
	"github.com/lazuale/espocrm-ops/internal/usecase/backup"
	"github.com/spf13/cobra"
)

func newBackupCmd() *cobra.Command {
	var scope string
	var createdAtRaw string
	var dbBackupPath string
	var filesBackupPath string
	var manifestPath string
	var dbChecksumPath string
	var filesChecksumPath string

	cmd := &cobra.Command{
		Use:   "backup",
		Short: "Write backup metadata for existing artifacts",
		Args:  noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			app := appForCommand(cmd)
			in := backupInput{
				scope:             scope,
				createdAtRaw:      createdAtRaw,
				dbBackupPath:      dbBackupPath,
				filesBackupPath:   filesBackupPath,
				manifestPath:      manifestPath,
				dbChecksumPath:    dbChecksumPath,
				filesChecksumPath: filesChecksumPath,
			}
			if err := validateBackupInput(cmd, &in); err != nil {
				return err
			}

			createdAt := app.runtime.Now()
			if in.createdAtRaw != "" {
				parsedAt, err := time.Parse(time.RFC3339, in.createdAtRaw)
				if err != nil {
					return usageError(fmt.Errorf("--created-at must be RFC3339: %w", err))
				}
				createdAt = parsedAt
			}

			return RunCommand(cmd, CommandSpec{
				Name:      "backup",
				ErrorCode: "backup_failed",
				ExitCode:  exitcode.InternalError,
			}, func() (result.Result, error) {
				res := result.Result{
					Artifacts: result.BackupArtifacts{
						Manifest:      in.manifestPath,
						DBBackup:      in.dbBackupPath,
						FilesBackup:   in.filesBackupPath,
						DBChecksum:    in.resolvedDBChecksumPath(),
						FilesChecksum: in.resolvedFilesChecksumPath(),
					},
				}

				info, err := backup.FinalizeBackup(backup.FinalizeRequest{
					Scope:            in.scope,
					CreatedAt:        createdAt,
					DBBackupPath:     in.dbBackupPath,
					FilesBackupPath:  in.filesBackupPath,
					ManifestPath:     in.manifestPath,
					DBSidecarPath:    in.dbChecksumPath,
					FilesSidecarPath: in.filesChecksumPath,
				})
				if err != nil {
					return res, err
				}

				res.Message = "backup metadata written"
				res.Details = result.BackupDetails{
					Scope:     info.Scope,
					CreatedAt: info.CreatedAt,
					Sidecars:  true,
				}
				res.Artifacts = result.BackupArtifacts{
					Manifest:      info.ManifestPath,
					DBBackup:      info.DBBackupPath,
					FilesBackup:   info.FilesBackupPath,
					DBChecksum:    info.DBSidecarPath,
					FilesChecksum: info.FilesSidecarPath,
				}

				return res, nil
			})
		},
	}

	cmd.Flags().StringVar(&scope, "scope", "", "backup scope")
	cmd.Flags().StringVar(&createdAtRaw, "created-at", "", "manifest timestamp in RFC3339 format (defaults to command time)")
	cmd.Flags().StringVar(&dbBackupPath, "db-backup", "", "path to db backup artifact")
	cmd.Flags().StringVar(&filesBackupPath, "files-backup", "", "path to files backup artifact")
	cmd.Flags().StringVar(&manifestPath, "manifest", "", "path to write backup manifest.json")
	cmd.Flags().StringVar(&dbChecksumPath, "db-checksum", "", "path to write db sha256 sidecar (defaults to --db-backup + .sha256)")
	cmd.Flags().StringVar(&filesChecksumPath, "files-checksum", "", "path to write files sha256 sidecar (defaults to --files-backup + .sha256)")

	return cmd
}

type backupInput struct {
	scope             string
	createdAtRaw      string
	dbBackupPath      string
	filesBackupPath   string
	manifestPath      string
	dbChecksumPath    string
	filesChecksumPath string
}

func validateBackupInput(cmd *cobra.Command, in *backupInput) error {
	in.scope = strings.TrimSpace(in.scope)
	in.dbBackupPath = strings.TrimSpace(in.dbBackupPath)
	in.filesBackupPath = strings.TrimSpace(in.filesBackupPath)
	in.manifestPath = strings.TrimSpace(in.manifestPath)
	in.dbChecksumPath = strings.TrimSpace(in.dbChecksumPath)
	in.filesChecksumPath = strings.TrimSpace(in.filesChecksumPath)

	if err := requireNonBlankFlag("--scope", in.scope); err != nil {
		return err
	}
	if err := normalizeOptionalStringFlag(cmd, "created-at", &in.createdAtRaw); err != nil {
		return err
	}
	if in.createdAtRaw != "" {
		if _, err := time.Parse(time.RFC3339, in.createdAtRaw); err != nil {
			return usageError(fmt.Errorf("--created-at must be RFC3339: %w", err))
		}
	}
	if err := requireNonBlankFlag("--db-backup", in.dbBackupPath); err != nil {
		return err
	}
	if err := requireNonBlankFlag("--files-backup", in.filesBackupPath); err != nil {
		return err
	}
	if err := requireNonBlankFlag("--manifest", in.manifestPath); err != nil {
		return err
	}
	if err := normalizeOptionalStringFlag(cmd, "db-checksum", &in.dbChecksumPath); err != nil {
		return err
	}
	if err := normalizeOptionalStringFlag(cmd, "files-checksum", &in.filesChecksumPath); err != nil {
		return err
	}

	return nil
}

func (in backupInput) resolvedDBChecksumPath() string {
	if in.dbChecksumPath != "" {
		return in.dbChecksumPath
	}
	return in.dbBackupPath + ".sha256"
}

func (in backupInput) resolvedFilesChecksumPath() string {
	if in.filesChecksumPath != "" {
		return in.filesChecksumPath
	}
	return in.filesBackupPath + ".sha256"
}
