package cli

import (
	"fmt"
	"io"

	"github.com/lazuale/espocrm-ops/internal/contract/exitcode"
	"github.com/lazuale/espocrm-ops/internal/contract/result"
	journalusecase "github.com/lazuale/espocrm-ops/internal/usecase/journal"
	"github.com/spf13/cobra"
)

func newJournalPruneCmd() *cobra.Command {
	var keepDays int
	var keep int
	var dryRun bool

	cmd := &cobra.Command{
		Use:   "journal-prune",
		Short: "Prune old journal entries",
		Args:  noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			app := appForCommand(cmd)
			if err := validateJournalPruneInput(keepDays, keep); err != nil {
				return err
			}

			return RunResultCommand(cmd, CommandSpec{
				Name:       "journal-prune",
				ErrorCode:  "journal_prune_failed",
				ExitCode:   exitcode.InternalError,
				RenderText: renderJournalPruneText,
			}, func() (result.Result, error) {
				pruneResult, err := journalusecase.Prune(journalusecase.PruneInput{
					JournalDir: app.options.JournalDir,
					KeepDays:   keepDays,
					Keep:       keep,
					DryRun:     dryRun,
				})
				if err != nil {
					return result.Result{}, err
				}

				return result.Result{
					OK:       true,
					Message:  fmt.Sprintf("journal prune completed, deleted=%d removed_dirs=%d", pruneResult.Deleted, pruneResult.RemovedDirs),
					Warnings: journalusecase.WarningsFromReadStats(pruneResult.ReadStats),
					DryRun:   dryRun,
					Details: journalusecase.PruneDetailsFromStats(
						pruneResult.ReadStats,
						pruneResult.Checked,
						pruneResult.Deleted,
						pruneResult.RemovedDirs,
						keepDays,
						keep,
						dryRun,
					),
					Items: prunePathsAsItems(pruneResult),
				}, nil
			})
		},
	}

	cmd.Flags().IntVar(&keepDays, "keep-days", 0, "keep journal entries newer than N days")
	cmd.Flags().IntVar(&keep, "keep", 0, "keep at most N most recent journal entries")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "show what would be deleted without deleting")

	return cmd
}

func validateJournalPruneInput(keepDays, keep int) error {
	if keepDays < 0 {
		return usageError(fmt.Errorf("--keep-days must be non-negative"))
	}
	if keep < 0 {
		return usageError(fmt.Errorf("--keep must be non-negative"))
	}
	if keepDays == 0 && keep == 0 {
		return usageError(fmt.Errorf("provide --keep-days or --keep"))
	}

	return nil
}

func prunePathsAsItems(pruneResult journalusecase.PruneOutput) []any {
	items := make([]any, 0, len(pruneResult.Paths)+len(pruneResult.RemovedPaths))
	for _, path := range pruneResult.Paths {
		items = append(items, result.PruneItem{Type: "file", Path: path})
	}
	for _, path := range pruneResult.RemovedPaths {
		items = append(items, result.PruneItem{Type: "dir", Path: path})
	}
	return items
}

func renderJournalPruneText(w io.Writer, res result.Result) error {
	details, ok := res.Details.(result.PruneDetails)
	if !ok {
		return fmt.Errorf("unexpected journal prune details type %T", res.Details)
	}

	_, err := fmt.Fprintf(
		w,
		"journal prune completed: total_files_seen=%d loaded_entries=%d skipped_corrupt=%d deleted=%d removed_dirs=%d dry_run=%t\n",
		details.TotalFilesSeen,
		details.LoadedEntries,
		details.SkippedCorrupt,
		details.Deleted,
		details.RemovedDirs,
		details.DryRun,
	)
	return err
}
