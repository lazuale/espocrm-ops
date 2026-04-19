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
	healthsummaryusecase "github.com/lazuale/espocrm-ops/internal/usecase/healthsummary"
	"github.com/spf13/cobra"
)

func newHealthSummaryCmd() *cobra.Command {
	var scope string
	var projectDir string
	var composeFile string
	var envFile string
	var noVerifyChecksum bool
	var maxAgeHours int

	cmd := &cobra.Command{
		Use:   "health-summary",
		Short: "Show the canonical contour health verdict and alert summary",
		Args:  noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			in := healthSummaryInput{
				scope:          scope,
				projectDir:     projectDir,
				composeFile:    composeFile,
				envFile:        envFile,
				verifyChecksum: !noVerifyChecksum,
				maxAgeHours:    maxAgeHours,
			}
			if err := validateHealthSummaryInput(cmd, &in); err != nil {
				return err
			}

			info, err := healthsummaryusecase.Summarize(healthsummaryusecase.Request{
				Scope:           in.scope,
				ProjectDir:      in.projectDir,
				ComposeFile:     in.composeFile,
				EnvFileOverride: in.envFile,
				EnvContourHint:  envFileContourHint(),
				JournalDir:      appForCommand(cmd).options.JournalDir,
				VerifyChecksum:  in.verifyChecksum,
				MaxAgeHours:     in.maxAgeHours,
				Now:             appForCommand(cmd).runtime.Now(),
			})

			res := healthSummaryResult(info)
			if err == nil && info.Verdict != healthsummaryusecase.VerdictBlocked {
				return renderCommandResult(cmd, CommandSpec{
					Name:       "health-summary",
					RenderText: renderHealthSummaryText,
				}, res)
			}

			errCode := healthSummaryCodeError(err)
			if info.Verdict == healthsummaryusecase.VerdictBlocked {
				errCode = CodeError{
					Code:    exitcode.ValidationError,
					Err:     apperr.Wrap(apperr.KindValidation, "health_summary_blocked", errors.New("health summary reported blocking alerts")),
					ErrCode: "health_summary_blocked",
				}
			}

			if appForCommand(cmd).JSONEnabled() {
				return ResultCodeError{
					CodeError: errCode,
					Result:    res,
				}
			}

			if renderErr := renderHealthSummaryText(cmd.OutOrStdout(), res); renderErr != nil {
				return renderErr
			}
			if renderErr := renderWarnings(cmd.OutOrStdout(), res.Warnings); renderErr != nil {
				return renderErr
			}
			return silentCodeError{CodeError: errCode}
		},
	}

	cmd.Flags().StringVar(&scope, "scope", "", "health-summary contour: dev or prod")
	cmd.Flags().StringVar(&projectDir, "project-dir", ".", "project directory containing the compose file and env files")
	cmd.Flags().StringVar(&composeFile, "compose-file", "", "compose file path (defaults to project-dir/compose.yaml)")
	cmd.Flags().StringVar(&envFile, "env-file", "", "override env file path")
	cmd.Flags().BoolVar(&noVerifyChecksum, "no-verify-checksum", false, "skip checksum verification while evaluating backup posture")
	cmd.Flags().IntVar(&maxAgeHours, "max-age-hours", 48, "maximum allowed age in hours for the latest restore-ready backup set")

	return cmd
}

type healthSummaryInput struct {
	scope          string
	projectDir     string
	composeFile    string
	envFile        string
	verifyChecksum bool
	maxAgeHours    int
}

