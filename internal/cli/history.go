package cli

import (
	"fmt"
	"time"

	"github.com/lazuale/espocrm-ops/internal/contract/result"
	journalusecase "github.com/lazuale/espocrm-ops/internal/usecase/journal"
	"github.com/spf13/cobra"
)

func newHistoryCmd() *cobra.Command {
	var limit int
	var commandName string
	var okOnly bool
	var failedOnly bool
	var sinceRaw string
	var untilRaw string

	cmd := &cobra.Command{
		Use:   "history",
		Short: "Show recent operation journal entries",
		Args:  noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			app := appForCommand(cmd)
			if err := normalizeOptionalStringFlag(cmd, "command", &commandName); err != nil {
				return err
			}
			if limit < 0 {
				return usageError(fmt.Errorf("--limit must be non-negative"))
			}
			if okOnly && failedOnly {
				return usageError(fmt.Errorf("use either --ok-only or --failed-only, not both"))
			}

			since, err := parseRFC3339Flag("--since", sinceRaw)
			if err != nil {
				return err
			}
			until, err := parseRFC3339Flag("--until", untilRaw)
			if err != nil {
				return err
			}
			if since != nil && until != nil && since.After(*until) {
				return usageError(fmt.Errorf("--since must be before or equal to --until"))
			}

			history, err := journalusecase.History(journalusecase.HistoryInput{
				JournalDir: app.options.JournalDir,
				Filters: journalusecase.Filters{
					Command:    commandName,
					OKOnly:     okOnly,
					FailedOnly: failedOnly,
					Since:      since,
					Until:      until,
					Limit:      limit,
				},
			})
			if err != nil {
				return err
			}

			warnings := journalusecase.WarningsFromReadStats(history.Stats)
			details := journalusecase.HistoryDetailsFromReadStats(history.Stats)
			details.Limit = limit
			details.Command = commandName
			details.OKOnly = okOnly
			details.FailedOnly = failedOnly
			details.Since = sinceRaw
			details.Until = untilRaw

			if app.IsJSONEnabled() {
				return result.Render(cmd.OutOrStdout(), result.Result{
					Command:  "history",
					OK:       true,
					Message:  fmt.Sprintf("found %d operations", len(history.Entries)),
					Warnings: warnings,
					Items:    journalEntriesAsItems(history.Entries),
					Details:  details,
				}, true)
			}

			if len(history.Entries) == 0 {
				if _, err := fmt.Fprintln(cmd.OutOrStdout(), "no operations found"); err != nil {
					return err
				}
			} else {
				for _, entry := range history.Entries {
					if _, err := fmt.Fprintln(cmd.OutOrStdout(), formatEntryLine(entry)); err != nil {
						return err
					}
				}
			}

			return renderWarnings(cmd.OutOrStdout(), warnings)
		},
	}

	cmd.Flags().IntVar(&limit, "limit", 10, "maximum number of operations to show")
	cmd.Flags().StringVar(&commandName, "command", "", "filter by command name")
	cmd.Flags().BoolVar(&okOnly, "ok-only", false, "show only successful operations")
	cmd.Flags().BoolVar(&failedOnly, "failed-only", false, "show only failed operations")
	cmd.Flags().StringVar(&sinceRaw, "since", "", "show operations since RFC3339 timestamp")
	cmd.Flags().StringVar(&untilRaw, "until", "", "show operations until RFC3339 timestamp")

	return cmd
}

func journalEntriesAsItems(entries []journalusecase.Entry) []any {
	items := make([]any, 0, len(entries))
	for _, entry := range entries {
		items = append(items, entry)
	}
	return items
}

func parseRFC3339Flag(name, raw string) (*time.Time, error) {
	if raw == "" {
		return nil, nil
	}

	parsed, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return nil, usageError(fmt.Errorf("invalid %s value: %w", name, err))
	}

	return &parsed, nil
}
