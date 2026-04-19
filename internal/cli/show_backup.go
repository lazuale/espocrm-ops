package cli

import (
	"fmt"
	"io"
	"strings"

	"github.com/lazuale/espocrm-ops/internal/contract/exitcode"
	"github.com/lazuale/espocrm-ops/internal/contract/result"
	"github.com/lazuale/espocrm-ops/internal/usecase/backup"
	"github.com/spf13/cobra"
)

func newShowBackupCmd() *cobra.Command {
	var backupRoot string
	var id string
	var verifyChecksum bool

	cmd := &cobra.Command{
		Use:   "show-backup",
		Short: "Show one backup set by id",
		Args:  noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			app := appForCommand(cmd)
			in := showBackupInput{
				backupRoot:     backupRoot,
				id:             id,
				verifyChecksum: verifyChecksum,
			}
			if err := validateShowBackupInput(&in); err != nil {
				return err
			}

			return RunResultCommand(cmd, CommandSpec{
				Name:       "show-backup",
				ErrorCode:  "show_backup_failed",
				ExitCode:   exitcode.InternalError,
				RenderText: renderShowBackupText,
			}, func() (result.Result, error) {
				info, err := backup.Show(backup.ShowRequest{
					BackupRoot:     in.backupRoot,
					ID:             in.id,
					JournalDir:     app.options.JournalDir,
					VerifyChecksum: in.verifyChecksum,
					Now:            app.runtime.Now(),
				})
				if err != nil {
					return result.Result{}, err
				}

				return result.Result{
					OK:       true,
					Message:  "backup inspection loaded",
					Warnings: backupJournalWarnings(info.JournalRead),
					Details: result.BackupShowDetails{
						JournalReadDetails: backupJournalReadDetails(info.JournalRead),
						BackupRoot:         info.BackupRoot,
						ID:                 info.ID,
						VerifyChecksum:     info.VerifyChecksum,
					},
					Items: []any{info.Item},
				}, nil
			})
		},
	}

	cmd.Flags().StringVar(&backupRoot, "backup-root", "", "backup root containing db, files and manifests directories")
	cmd.Flags().StringVar(&id, "id", "", "canonical backup set id from backup-catalog")
	cmd.Flags().BoolVar(&verifyChecksum, "verify-checksum", false, "verify checksum sidecars")

	return cmd
}

type showBackupInput struct {
	backupRoot     string
	id             string
	verifyChecksum bool
}

func validateShowBackupInput(in *showBackupInput) error {
	in.backupRoot = strings.TrimSpace(in.backupRoot)
	in.id = strings.TrimSpace(in.id)
	if err := requireNonBlankFlag("--backup-root", in.backupRoot); err != nil {
		return err
	}
	if err := requireNonBlankFlag("--id", in.id); err != nil {
		return err
	}

	return nil
}

func renderShowBackupText(w io.Writer, res result.Result) error {
	details, ok := res.Details.(result.BackupShowDetails)
	if !ok {
		return result.Render(w, res, false)
	}
	if len(res.Items) != 1 {
		return fmt.Errorf("expected one backup item, got %d", len(res.Items))
	}

	item, ok := res.Items[0].(backup.CatalogItem)
	if !ok {
		return fmt.Errorf("unexpected show-backup item type %T", res.Items[0])
	}

	if _, err := fmt.Fprintln(w, "EspoCRM backup inspection"); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "Backup directory:         %s\n", details.BackupRoot); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "Backup id:               %s\n", details.ID); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "Checksum verification:   %s\n", enabledText(details.VerifyChecksum)); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "Journal files scanned:   %d\n", details.TotalFilesSeen); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "Readiness:               %s\n", readinessText(item.RestoreReadiness)); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "Source:                  %s\n", item.Origin.Label); err != nil {
		return err
	}
	if err := renderBackupIdentityText(w, item); err != nil {
		return err
	}

	if _, err := fmt.Fprintln(w, "\nArtifacts:"); err != nil {
		return err
	}
	return renderBackupArtifactsText(w, item)
}
