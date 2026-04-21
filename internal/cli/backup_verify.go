package cli

import (
	"fmt"
	"io"

	"github.com/lazuale/espocrm-ops/internal/contract/exitcode"
	"github.com/lazuale/espocrm-ops/internal/contract/result"
	"github.com/lazuale/espocrm-ops/internal/app/backup"
	"github.com/spf13/cobra"
)

func newBackupVerifyCmd() *cobra.Command {
	var manifestPath string
	var backupRoot string

	cmd := &cobra.Command{
		Use:   "verify",
		Short: "Verify backup set from manifest",
		Args:  noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateBackupVerifyInput(manifestPath, backupRoot); err != nil {
				return err
			}

			return RunCommand(cmd, CommandSpec{
				Name:       "backup verify",
				ErrorCode:  "backup_verification_failed",
				ExitCode:   exitcode.ValidationError,
				RenderText: renderBackupVerifyText,
			}, func() (result.Result, error) {
				res := result.Result{
					Artifacts: result.BackupVerifyArtifacts{
						Manifest: manifestPath,
					},
				}

				verifyManifestPath := manifestPath
				if backupRoot != "" {
					selectedManifestPath, err := appForCommand(cmd).backup.LatestCompleteManifest(backupRoot)
					if err != nil {
						return res, err
					}
					verifyManifestPath = selectedManifestPath
					res.Artifacts = result.BackupVerifyArtifacts{
						Manifest: selectedManifestPath,
					}
				}

				info, err := appForCommand(cmd).backup.VerifyDetailed(backup.VerifyRequest{
					ManifestPath: verifyManifestPath,
				})
				if err != nil {
					return res, err
				}

				res.Message = "backup verification passed"
				res.Artifacts = result.BackupVerifyArtifacts{
					Manifest:    info.ManifestPath,
					DBBackup:    info.DBBackupPath,
					FilesBackup: info.FilesPath,
				}
				res.Details = result.BackupVerifyDetails{
					Scope:     info.Scope,
					CreatedAt: info.CreatedAt,
				}

				return res, nil
			})
		},
	}

	cmd.Flags().StringVar(&manifestPath, "manifest", "", "path to backup manifest.json")
	cmd.Flags().StringVar(&backupRoot, "backup-root", "", "backup root containing db, files and manifests directories")

	return cmd
}

func validateBackupVerifyInput(manifestPath, backupRoot string) error {
	hasManifest := manifestPath != ""
	hasBackupRoot := backupRoot != ""
	if hasManifest && hasBackupRoot {
		return usageError(fmt.Errorf("use either --manifest or --backup-root, not both"))
	}
	if hasManifest {
		return requireNonBlankFlag("--manifest", manifestPath)
	}
	if hasBackupRoot {
		return requireNonBlankFlag("--backup-root", backupRoot)
	}

	return usageError(fmt.Errorf("--manifest or --backup-root is required"))
}

func renderBackupVerifyText(w io.Writer, res result.Result) error {
	artifacts, ok := res.Artifacts.(result.BackupVerifyArtifacts)
	if !ok {
		return result.Render(w, res, false)
	}

	if _, err := fmt.Fprintln(w, "Verifying backup set"); err != nil {
		return err
	}
	if artifacts.DBBackup != "" {
		if _, err := fmt.Fprintf(w, "Database backup: %s\n", artifacts.DBBackup); err != nil {
			return err
		}
	}
	if artifacts.FilesBackup != "" {
		if _, err := fmt.Fprintf(w, "Files backup: %s\n", artifacts.FilesBackup); err != nil {
			return err
		}
	}
	if artifacts.Manifest != "" {
		if _, err := fmt.Fprintf(w, "JSON manifest: %s\n", artifacts.Manifest); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintln(w, "[ok] Backup set: manifest, checksums, and archive readability confirmed"); err != nil {
		return err
	}
	_, err := fmt.Fprintln(w, "Backup-file verification completed successfully")
	return err
}
