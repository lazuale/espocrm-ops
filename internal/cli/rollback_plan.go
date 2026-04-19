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
	rollbackusecase "github.com/lazuale/espocrm-ops/internal/usecase/rollback"
	"github.com/spf13/cobra"
)

func newRollbackPlanCmd() *cobra.Command {
	var scope string
	var projectDir string
	var composeFile string
	var envFile string
	var dbBackup string
	var filesBackup string
	var noSnapshot bool
	var noStart bool
	var skipHTTPProbe bool
	var timeoutSeconds int

	cmd := &cobra.Command{
		Use:   "rollback-plan",
		Short: "Show what a rollback would do without making changes",
		Args:  noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			in := rollbackPlanInput{
				scope:          scope,
				projectDir:     projectDir,
				composeFile:    composeFile,
				envFile:        envFile,
				dbBackup:       dbBackup,
				filesBackup:    filesBackup,
				noSnapshot:     noSnapshot,
				noStart:        noStart,
				skipHTTPProbe:  skipHTTPProbe,
				timeoutSeconds: timeoutSeconds,
			}
			if err := validateRollbackPlanInput(cmd, &in); err != nil {
				return err
			}

			plan, err := rollbackusecase.BuildPlan(rollbackusecase.PlanRequest{
				Scope:           in.scope,
				ProjectDir:      in.projectDir,
				ComposeFile:     in.composeFile,
				EnvFileOverride: in.envFile,
				EnvContourHint:  envFileContourHint(),
				DBBackup:        in.dbBackup,
				FilesBackup:     in.filesBackup,
				NoSnapshot:      in.noSnapshot,
				NoStart:         in.noStart,
				SkipHTTPProbe:   in.skipHTTPProbe,
				TimeoutSeconds:  in.timeoutSeconds,
			})
			if err != nil {
				return err
			}

			res := rollbackPlanResult(plan)
			if plan.Ready() {
				return renderCommandResult(cmd, CommandSpec{
					Name:       "rollback-plan",
					RenderText: renderRollbackPlanText,
				}, res)
			}

			errCode := CodeError{
				Code:    exitcode.ValidationError,
				Err:     apperr.Wrap(apperr.KindValidation, "rollback_plan_blocked", errors.New("rollback dry-run plan found blocking conditions")),
				ErrCode: "rollback_plan_blocked",
			}

			if appForCommand(cmd).JSONEnabled() {
				return ResultCodeError{
					CodeError: errCode,
					Result:    res,
				}
			}

			if err := renderRollbackPlanText(cmd.OutOrStdout(), res); err != nil {
				return err
			}
			return silentCodeError{CodeError: errCode}
		},
	}

	cmd.Flags().StringVar(&scope, "scope", "", "rollback contour")
	cmd.Flags().StringVar(&projectDir, "project-dir", ".", "project directory containing the compose file and env files")
	cmd.Flags().StringVar(&composeFile, "compose-file", "", "compose file path (defaults to project-dir/compose.yaml)")
	cmd.Flags().StringVar(&envFile, "env-file", "", "override env file path")
	cmd.Flags().StringVar(&dbBackup, "db-backup", "", "explicit db backup path for rollback target selection")
	cmd.Flags().StringVar(&filesBackup, "files-backup", "", "explicit files backup path for rollback target selection")
	cmd.Flags().BoolVar(&noSnapshot, "no-snapshot", false, "skip the emergency recovery-point step in the planned flow")
	cmd.Flags().BoolVar(&noStart, "no-start", false, "leave the contour stopped in the planned flow")
	cmd.Flags().BoolVar(&skipHTTPProbe, "skip-http-probe", false, "skip the final HTTP probe in the planned flow")
	cmd.Flags().IntVar(&timeoutSeconds, "timeout", 600, "shared readiness timeout in seconds")

	return cmd
}

type rollbackPlanInput struct {
	scope          string
	projectDir     string
	composeFile    string
	envFile        string
	dbBackup       string
	filesBackup    string
	noSnapshot     bool
	noStart        bool
	skipHTTPProbe  bool
	timeoutSeconds int
}

func validateRollbackPlanInput(cmd *cobra.Command, in *rollbackPlanInput) error {
	in.scope = strings.TrimSpace(in.scope)
	in.projectDir = strings.TrimSpace(in.projectDir)
	in.composeFile = strings.TrimSpace(in.composeFile)
	in.envFile = strings.TrimSpace(in.envFile)
	in.dbBackup = strings.TrimSpace(in.dbBackup)
	in.filesBackup = strings.TrimSpace(in.filesBackup)

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

	if (in.dbBackup == "") != (in.filesBackup == "") {
		return usageError(fmt.Errorf("pass both --db-backup and --files-backup together"))
	}
	if in.dbBackup != "" {
		dbAbs, err := filepath.Abs(filepath.Clean(in.dbBackup))
		if err != nil {
			return usageError(fmt.Errorf("resolve --db-backup: %w", err))
		}
		in.dbBackup = dbAbs
		filesAbs, err := filepath.Abs(filepath.Clean(in.filesBackup))
		if err != nil {
			return usageError(fmt.Errorf("resolve --files-backup: %w", err))
		}
		in.filesBackup = filesAbs
	}

	if in.timeoutSeconds < 0 {
		return usageError(fmt.Errorf("--timeout must be non-negative"))
	}

	return nil
}

