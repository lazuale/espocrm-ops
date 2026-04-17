package cli

import (
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

			operation, err := journalusecase.ShowOperation(journalusecase.ShowOperationInput{
				JournalDir: app.options.JournalDir,
				ID:         id,
			})
			if err != nil {
				return err
			}

			warnings := journalusecase.WarningsFromReadStats(operation.Stats)
			details := journalusecase.OperationLookupDetailsFromReadStats(operation.Stats)
			details.ID = id
			if app.IsJSONEnabled() {
				return result.Render(cmd.OutOrStdout(), result.Result{
					Command:  "show-operation",
					OK:       true,
					Warnings: warnings,
					Details:  details,
					Items:    []any{operation.Entry},
				}, true)
			}

			if err := renderEntryDetail(cmd.OutOrStdout(), operation.Entry); err != nil {
				return err
			}
			return renderWarnings(cmd.OutOrStdout(), warnings)
		},
	}

	cmd.Flags().StringVar(&id, "id", "", "operation id")

	return cmd
}