func validateHealthSummaryInput(cmd *cobra.Command, in *healthSummaryInput) error {
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
	if in.maxAgeHours < 0 {
		return usageError(fmt.Errorf("--max-age-hours must be non-negative"))
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

	return nil
}

func healthSummaryResult(info healthsummaryusecase.Info) result.Result {
	return result.Result{
		Command:  "health-summary",
		OK:       info.Verdict == healthsummaryusecase.VerdictHealthy || info.Verdict == healthsummaryusecase.VerdictDegraded,
		Message:  "health summary " + info.Verdict,
		Warnings: append([]string(nil), info.Warnings...),
		Details: result.HealthSummaryDetails{
			Scope:                info.Scope,
			GeneratedAt:          info.GeneratedAt,
			Verdict:              info.Verdict,
			NextAction:           info.NextAction,
			DoctorState:          info.DoctorState,
			RuntimeState:         info.RuntimeState,
			BackupState:          info.BackupState,
			LatestOperationState: info.LatestOperationState,
			LatestOperationID:    info.LatestOperationID,
			LatestOperationCmd:   info.LatestOperationCmd,
			MaintenanceState:     info.MaintenanceState,
			WarningAlerts:        healthSummaryAlertCount(info.Alerts, healthsummaryusecase.AlertSeverityWarning),
			BlockingAlerts:       healthSummaryAlertCount(info.Alerts, healthsummaryusecase.AlertSeverityBlocking),
			FailureAlerts:        healthSummaryAlertCount(info.Alerts, healthsummaryusecase.AlertSeverityFailure),
			SectionResults:       healthSummarySections(info.Sections),
			SectionSummary:       result.NewSectionSummary(len(info.Sections), len(info.Warnings), info.IncludedSections, info.OmittedSections, info.FailedSections),
		},
		Artifacts: result.HealthSummaryArtifacts{
			ProjectDir:  info.ProjectDir,
			ComposeFile: info.ComposeFile,
			EnvFile:     info.EnvFile,
			BackupRoot:  info.BackupRoot,
			ReportsDir:  info.ReportsDir,
			SupportDir:  info.SupportDir,
		},
		Items: healthSummaryAlertItems(info.Alerts),
	}
}

func renderHealthSummaryText(w io.Writer, res result.Result) error {
	details, ok := res.Details.(result.HealthSummaryDetails)
	if !ok {
		return result.Render(w, res, false)
	}
	artifacts, ok := res.Artifacts.(result.HealthSummaryArtifacts)
	if !ok {
		return fmt.Errorf("unexpected health-summary artifacts type %T", res.Artifacts)
	}

	if _, err := fmt.Fprintln(w, "EspoCRM health summary"); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "Scope: %s\n", details.Scope); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "Generated at: %s\n", details.GeneratedAt); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "Project dir: %s\n", artifacts.ProjectDir); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "Compose file: %s\n", artifacts.ComposeFile); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "Env file: %s\n", valueOrNA(artifacts.EnvFile)); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "Backup root: %s\n", valueOrNA(artifacts.BackupRoot)); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "Reports dir: %s\n", valueOrNA(artifacts.ReportsDir)); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "Support dir: %s\n", valueOrNA(artifacts.SupportDir)); err != nil {
		return err
	}

	extraLines := []operatorSummaryLine{
		{Label: "Verdict", Value: details.Verdict},
		{Label: "Doctor state", Value: details.DoctorState},
		{Label: "Runtime state", Value: details.RuntimeState},
		{Label: "Backup state", Value: details.BackupState},
		{Label: "Latest operation state", Value: details.LatestOperationState},
		{Label: "Latest operation", Value: valueOrNA(strings.TrimSpace(strings.Join([]string{details.LatestOperationCmd, details.LatestOperationID}, " ")))},
		{Label: "Maintenance state", Value: details.MaintenanceState},
		{Label: "Warning alerts", Value: details.WarningAlerts},
		{Label: "Blocking alerts", Value: details.BlockingAlerts},
		{Label: "Failure alerts", Value: details.FailureAlerts},
		{Label: "Next action", Value: valueOrNA(details.NextAction)},
	}
	if err := renderOperatorSummaryBlock(w, details.SectionSummary, operatorSummaryRenderOptions{
		IncludeWarnings: true,
		ExtraLines:      extraLines,
	}); err != nil {
		return err
	}

	if err := renderHealthSummarySections(w, details.SectionResults); err != nil {
		return err
	}
	if err := renderHealthSummaryAlerts(w, res.Items); err != nil {
		return err
	}

	return nil
}

