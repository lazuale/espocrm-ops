package cli

import (
	v3config "github.com/lazuale/espocrm-ops/internal/v3/config"
	"github.com/lazuale/espocrm-ops/internal/v3/ops"
	v3runtime "github.com/lazuale/espocrm-ops/internal/v3/runtime"
	"github.com/spf13/cobra"
)

func newDoctorCmd() *cobra.Command {
	var scope string
	var projectDir string

	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Check whether v3 backup and restore prerequisites are ready",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			result, err := ops.Doctor(cmd.Context(), v3config.BackupRequest{
				Scope:      scope,
				ProjectDir: projectDir,
			}, v3runtime.DockerCompose{})
			if err != nil {
				return doctorCommandError(result, err)
			}

			return writeJSON(cmd.OutOrStdout(), envelope{
				Command:  "doctor",
				OK:       true,
				Message:  "doctor passed",
				Error:    nil,
				Warnings: []string{},
				Result:   result,
			})
		},
	}

	cmd.Flags().StringVar(&scope, "scope", "", "doctor contour")
	cmd.Flags().StringVar(&projectDir, "project-dir", ".", "project directory containing the compose file and env files")
	return cmd
}

func doctorCommandError(result ops.DoctorResult, err error) error {
	verifyErr, ok := err.(*ops.VerifyError)
	if !ok {
		return &commandError{
			command:  "doctor",
			kind:     ops.ErrorKindIO,
			exitCode: exitIO,
			message:  "doctor failed",
			err:      err,
			result:   result,
		}
	}

	return &commandError{
		command:  "doctor",
		kind:     verifyErr.Kind,
		exitCode: backupExitCode(verifyErr.Kind),
		message:  "doctor failed",
		err:      verifyErr,
		result:   result,
	}
}
