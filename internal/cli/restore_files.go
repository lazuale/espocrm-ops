package cli

import (
	"fmt"
	"path/filepath"

	"github.com/lazuale/espocrm-ops/internal/contract/exitcode"
	"github.com/lazuale/espocrm-ops/internal/contract/result"
	"github.com/lazuale/espocrm-ops/internal/usecase/restore"
	"github.com/spf13/cobra"
)

func newRestoreFilesCmd() *cobra.Command {
	var manifestPath string
	var filesBackup string
	var targetDir string
	var dryRun bool

	cmd := &cobra.Command{
		Use:   "restore-files",
		Short: "Restore files from a manifest-backed backup set",
		Args:  noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRestoreFilesInput(manifestPath, filesBackup, targetDir); err != nil {
				return err
			}

			return RunCommand(cmd, CommandSpec{
				Name:      "restore-files",
				ErrorCode: "restore_files_failed",
				ExitCode:  exitcode.RestoreError,
			}, func() (result.Result, error) {
				res := result.Result{
					DryRun: dryRun,
					Artifacts: result.RestoreFilesArtifacts{
						Manifest:  manifestPath,
						Files:     filesBackup,
						TargetDir: targetDir,
					},
					Details: result.RestoreFilesDetails{
						DryRun: dryRun,
					},
				}

				req := restore.RestoreFilesRequest{
					ManifestPath: manifestPath,
					FilesBackup:  filesBackup,
					TargetDir:    targetDir,
					DryRun:       dryRun,
				}

				plan, err := restore.RestoreFiles(req)
				if err != nil {
					return res, err
				}

				res.Details = result.RestoreFilesDetails{
					DryRun: dryRun,
					Plan:   restorePlanDetails(plan.Plan),
				}

				msg := "files restore completed"
				if dryRun {
					msg = "files restore dry-run passed"
				}

				var warnings []string
				if !dryRun {
					warnings = append(warnings, "files restore is destructive for the target directory")
				}

				res.Message = msg
				res.Warnings = warnings

				return res, nil
			})
		},
	}

	cmd.Flags().StringVar(&manifestPath, "manifest", "", "path to backup manifest.json")
	cmd.Flags().StringVar(&filesBackup, "files-backup", "", "path to files backup tar.gz")
	cmd.Flags().StringVar(&targetDir, "target-dir", "", "path to restore target directory")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "run preflight and lock checks without restoring")

	return cmd
}

func validateRestoreFilesInput(manifestPath, filesBackup, targetDir string) error {
	hasManifest := manifestPath != ""
	hasFilesBackup := filesBackup != ""
	switch {
	case hasManifest && hasFilesBackup:
		return usageError(fmt.Errorf("use either --manifest or --files-backup, not both"))
	case !hasManifest && !hasFilesBackup:
		return usageError(fmt.Errorf("--manifest or --files-backup is required"))
	}
	if err := requireNonBlankFlag("--target-dir", targetDir); err != nil {
		return err
	}

	cleanTarget := filepath.Clean(targetDir)
	if cleanTarget == "." {
		return usageError(fmt.Errorf("--target-dir must not be the current directory"))
	}
	if cleanTarget == string(filepath.Separator) {
		return usageError(fmt.Errorf("--target-dir must not be the filesystem root"))
	}

	return nil
}
