package cli

import (
	"github.com/lazuale/espocrm-ops/internal/config"
	"github.com/lazuale/espocrm-ops/internal/ops"
	"github.com/lazuale/espocrm-ops/internal/runtime"
	"github.com/spf13/cobra"
)

func newDoctorCmd() *cobra.Command {
	var scope string
	var projectDir string

	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Check backup and restore prerequisites",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			result, err := ops.Doctor(cmd.Context(), config.Request{
				Scope:      scope,
				ProjectDir: projectDir,
			}, runtime.DockerCompose{})
			if err != nil {
				return opsCommandError("doctor", "doctor failed", result, err)
			}

			return writeJSON(cmd.OutOrStdout(), envelope{
				Command: "doctor",
				OK:      true,
				Message: "doctor passed",
				Error:   nil,
				Result:  result,
			})
		},
	}

	cmd.Flags().StringVar(&scope, "scope", "", "doctor scope")
	cmd.Flags().StringVar(&projectDir, "project-dir", ".", "project directory containing compose.yaml and .env.<scope>")
	return cmd
}