func rollbackPlanResult(plan rollbackusecase.RollbackPlan) result.Result {
	wouldRun, skipped, blocked, unknown := plan.Counts()

	message := "rollback dry-run plan completed"
	if plan.Recovery.Active() {
		message = "rollback recovery plan completed"
	}
	if !plan.Ready() {
		message = "rollback dry-run plan found blocking conditions"
		if plan.Recovery.Active() {
			message = "rollback recovery plan found blocking conditions"
		}
	}

	items := make([]any, 0, len(plan.Steps))
	for _, step := range plan.Steps {
		items = append(items, result.RollbackPlanItem{
			SectionItem: result.SectionItem{
				Code:    step.Code,
				Status:  step.Status,
				Summary: step.Summary,
				Details: step.Details,
				Action:  step.Action,
			},
		})
	}

	res := result.Result{
		Command:  "rollback-plan",
		OK:       plan.Ready(),
		Message:  message,
		DryRun:   true,
		Warnings: append([]string(nil), plan.Warnings...),
		Details: result.RollbackPlanDetails{
			Scope:           plan.Scope,
			Ready:           plan.Ready(),
			SelectionMode:   plan.SelectionMode,
			Steps:           len(plan.Steps),
			WouldRun:        wouldRun,
			Skipped:         skipped,
			Blocked:         blocked,
			Unknown:         unknown,
			Warnings:        len(plan.Warnings),
			TimeoutSeconds:  plan.TimeoutSeconds,
			SnapshotEnabled: plan.SnapshotEnabled,
			NoStart:         plan.NoStart,
			SkipHTTPProbe:   plan.SkipHTTPProbe,
			Recovery:        recoveryResultDetails(plan.Recovery),
		},
		Artifacts: result.RollbackPlanArtifacts{
			ProjectDir:     plan.ProjectDir,
			ComposeFile:    plan.ComposeFile,
			EnvFile:        plan.EnvFile,
			ComposeProject: plan.ComposeProject,
			BackupRoot:     plan.BackupRoot,
			SiteURL:        plan.SiteURL,
			SelectedPrefix: plan.SelectedPrefix,
			SelectedStamp:  plan.SelectedStamp,
			ManifestTXT:    plan.ManifestTXT,
			ManifestJSON:   plan.ManifestJSON,
			DBBackup:       plan.DBBackup,
			FilesBackup:    plan.FilesBackup,
		},
		Items: items,
	}

	if !plan.Ready() {
		res.Error = &result.ErrorInfo{
			Code:     "rollback_plan_blocked",
			Kind:     string(apperr.KindValidation),
			ExitCode: exitcode.ValidationError,
			Message:  message,
		}
	}

	return res
}

func renderRollbackPlanText(w io.Writer, res result.Result) error {
	details, ok := res.Details.(result.RollbackPlanDetails)
	if !ok {
		return result.Render(w, res, false)
	}

	artifacts, ok := res.Artifacts.(result.RollbackPlanArtifacts)
	if !ok {
		return fmt.Errorf("unexpected rollback-plan artifacts type %T", res.Artifacts)
	}

	if _, err := fmt.Fprintln(w, "EspoCRM rollback dry-run plan"); err != nil {
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
	if strings.TrimSpace(details.SelectionMode) != "" {
		if _, err := fmt.Fprintf(w, "Selection:      %s\n", details.SelectionMode); err != nil {
			return err
		}
	}
	if strings.TrimSpace(artifacts.SelectedPrefix) != "" {
		if _, err := fmt.Fprintf(w, "Target prefix:  %s\n", artifacts.SelectedPrefix); err != nil {
			return err
		}
	}
	if strings.TrimSpace(artifacts.SelectedStamp) != "" {
		if _, err := fmt.Fprintf(w, "Target stamp:   %s\n", artifacts.SelectedStamp); err != nil {
			return err
		}
	}
	if strings.TrimSpace(artifacts.ManifestJSON) != "" {
		if _, err := fmt.Fprintf(w, "Manifest JSON:  %s\n", artifacts.ManifestJSON); err != nil {
			return err
		}
	}
	if strings.TrimSpace(artifacts.DBBackup) != "" {
		if _, err := fmt.Fprintf(w, "DB backup:      %s\n", artifacts.DBBackup); err != nil {
			return err
		}
	}
	if strings.TrimSpace(artifacts.FilesBackup) != "" {
		if _, err := fmt.Fprintf(w, "Files backup:   %s\n", artifacts.FilesBackup); err != nil {
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
		item, ok := rawItem.(result.RollbackPlanItem)
		if !ok {
			return fmt.Errorf("unexpected rollback-plan item type %T", rawItem)
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
