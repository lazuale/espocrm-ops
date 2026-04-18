package cli

import (
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/lazuale/espocrm-ops/internal/contract/apperr"
	"github.com/lazuale/espocrm-ops/internal/contract/exitcode"
	"github.com/lazuale/espocrm-ops/internal/contract/result"
	updateusecase "github.com/lazuale/espocrm-ops/internal/usecase/update"
	"github.com/spf13/cobra"
)

func newUpdatePlanCmd() *cobra.Command {
	var scope string
	var projectDir string
	var composeFile string
	var envFile string
	var timeoutSeconds int
	var skipDoctor bool
	var skipBackup bool
	var skipPull bool
	var skipHTTPProbe bool

	cmd := &cobra.Command{
		Use:   "update-plan",
		Short: "Show what an update would do without making changes",
		Args:  noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			in := updatePlanInput{
				scope:          scope,
				projectDir:     projectDir,
				composeFile:    composeFile,
				envFile:        envFile,
				timeoutSeconds: timeoutSeconds,
				skipDoctor:     skipDoctor,
				skipBackup:     skipBackup,
				skipPull:       skipPull,
				skipHTTPProbe:  skipHTTPProbe,
			}
			if err := validateUpdatePlanInput(cmd, &in); err != nil {
				return err
			}

			plan, err := updateusecase.BuildPlan(updateusecase.PlanRequest{
				Scope:           in.scope,
				ProjectDir:      in.projectDir,
				ComposeFile:     in.composeFile,
				EnvFileOverride: in.envFile,
				EnvContourHint:  envFileContourHint(),
				TimeoutSeconds:  in.timeoutSeconds,
				SkipDoctor:      in.skipDoctor,
				SkipBackup:      in.skipBackup,
				SkipPull:        in.skipPull,
				SkipHTTPProbe:   in.skipHTTPProbe,
			})
			if err != nil {
				return err
			}

			res := updatePlanResult(plan)
			if plan.Ready() {
				return renderCommandResult(cmd, CommandSpec{
					Name:       "update-plan",
					RenderText: renderUpdatePlanText,
				}, res)
			}

			errCode := CodeError{
				Code:    exitcode.ValidationError,
				Err:     apperr.Wrap(apperr.KindValidation, "update_plan_blocked", errors.New("update dry-run plan found blocking conditions")),
				ErrCode: "update_plan_blocked",
			}

			if appForCommand(cmd).JSONEnabled() {
				return ResultCodeError{
					CodeError: errCode,
					Result:    res,
				}
			}

			if err := renderUpdatePlanText(cmd.OutOrStdout(), res); err != nil {
				return err
			}
			return silentCodeError{CodeError: errCode}
		},
	}

	cmd.Flags().StringVar(&scope, "scope", "", "update contour")
	cmd.Flags().StringVar(&projectDir, "project-dir", ".", "project directory containing the compose file and env files")
	cmd.Flags().StringVar(&composeFile, "compose-file", "", "compose file path (defaults to project-dir/compose.yaml)")
	cmd.Flags().StringVar(&envFile, "env-file", "", "override env file path")
	cmd.Flags().IntVar(&timeoutSeconds, "timeout", 600, "shared readiness timeout in seconds")
	cmd.Flags().BoolVar(&skipDoctor, "skip-doctor", false, "skip the doctor step in the planned flow")
	cmd.Flags().BoolVar(&skipBackup, "skip-backup", false, "skip the recovery-point step in the planned flow")
	cmd.Flags().BoolVar(&skipPull, "skip-pull", false, "skip image pull in the planned runtime apply step")
	cmd.Flags().BoolVar(&skipHTTPProbe, "skip-http-probe", false, "skip the final HTTP probe in the planned runtime readiness step")

	return cmd
}

type updatePlanInput struct {
	scope          string
	projectDir     string
	composeFile    string
	envFile        string
	timeoutSeconds int
	skipDoctor     bool
	skipBackup     bool
	skipPull       bool
	skipHTTPProbe  bool
}

func validateUpdatePlanInput(cmd *cobra.Command, in *updatePlanInput) error {
	in.scope = strings.TrimSpace(in.scope)
	in.projectDir = strings.TrimSpace(in.projectDir)
	in.composeFile = strings.TrimSpace(in.composeFile)
	in.envFile = strings.TrimSpace(in.envFile)

	switch in.scope {
	case "dev", "prod":
	default:
		return usageError(fmt.Errorf("--scope must be dev or prod"))
	}

	if err := requireNonBlankFlag("--project-dir", in.projectDir); err != nil {
		return err
	}

	projectAbs, err := filepath.Abs(filepath.Clean(in.projectDir))
	if err != nil {
		return usageError(fmt.Errorf("resolve --project-dir: %w", err))
	}
	in.projectDir = projectAbs

	if err := normalizeOptionalStringFlag(cmd, "compose-file", &in.composeFile); err != nil {
		return err
	}
	if in.composeFile == "" {
		in.composeFile = filepath.Join(in.projectDir, "compose.yaml")
	} else if !filepath.IsAbs(in.composeFile) {
		in.composeFile = filepath.Join(in.projectDir, in.composeFile)
	}
	in.composeFile = filepath.Clean(in.composeFile)

	if err := normalizeOptionalStringFlag(cmd, "env-file", &in.envFile); err != nil {
		return err
	}
	if in.envFile != "" && !filepath.IsAbs(in.envFile) {
		in.envFile = filepath.Join(in.projectDir, in.envFile)
	}
	if in.envFile != "" {
		in.envFile = filepath.Clean(in.envFile)
	}

	if in.timeoutSeconds < 0 {
		return usageError(fmt.Errorf("--timeout must be non-negative"))
	}

	return nil
}

