package cli

import (
	"fmt"

	backupverifyapp "github.com/lazuale/espocrm-ops/internal/app/backupverify"
	resultbridge "github.com/lazuale/espocrm-ops/internal/cli/resultbridge"
	"github.com/lazuale/espocrm-ops/internal/contract/exitcode"
	"github.com/lazuale/espocrm-ops/internal/contract/result"
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
			in := backupVerifyInput{
				manifestPath: manifestPath,
				backupRoot:   backupRoot,
			}
			if err := validateBackupVerifyInput(cmd, &in); err != nil {
				return err
			}

			return RunCommand(cmd, CommandSpec{
				Name:       "backup verify",
				ErrorCode:  "backup_verification_failed",
				ExitCode:   exitcode.ValidationError,
				RenderText: resultbridge.RenderBackupVerifyText,
			}, func() (result.Result, error) {
				res := resultbridge.BackupVerifyPendingResult(in.manifestPath)

				report, err := appForCommand(cmd).backupVerify.Diagnose(backupverifyapp.Request{
					ManifestPath: in.manifestPath,
					BackupRoot:   in.backupRoot,
				})
				if err != nil {
					return res, err
				}

				return resultbridge.BackupVerifyResult(report), nil
			})
		},
	}

	cmd.Flags().StringVar(&manifestPath, "manifest", "", "path to backup manifest.json")
	cmd.Flags().StringVar(&backupRoot, "backup-root", "", "backup root containing db, files and manifests directories")

	return cmd
}

type backupVerifyInput struct {
	manifestPath string
	backupRoot   string
}

func validateBackupVerifyInput(cmd *cobra.Command, in *backupVerifyInput) error {
	if err := normalizeOptionalAbsolutePathFlag(cmd, "manifest", &in.manifestPath); err != nil {
		return err
	}
	if err := normalizeOptionalAbsolutePathFlag(cmd, "backup-root", &in.backupRoot); err != nil {
		return err
	}

	hasManifest := in.manifestPath != ""
	hasBackupRoot := in.backupRoot != ""
	if hasManifest && hasBackupRoot {
		return usageError(fmt.Errorf("use either --manifest or --backup-root, not both"))
	}
	if hasManifest || hasBackupRoot {
		return nil
	}

	return usageError(fmt.Errorf("--manifest or --backup-root is required"))
}
