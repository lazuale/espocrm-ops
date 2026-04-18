package cli

import (
	"fmt"
	"io"
	"strings"

	"github.com/lazuale/espocrm-ops/internal/contract/exitcode"
	"github.com/lazuale/espocrm-ops/internal/contract/result"
	journalusecase "github.com/lazuale/espocrm-ops/internal/usecase/journal"
	"github.com/spf13/cobra"
)

func newJournalPruneCmd() *cobra.Command {
	var keepDays int
	var keepLast int
	var keepLegacy int
	var dryRun bool

	cmd := &cobra.Command{
		Use:   "journal-prune",
		Short: "Prune old journal entries",
		Args:  noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			app := appForCommand(cmd)
			effectiveKeepLast, err := resolveJournalPruneKeepLast(cmd, keepLast, keepLegacy)
			if err != nil {
				return err
			}
			if err := validateJournalPruneInput(keepDays, effectiveKeepLast); err != nil {
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
					KeepLast:   effectiveKeepLast,
					DryRun:     dryRun,
				})
				if err != nil {
					return result.Result{}, err
				}

				message := fmt.Sprintf(
					"journal prune completed, retained=%d protected=%d removed=%d removed_dirs=%d",
					pruneResult.Retained,
					pruneResult.Protected,
					pruneResult.Deleted,
					pruneResult.RemovedDirs,
				)
				if dryRun {
					message = fmt.Sprintf(
						"journal prune preview ready, retained=%d protected=%d would_remove=%d",
						pruneResult.Retained,
						pruneResult.Protected,
						pruneResult.Deleted,
					)
				}

				return result.Result{
					OK:       true,
					Message:  message,
					Warnings: journalusecase.WarningsFromReadStats(pruneResult.ReadStats),
					DryRun:   dryRun,
					Details: journalusecase.PruneDetailsFromStats(
						pruneResult.ReadStats,
						pruneResult.Checked,
						pruneResult.Retained,
						pruneResult.Protected,
						pruneResult.Deleted,
						pruneResult.RemovedDirs,
						keepDays,
						effectiveKeepLast,
						pruneResult.LatestOperationID,
						dryRun,
					),
					Items: pruneItemsAsItems(pruneResult.Items),
				}, nil
			})
		},
	}

	cmd.Flags().IntVar(&keepDays, "keep-days", 0, "keep journal entries newer than N days")
	cmd.Flags().IntVar(&keepLast, "keep-last", 0, "keep at most N most recent journal entries")
	cmd.Flags().IntVar(&keepLegacy, "keep", 0, "legacy alias for --keep-last")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "show what would be deleted without deleting")
	_ = cmd.Flags().MarkHidden("keep")

	return cmd
}

func resolveJournalPruneKeepLast(cmd *cobra.Command, keepLast, keepLegacy int) (int, error) {
	keepLastChanged := cmd.Flags().Changed("keep-last")
	keepLegacyChanged := cmd.Flags().Changed("keep")
	if keepLastChanged && keepLegacyChanged && keepLast != keepLegacy {
		return 0, usageError(fmt.Errorf("use either --keep-last or --keep, not both"))
	}
	if keepLegacyChanged {
		return keepLegacy, nil
	}

	return keepLast, nil
}

func validateJournalPruneInput(keepDays, keepLast int) error {
	if keepDays < 0 {
		return usageError(fmt.Errorf("--keep-days must be non-negative"))
	}
	if keepLast < 0 {
		return usageError(fmt.Errorf("--keep-last must be non-negative"))
	}
	if keepDays == 0 && keepLast == 0 {
		return usageError(fmt.Errorf("provide --keep-days or --keep-last"))
	}

	return nil
}

func pruneItemsAsItems(items []journalusecase.PruneItem) []any {
	out := make([]any, 0, len(items))
	for _, item := range items {
		out = append(out, item)
	}
	return out
}

func renderJournalPruneText(w io.Writer, res result.Result) error {
	details, ok := res.Details.(result.PruneDetails)
	if !ok {
		return fmt.Errorf("unexpected journal prune details type %T", res.Details)
	}

	actionLabel := "removed"
	modeLabel := "journal prune completed"
	if details.DryRun {
		actionLabel = "would_remove"
		modeLabel = "journal prune dry-run"
	}

	if _, err := fmt.Fprintf(
		w,
		"%s: total_files_seen=%d loaded_entries=%d skipped_corrupt=%d checked=%d retained=%d protected=%d %s=%d removed_dirs=%d dry_run=%t",
		modeLabel,
		details.TotalFilesSeen,
		details.LoadedEntries,
		details.SkippedCorrupt,
		details.Checked,
		details.Retained,
		details.Protected,
		actionLabel,
		details.Deleted,
		details.RemovedDirs,
		details.DryRun,
	); err != nil {
		return err
	}
	if details.LatestOperationID != "" {
		if _, err := fmt.Fprintf(w, " latest_operation_id=%s", details.LatestOperationID); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintln(w); err != nil {
		return err
	}

	for _, raw := range res.Items {
		item, ok := raw.(journalusecase.PruneItem)
		if !ok {
			return fmt.Errorf("unexpected journal prune item type %T", raw)
		}
		line, err := formatJournalPruneLine(item)
		if err != nil {
			return err
		}
		if _, err := fmt.Fprintln(w, line); err != nil {
			return err
		}
	}

	return nil
}

func formatJournalPruneLine(item journalusecase.PruneItem) (string, error) {
	parts := []string{strings.ToUpper(item.Decision)}

	switch item.Kind {
	case journalusecase.PruneItemKindOperation:
		if item.Operation == nil {
			return "", fmt.Errorf("journal prune operation item is missing operation data")
		}
		parts = append(parts, formatHistoryLine(*item.Operation))
	case journalusecase.PruneItemKindDir:
		parts = append(parts, "dir")
	default:
		return "", fmt.Errorf("unexpected journal prune item kind %q", item.Kind)
	}

	if item.Path != "" {
		parts = append(parts, "path="+item.Path)
	}
	if len(item.Reasons) > 0 {
		parts = append(parts, "reasons="+strings.Join(item.Reasons, ","))
	}

	return strings.Join(parts, "  "), nil
}