func updatePlanResult(plan updateusecase.UpdatePlan) result.Result {
	wouldRun, skipped, blocked, unknown := plan.Counts()

	message := "update dry-run plan completed"
	if plan.Recovery.Active() {
		message = "update recovery plan completed"
	}
	if !plan.Ready() {
		message = "update dry-run plan found blocking conditions"
		if plan.Recovery.Active() {
			message = "update recovery plan found blocking conditions"
		}
	}

	items := make([]any, 0, len(plan.Steps))
	for _, step := range plan.Steps {
		items = append(items, result.UpdatePlanItem{
			Code:    step.Code,
			Status:  step.Status,
			Summary: step.Summary,
			Details: step.Details,
			Action:  step.Action,
		})
	}

	res := result.Result{
		Command:  "update-plan",
		OK:       plan.Ready(),
		Message:  message,
		DryRun:   true,
		Warnings: append([]string(nil), plan.Warnings...),
		Details: result.UpdatePlanDetails{
			Scope:          plan.Scope,
			Ready:          plan.Ready(),
			Steps:          len(plan.Steps),
			WouldRun:       wouldRun,
			Skipped:        skipped,
			Blocked:        blocked,
			Unknown:        unknown,
			Warnings:       len(plan.Warnings),
			TimeoutSeconds: plan.TimeoutSeconds,
			SkipDoctor:     plan.SkipDoctor,
			SkipBackup:     plan.SkipBackup,
			SkipPull:       plan.SkipPull,
			SkipHTTPProbe:  plan.SkipHTTPProbe,
			Recovery:       recoveryResultDetails(plan.Recovery),
		},
		Artifacts: result.UpdatePlanArtifacts{
			ProjectDir:     plan.ProjectDir,
			ComposeFile:    plan.ComposeFile,
			EnvFile:        plan.EnvFile,
			ComposeProject: plan.ComposeProject,
			BackupRoot:     plan.BackupRoot,
			SiteURL:        plan.SiteURL,
		},
		Items: items,
	}

	if !plan.Ready() {
		res.Error = &result.ErrorInfo{
			Code:     "update_plan_blocked",
			Kind:     string(apperr.KindValidation),
			ExitCode: exitcode.ValidationError,
			Message:  message,
		}
	}

	return res
}

func renderUpdatePlanText(w io.Writer, res result.Result) error {
	details, ok := res.Details.(result.UpdatePlanDetails)
	if !ok {
		return result.Render(w, res, false)
	}

	artifacts, ok := res.Artifacts.(result.UpdatePlanArtifacts)
	if !ok {
		return fmt.Errorf("unexpected update-plan artifacts type %T", res.Artifacts)
	}

	if _, err := fmt.Fprintln(w, "EspoCRM update dry-run plan"); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "Scope:          %s\n", details.Scope); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "Project dir:    %s\n", artifacts.ProjectDir); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "Compose file:   %s\n", artifacts.ComposeFile); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "Env file:       %s\n", artifacts.EnvFile); err != nil {
		return err
	}
	if strings.TrimSpace(artifacts.ComposeProject) != "" {
		if _, err := fmt.Fprintf(w, "Compose name:   %s\n", artifacts.ComposeProject); err != nil {
			return err
		}
	}
	if strings.TrimSpace(artifacts.BackupRoot) != "" {
		if _, err := fmt.Fprintf(w, "Backup root:    %s\n", artifacts.BackupRoot); err != nil {
			return err
		}
	}
	if strings.TrimSpace(artifacts.SiteURL) != "" {
		if _, err := fmt.Fprintf(w, "Site URL:       %s\n", artifacts.SiteURL); err != nil {
			return err
		}
	}

	if _, err := fmt.Fprintln(w, "\nSummary:"); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "  Ready:        %t\n", details.Ready); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "  Steps:        %d\n", details.Steps); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "  Would run:    %d\n", details.WouldRun); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "  Skipped:      %d\n", details.Skipped); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "  Blocked:      %d\n", details.Blocked); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "  Unknown:      %d\n", details.Unknown); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "  Warnings:     %d\n", details.Warnings); err != nil {
		return err
	}

	if err := renderRecoveryAttemptSection(w, details.Recovery); err != nil {
		return err
	}

	if len(res.Warnings) != 0 {
		if _, err := fmt.Fprintln(w, "\nWarnings:"); err != nil {
			return err
		}
		for _, warning := range res.Warnings {
			if _, err := fmt.Fprintf(w, "- %s\n", warning); err != nil {
				return err
			}
		}
	}

	if _, err := fmt.Fprintln(w, "\nPlan:"); err != nil {
		return err
	}
	for _, rawItem := range res.Items {
		item, ok := rawItem.(result.UpdatePlanItem)
		if !ok {
			return fmt.Errorf("unexpected update-plan item type %T", rawItem)
		}
		label := strings.ToUpper(strings.ReplaceAll(item.Status, "_", " "))
		if _, err := fmt.Fprintf(w, "[%s] %s\n", label, item.Summary); err != nil {
			return err
		}
		if strings.TrimSpace(item.Details) != "" {
			if _, err := fmt.Fprintf(w, "  %s\n", item.Details); err != nil {
				return err
			}
		}
		if strings.TrimSpace(item.Action) != "" {
			if _, err := fmt.Fprintf(w, "  Action: %s\n", item.Action); err != nil {
				return err
			}
		}
	}

	return nil
}
