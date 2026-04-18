package cli

import (
	"fmt"
	"io"

	"github.com/lazuale/espocrm-ops/internal/contract/exitcode"
	"github.com/lazuale/espocrm-ops/internal/contract/result"
	journalusecase "github.com/lazuale/espocrm-ops/internal/usecase/journal"
	"github.com/spf13/cobra"
)

func newShowOperationCmd() *cobra.Command {
	var id string

	cmd := &cobra.Command{
		Use:   "show-operation",
		Short: "Show one operation by id",
		Args:  noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			app := appForCommand(cmd)
			if err := requireNonBlankFlag("--id", id); err != nil {
				return err
			}

			return RunResultCommand(cmd, CommandSpec{
				Name:       "show-operation",
				ErrorCode:  "show_operation_failed",
				ExitCode:   exitcode.InternalError,
				RenderText: renderShowOperationText,
			}, func() (result.Result, error) {
				operation, err := journalusecase.ShowOperation(journalusecase.ShowOperationInput{
					JournalDir: app.options.JournalDir,
					ID:         id,
				})
				if err != nil {
					return result.Result{}, err
				}

				details := journalusecase.OperationLookupDetailsFromReadStats(operation.Stats)
				details.ID = id
				return result.Result{
					OK:       true,
					Warnings: journalusecase.WarningsFromReadStats(operation.Stats),
					Details:  details,
					Items:    []any{journalusecase.Explain(operation.Entry)},
				}, nil
			})
		},
	}

	cmd.Flags().StringVar(&id, "id", "", "operation id")

	return cmd
}

func renderShowOperationText(w io.Writer, res result.Result) error {
	if len(res.Items) != 1 {
		return fmt.Errorf("expected one operation item, got %d", len(res.Items))
	}

	report, ok := res.Items[0].(journalusecase.OperationReport)
	if !ok {
		return fmt.Errorf("unexpected show-operation item type %T", res.Items[0])
	}

	return renderOperationReportText(w, report)
}
