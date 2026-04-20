package cli

import (
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/lazuale/espocrm-ops/internal/contract/apperr"
	"github.com/lazuale/espocrm-ops/internal/contract/exitcode"
	"github.com/lazuale/espocrm-ops/internal/contract/result"
	doctorusecase "github.com/lazuale/espocrm-ops/internal/usecase/doctor"
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

			report, err := doctorusecase.Diagnose(doctorusecase.Request{
				Scope:           in.scope,
				ProjectDir:      in.projectDir,
				ComposeFile:     in.composeFile,
				EnvFileOverride: in.envFile,
				EnvContourHint:  envFileContourHint(),
			})
			if err != nil {
				return err
			}

			res := doctorResult(report)
			if report.Ready() {
				return renderCommandResult(cmd, CommandSpec{
					Name:       "doctor",
					RenderText: renderDoctorText,
				}, res)
			}

			errCode := CodeError{
				Code:    exitcode.ValidationError,
				Err:     apperr.Wrap(apperr.KindValidation, "doctor_failed", errors.New("doctor found readiness failures")),
				ErrCode: "doctor_failed",
			}

			if appForCommand(cmd).JSONEnabled() {
				return ResultCodeError{
					CodeError: errCode,
					Result:    res,
				}
			}

			if err := renderDoctorText(cmd.OutOrStdout(), res); err != nil {
				return err
			}
			return silentCodeError{CodeError: errCode}
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

func doctorResult(report doctorusecase.Report) result.Result {
	passed, warnings, failed := report.Counts()
	message := "doctor passed"
	if !report.Ready() {
		message = "doctor found readiness failures"
	}

	items := make([]any, 0, len(report.Checks))
	for _, check := range report.Checks {
		items = append(items, result.DoctorCheck{
			Scope:   check.Scope,
			Code:    check.Code,
			Status:  check.Status,
			Summary: check.Summary,
			Details: check.Details,
			Action:  check.Action,
		})
	}

	scopes := make([]result.DoctorScopeArtifact, 0, len(report.Scopes))
	for _, scope := range report.Scopes {
		scopes = append(scopes, result.DoctorScopeArtifact{
			Scope:      scope.Scope,
			EnvFile:    scope.EnvFile,
			BackupRoot: scope.BackupRoot,
		})
	}

	res := result.Result{
		Command: "doctor",
		OK:      report.Ready(),
		Message: message,
		Details: result.DoctorDetails{
			TargetScope: report.TargetScope,
			Ready:       report.Ready(),
			Checks:      len(report.Checks),
			Passed:      passed,
			Warnings:    warnings,
			Failed:      failed,
		},
		Artifacts: result.DoctorArtifacts{
			ProjectDir:  report.ProjectDir,
			ComposeFile: report.ComposeFile,
			Scopes:      scopes,
		},
		Items: items,
	}

	if !report.Ready() {
		res.Error = &result.ErrorInfo{
			Code:     "doctor_failed",
			Kind:     string(apperr.KindValidation),
			ExitCode: exitcode.ValidationError,
			Message:  message,
		}
	}

	return res
}

func renderDoctorText(w io.Writer, res result.Result) error {
	details, ok := res.Details.(result.DoctorDetails)
	if !ok {
		return result.Render(w, res, false)
	}

	artifacts, ok := res.Artifacts.(result.DoctorArtifacts)
	if !ok {
		return fmt.Errorf("unexpected doctor artifacts type %T", res.Artifacts)
	}

	if _, err := fmt.Fprintln(w, "EspoCRM doctor"); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "Target scope:    %s\n", details.TargetScope); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "Project dir:     %s\n", artifacts.ProjectDir); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "Compose file:    %s\n", artifacts.ComposeFile); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(w, "\nSummary:"); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "  Ready:         %t\n", details.Ready); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "  Checks:        %d\n", details.Checks); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "  Passed:        %d\n", details.Passed); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "  Warnings:      %d\n", details.Warnings); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "  Failed:        %d\n", details.Failed); err != nil {
		return err
	}

	if _, err := fmt.Fprintln(w, "\nChecks:"); err != nil {
		return err
	}
	for _, rawItem := range res.Items {
		check, ok := rawItem.(result.DoctorCheck)
		if !ok {
			return fmt.Errorf("unexpected doctor item type %T", rawItem)
		}
		prefix := fmt.Sprintf("[%s]", strings.ToUpper(check.Status))
		if strings.TrimSpace(check.Scope) != "" {
			prefix = fmt.Sprintf("[%s][%s]", check.Scope, strings.ToUpper(check.Status))
		}
		if _, err := fmt.Fprintf(w, "%s %s\n", prefix, check.Summary); err != nil {
			return err
		}
		if strings.TrimSpace(check.Details) != "" {
			if _, err := fmt.Fprintf(w, "  Details: %s\n", check.Details); err != nil {
				return err
			}
		}
		if strings.TrimSpace(check.Action) != "" {
			if _, err := fmt.Fprintf(w, "  Action:  %s\n", check.Action); err != nil {
				return err
			}
		}
	}

	return nil
}