func renderHealthSummarySections(w io.Writer, sections []result.HealthSummarySection) error {
	if len(sections) == 0 {
		return nil
	}

	if _, err := fmt.Fprintln(w, "\nSections:"); err != nil {
		return err
	}
	for _, section := range sections {
		if _, err := fmt.Fprintf(w, "[%s][%s] %s\n", upperStatusText(section.State), upperSpacedStatusText(section.Status), section.Summary); err != nil {
			return err
		}
		if strings.TrimSpace(section.SourceCommand) != "" {
			if _, err := fmt.Fprintf(w, "  Source: %s\n", section.SourceCommand); err != nil {
				return err
			}
		}
		if strings.TrimSpace(section.Details) != "" {
			if _, err := fmt.Fprintf(w, "  %s\n", section.Details); err != nil {
				return err
			}
		}
		if strings.TrimSpace(section.NextAction) != "" {
			if _, err := fmt.Fprintf(w, "  Action: %s\n", section.NextAction); err != nil {
				return err
			}
		}
	}

	return nil
}

func renderHealthSummaryAlerts(w io.Writer, items []any) error {
	if len(items) == 0 {
		return nil
	}

	if _, err := fmt.Fprintln(w, "\nAlerts:"); err != nil {
		return err
	}
	for _, raw := range items {
		alert, ok := raw.(healthsummaryusecase.Alert)
		if !ok {
			return fmt.Errorf("unexpected health-summary alert type %T", raw)
		}
		if _, err := fmt.Fprintf(w, "[%s][%s] %s\n", upperStatusText(alert.Severity), strings.ToUpper(strings.ReplaceAll(alert.Section, "_", " ")), alert.Summary); err != nil {
			return err
		}
		if strings.TrimSpace(alert.Cause) != "" {
			if _, err := fmt.Fprintf(w, "  Cause: %s\n", alert.Cause); err != nil {
				return err
			}
		}
		if strings.TrimSpace(alert.SourceCommand) != "" {
			if _, err := fmt.Fprintf(w, "  Source: %s\n", alert.SourceCommand); err != nil {
				return err
			}
		}
		if strings.TrimSpace(alert.NextAction) != "" {
			if _, err := fmt.Fprintf(w, "  Action: %s\n", alert.NextAction); err != nil {
				return err
			}
		}
	}

	return nil
}

func healthSummarySections(sections []healthsummaryusecase.Section) []result.HealthSummarySection {
	out := make([]result.HealthSummarySection, 0, len(sections))
	for _, section := range sections {
		out = append(out, result.HealthSummarySection{
			Code:          section.Code,
			Status:        section.Status,
			State:         section.State,
			SourceCommand: section.SourceCommand,
			Summary:       section.Summary,
			Details:       section.Details,
			CauseCode:     section.CauseCode,
			NextAction:    section.NextAction,
		})
	}
	return out
}

func healthSummaryAlertItems(alerts []healthsummaryusecase.Alert) []any {
	out := make([]any, 0, len(alerts))
	for _, alert := range alerts {
		out = append(out, alert)
	}
	return out
}

func healthSummaryAlertCount(alerts []healthsummaryusecase.Alert, severity string) int {
	count := 0
	for _, alert := range alerts {
		if alert.Severity == severity {
			count++
		}
	}
	return count
}

func healthSummaryFailureCode(err error) string {
	code, ok := apperr.CodeOf(err)
	if !ok {
		return "health_summary_failed"
	}
	return code
}

func healthSummaryCodeError(err error) CodeError {
	if err == nil {
		err = apperr.Wrap(apperr.KindValidation, "health_summary_failed", errors.New("health summary failed"))
	}
	return CodeError{
		Code:    codeForError(err, exitcode.ValidationError),
		Err:     err,
		ErrCode: healthSummaryFailureCode(err),
	}
}
