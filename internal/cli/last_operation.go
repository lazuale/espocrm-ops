package cli

import (
	"fmt"
	"io"

	"github.com/lazuale/espocrm-ops/internal/contract/exitcode"
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

			return RunResultCommand(cmd, CommandSpec{
				Name:       "last-operation",
				ErrorCode:  "last_operation_failed",
				ExitCode:   exitcode.InternalError,
				RenderText: renderLastOperationText,
			}, func() (result.Result, error) {
				last, err := journalusecase.LastOperation(journalusecase.LastOperationInput{
					JournalDir: app.options.JournalDir,
					Command:    commandName,
				})
				if err != nil {
					return result.Result{}, err
				}

				details := journalusecase.OperationLookupDetailsFromReadStats(last.Stats)
				details.Command = commandName

				items := []any{}
				message := "no operation found"
				if last.Entry != nil {
					items = append(items, *last.Entry)
					message = ""
				}

				return result.Result{
					OK:       true,
					Message:  message,
					Warnings: journalusecase.WarningsFromReadStats(last.Stats),
					Items:    items,
					Details:  details,
				}, nil
			})
		},
	}

	cmd.Flags().StringVar(&commandName, "command", "", "filter by command name")

	return cmd
}

func renderLastOperationText(w io.Writer, res result.Result) error {
	if len(res.Items) == 0 {
		_, err := fmt.Fprintln(w, "no operation found")
		return err
	}

	entry, ok := res.Items[0].(journalusecase.Entry)
	if !ok {
		return fmt.Errorf("unexpected last operation item type %T", res.Items[0])
	}

	_, err := fmt.Fprintln(w, formatEntryLine(entry))
	return err
}
