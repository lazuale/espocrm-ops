package cli

import (
	"errors"
	"fmt"

	doctorusecase "github.com/lazuale/espocrm-ops/internal/app/doctor"
	errortransport "github.com/lazuale/espocrm-ops/internal/cli/errortransport"
	resultbridge "github.com/lazuale/espocrm-ops/internal/cli/resultbridge"
	"github.com/lazuale/espocrm-ops/internal/contract/exitcode"
	"github.com/lazuale/espocrm-ops/internal/contract/result"
	"github.com/spf13/cobra"
)

func newDoctorCmd() *cobra.Command {
	var scope string
	var projectDir string
	var composeFile string
	var envFile string

	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Validate operational readiness before stateful operations",
		Args:  noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			in := doctorInput{
				scope:       scope,
				projectDir:  projectDir,
				composeFile: composeFile,
				envFile:     envFile,
			}
			if err := validateDoctorInput(cmd, &in); err != nil {
				return err
			}

			return runDoctor(cmd, in)
		},
	}

	cmd.Flags().StringVar(&scope, "scope", "", "readiness target: dev, prod, or all")
	cmd.Flags().StringVar(&projectDir, "project-dir", ".", "project directory containing the compose file and env files")
	cmd.Flags().StringVar(&composeFile, "compose-file", "", "compose file path (defaults to project-dir/compose.yaml)")
	cmd.Flags().StringVar(&envFile, "env-file", "", "override env file path for a single contour check")

	return cmd
}

type doctorInput struct {
	scope       string
	projectDir  string
	composeFile string
	envFile     string
}

func validateDoctorInput(cmd *cobra.Command, in *doctorInput) error {
	if err := normalizeDoctorScopeFlag(&in.scope); err != nil {
		return err
	}
	if err := normalizeProjectContext(cmd, &in.projectDir, &in.composeFile, &in.envFile); err != nil {
		return err
	}
	if in.scope == "all" && in.envFile != "" {
		return usageError(fmt.Errorf("--env-file cannot be used with --scope all"))
	}

	return nil
}

func runDoctor(cmd *cobra.Command, in doctorInput) error {
	app := appForCommand(cmd)
	return RunDiagnosticCommand(cmd, CommandSpec{
		Name:       "doctor",
		ErrorCode:  "doctor_failed",
		ExitCode:   exitcode.ValidationError,
		RenderText: resultbridge.RenderDoctorText,
	}, func() (result.Result, error) {
		report, err := app.doctor.Diagnose(doctorusecase.Request{
			Scope:           in.scope,
			ProjectDir:      in.projectDir,
			ComposeFile:     in.composeFile,
			EnvFileOverride: in.envFile,
		})
		if err != nil {
			return result.Result{}, err
		}

		res := resultbridge.DoctorResult(report)
		if report.Ready() {
			return res, nil
		}

		return res, errortransport.CodeError{
			Code:    exitcode.ValidationError,
			Err:     errors.New("doctor found readiness failures"),
			ErrCode: "doctor_failed",
		}
	})
}
