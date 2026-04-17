package cli

import (
	"fmt"

	"github.com/lazuale/espocrm-ops/internal/contract/result"
	journalusecase "github.com/lazuale/espocrm-ops/internal/usecase/journal"
	"github.com/spf13/cobra"
)

func newLastOperationCmd() *cobra.Command {
	var commandName string

	cmd := &cobra.Command{
		Use:   "last-operation",
		Short: "Show the most recent operation",
		Args:  noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			app := appForCommand(cmd)
			if err := normalizeOptionalStringFlag(cmd, "command", &commandName); err != nil {
				return err
			}

			last, err := journalusecase.LastOperation(journalusecase.LastOperationInput{
				JournalDir: app.options.JournalDir,
				Command:    commandName,
			})
			if err != nil {
				return err
			}
			warnings := journalusecase.WarningsFromReadStats(last.Stats)
			details := journalusecase.OperationLookupDetailsFromReadStats(last.Stats)
			details.Command = commandName

			if last.Entry != nil {
				if app.IsJSONEnabled() {
					return result.Render(cmd.OutOrStdout(), result.Result{
						Command:  "last-operation",
						OK:       true,
						Warnings: warnings,
						Items:    []any{*last.Entry},
						Details:  details,
					}, true)
				}

				if _, err := fmt.Fprintln(cmd.OutOrStdout(), formatEntryLine(*last.Entry)); err != nil {
					return err
				}
				return renderWarnings(cmd.OutOrStdout(), warnings)
			}

			if app.IsJSONEnabled() {
				return result.Render(cmd.OutOrStdout(), result.Result{
					Command:  "last-operation",
					OK:       true,
					Message:  "no operation found",
					Warnings: warnings,
					Items:    []any{},
					Details:  details,
				}, true)
			}

			if _, err = fmt.Fprintln(cmd.OutOrStdout(), "no operation found"); err != nil {
				return err
			}
			return renderWarnings(cmd.OutOrStdout(), warnings)
		},
	}

	cmd.Flags().StringVar(&commandName, "command", "", "filter by command name")

	return cmd
}
