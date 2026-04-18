package cli

import (
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/lazuale/espocrm-ops/internal/contract/exitcode"
	"github.com/lazuale/espocrm-ops/internal/contract/result"
	journalusecase "github.com/lazuale/espocrm-ops/internal/usecase/journal"
	"github.com/spf13/cobra"
)

func newHistoryCmd() *cobra.Command {
	var limit int
	var commandName string
	var okOnly bool
	var failedOnly bool
	var status string
	var scope string
	var recoveryOnly bool
	var targetPrefix string
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
			if err := normalizeOptionalStringFlag(cmd, "status", &status); err != nil {
				return err
			}
			if err := normalizeOptionalStringFlag(cmd, "scope", &scope); err != nil {
				return err
			}
			if err := normalizeOptionalStringFlag(cmd, "target-prefix", &targetPrefix); err != nil {
				return err
			}
			if limit < 0 {
				return usageError(fmt.Errorf("--limit must be non-negative"))
			}
			if okOnly && failedOnly {
				return usageError(fmt.Errorf("use either --ok-only or --failed-only, not both"))
			}
			status = strings.ToLower(status)
			if status != "" && !isValidHistoryStatus(status) {
				return usageError(fmt.Errorf("--status must be one of: completed, failed, blocked, running, unknown"))
			}
			if status != "" && (okOnly || failedOnly) {
				return usageError(fmt.Errorf("use either --status or --ok-only/--failed-only, not both"))
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

			return RunResultCommand(cmd, CommandSpec{
				Name:       "history",
				ErrorCode:  "history_failed",
				ExitCode:   exitcode.InternalError,
				RenderText: renderHistoryText,
			}, func() (result.Result, error) {
				history, err := journalusecase.History(journalusecase.HistoryInput{
					JournalDir: app.options.JournalDir,
					Filters: journalusecase.Filters{
						Command:      commandName,
						OKOnly:       okOnly,
						FailedOnly:   failedOnly,
						Status:       status,
						Scope:        scope,
						RecoveryOnly: recoveryOnly,
						TargetPrefix: targetPrefix,
						Since:        since,
						Until:        until,
						Limit:        limit,
					},
				})
				if err != nil {
					return result.Result{}, err
				}

				details := journalusecase.HistoryDetailsFromReadStats(history.Stats)
				details.Limit = limit
				details.Command = commandName
				details.OKOnly = okOnly
				details.FailedOnly = failedOnly
				details.Status = status
				details.Scope = scope
				details.RecoveryOnly = recoveryOnly
				details.TargetPrefix = targetPrefix
				details.Returned = len(history.Operations)
				details.RecentFirst = true
				details.Since = sinceRaw
				details.Until = untilRaw

				return result.Result{
					OK:       true,
					Message:  fmt.Sprintf("found %d operations", len(history.Operations)),
					Warnings: journalusecase.WarningsFromReadStats(history.Stats),
					Items:    operationSummariesAsItems(history.Operations),
					Details:  details,
				}, nil
			})
		},
	}

	cmd.Flags().IntVar(&limit, "limit", 10, "maximum number of operations to show")
	cmd.Flags().StringVar(&commandName, "command", "", "filter by command name")
	cmd.Flags().BoolVar(&okOnly, "ok-only", false, "show only successful operations")
	cmd.Flags().BoolVar(&failedOnly, "failed-only", false, "show only failed operations")
	cmd.Flags().StringVar(&status, "status", "", "filter by operation status")
	cmd.Flags().StringVar(&scope, "scope", "", "filter by operation scope")
	cmd.Flags().BoolVar(&recoveryOnly, "recovery-only", false, "show only recovery runs")
	cmd.Flags().StringVar(&targetPrefix, "target-prefix", "", "filter by rollback target prefix")
	cmd.Flags().StringVar(&sinceRaw, "since", "", "show operations since RFC3339 timestamp")
	cmd.Flags().StringVar(&untilRaw, "until", "", "show operations until RFC3339 timestamp")

	return cmd
}

func operationSummariesAsItems(operations []journalusecase.OperationSummary) []any {
	items := make([]any, 0, len(operations))
	for _, operation := range operations {
		items = append(items, operation)
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

func renderHistoryText(w io.Writer, res result.Result) error {
	if len(res.Items) == 0 {
		_, err := fmt.Fprintln(w, "no operations found")
		return err
	}

	for _, raw := range res.Items {
		operation, ok := raw.(journalusecase.OperationSummary)
		if !ok {
			return fmt.Errorf("unexpected history item type %T", raw)
		}
		if _, err := fmt.Fprintln(w, formatHistoryLine(operation)); err != nil {
			return err
		}
	}

	return nil
}

func isValidHistoryStatus(status string) bool {
	switch status {
	case journalusecase.OperationStatusCompleted,
		journalusecase.OperationStatusFailed,
		journalusecase.OperationStatusBlocked,
		journalusecase.OperationStatusRunning,
		journalusecase.OperationStatusUnknown:
		return true
	default:
		return false
	}
}
