package cli

import (
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/lazuale/espocrm-ops/internal/contract/apperr"
	"github.com/lazuale/espocrm-ops/internal/contract/exitcode"
	"github.com/lazuale/espocrm-ops/internal/contract/result"
	doctorusecase "github.com/lazuale/espocrm-ops/internal/usecase/doctor"
	overviewusecase "github.com/lazuale/espocrm-ops/internal/usecase/overview"
	"github.com/spf13/cobra"
)

func newOverviewCmd() *cobra.Command {
	var scope string
	var projectDir string
	var composeFile string
	var envFile string

	cmd := &cobra.Command{
		Use:   "overview",
		Short: "Show the canonical operator dashboard and contour summary",
		Args:  noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			in := overviewInput{
				scope:       scope,
				projectDir:  projectDir,
				composeFile: composeFile,
				envFile:     envFile,
			}
			if err := validateOverviewInput(cmd, &in); err != nil {
				return err
			}

			info, err := overviewusecase.Summarize(overviewusecase.Request{
				Scope:           in.scope,
				ProjectDir:      in.projectDir,
				ComposeFile:     in.composeFile,
				EnvFileOverride: in.envFile,
				EnvContourHint:  envFileContourHint(),
				JournalDir:      appForCommand(cmd).options.JournalDir,
				Now:             appForCommand(cmd).runtime.Now(),
			})

			res := overviewResult(info)
			if err == nil {
				return renderCommandResult(cmd, CommandSpec{
					Name:       "overview",
					RenderText: renderOverviewText,
				}, res)
			}

			errCode := overviewCodeError(err)

			if appForCommand(cmd).JSONEnabled() {
				return ResultCodeError{
					CodeError: errCode,
					Result:    res,
				}
			}

			if renderErr := renderOverviewText(cmd.OutOrStdout(), res); renderErr != nil {
				return renderErr
			}
			if renderErr := renderWarnings(cmd.OutOrStdout(), res.Warnings); renderErr != nil {
				return renderErr
			}
			return silentCodeError{CodeError: errCode}
		},
	}

	cmd.Flags().StringVar(&scope, "scope", "", "overview contour: dev or prod")
	cmd.Flags().StringVar(&projectDir, "project-dir", ".", "project directory containing the compose file and env files")
	cmd.Flags().StringVar(&composeFile, "compose-file", "", "compose file path (defaults to project-dir/compose.yaml)")
	cmd.Flags().StringVar(&envFile, "env-file", "", "override env file path")

	return cmd
}

type overviewInput struct {
	scope       string
	projectDir  string
	composeFile string
	envFile     string
}

func validateOverviewInput(cmd *cobra.Command, in *overviewInput) error {
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

	return nil
}

func overviewResult(info overviewusecase.Info) result.Result {
	ok := len(info.FailedSections) == 0
	message := "operator dashboard completed"
	if !ok {
		message = "operator dashboard found issues"
	}

	return result.Result{
		Command:  "overview",
		OK:       ok,
		Message:  message,
		Warnings: append([]string(nil), info.Warnings...),
		Details: result.OverviewDetails{
			Scope:          info.Scope,
			GeneratedAt:    info.GeneratedAt,
			SectionSummary: result.NewSectionSummary(len(info.Sections), len(info.Warnings), info.IncludedSections, info.OmittedSections, info.FailedSections),
		},
		Artifacts: result.OverviewArtifacts{
			ProjectDir:  info.ProjectDir,
			ComposeFile: info.ComposeFile,
			EnvFile:     info.EnvFile,
			BackupRoot:  info.BackupRoot,
		},
		Items: overviewSectionItems(info.Sections),
	}
}

func overviewSectionItems(sections []overviewusecase.Section) []any {
	items := make([]any, 0, len(sections))
	for _, section := range sections {
		items = append(items, section)
	}
	return items
}

func renderOverviewText(w io.Writer, res result.Result) error {
	details, ok := res.Details.(result.OverviewDetails)
	if !ok {
		return result.Render(w, res, false)
	}
	artifacts, ok := res.Artifacts.(result.OverviewArtifacts)
	if !ok {
		return fmt.Errorf("unexpected overview artifacts type %T", res.Artifacts)
	}

	if _, err := fmt.Fprintln(w, "EspoCRM operator dashboard"); err != nil {
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

	if err := renderOperatorSummaryBlock(w, details.SectionSummary, operatorSummaryRenderOptions{
		IncludeWarnings: true,
	}); err != nil {
		return err
	}

	for _, raw := range res.Items {
		section, ok := raw.(overviewusecase.Section)
		if !ok {
			return fmt.Errorf("unexpected overview item type %T", raw)
		}

		if _, err := fmt.Fprintf(w, "\n%s:\n", overviewSectionTitle(section.Code)); err != nil {
			return err
		}
		if _, err := fmt.Fprintf(w, "  Status: %s\n", section.Status); err != nil {
			return err
		}
		if _, err := fmt.Fprintf(w, "  Summary: %s\n", section.Summary); err != nil {
			return err
		}
		if strings.TrimSpace(section.Details) != "" {
			if _, err := fmt.Fprintf(w, "  Details: %s\n", section.Details); err != nil {
				return err
			}
		}
		if err := renderOverviewSectionBody(w, section); err != nil {
			return err
		}
		if strings.TrimSpace(section.Action) != "" {
			if _, err := fmt.Fprintf(w, "  Action: %s\n", section.Action); err != nil {
				return err
			}
		}
	}

	return nil
}

func renderOverviewSectionBody(w io.Writer, section overviewusecase.Section) error {
	switch {
	case section.Context != nil:
		return renderOverviewContextSection(w, *section.Context)
	case section.Doctor != nil:
		return renderOverviewDoctorSection(w, *section.Doctor)
	case section.Runtime != nil:
		return renderOverviewRuntimeSection(w, *section.Runtime)
	case section.LatestOperation != nil:
		return renderOverviewLatestOperationSection(w, *section.LatestOperation)
	case section.Backup != nil:
		return renderOverviewBackupSection(w, *section.Backup)
	default:
		return nil
	}
}

func renderOverviewContextSection(w io.Writer, data overviewusecase.ContextData) error {
	if _, err := fmt.Fprintf(w, "  Contour: %s\n", valueOrNA(data.Contour)); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "  Compose project: %s\n", valueOrNA(data.ComposeProject)); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "  Env file: %s\n", valueOrNA(data.EnvFile)); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "  Site URL: %s\n", valueOrNA(data.SiteURL)); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "  WebSocket URL: %s\n", valueOrNA(data.WSPublicURL)); err != nil {
		return err
	}
	return nil
}

func renderOverviewDoctorSection(w io.Writer, data overviewusecase.DoctorData) error {
	if _, err := fmt.Fprintf(w, "  Ready: %t\n", data.Ready); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "  Checks: %d\n", data.Checks); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "  Passed: %d\n", data.Passed); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "  Warnings: %d\n", data.Warnings); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "  Failed: %d\n", data.Failed); err != nil {
		return err
	}
	for _, check := range data.WarningChecks {
		if _, err := fmt.Fprintf(w, "  Warning: %s\n", overviewDoctorCheckText(check)); err != nil {
			return err
		}
	}
	for _, check := range data.FailedChecks {
		if _, err := fmt.Fprintf(w, "  Failure: %s\n", overviewDoctorCheckText(check)); err != nil {
			return err
		}
	}
	return nil
}

func renderOverviewRuntimeSection(w io.Writer, data overviewusecase.RuntimeData) error {
	if _, err := fmt.Fprintf(w, "  Compose project: %s\n", valueOrNA(data.ComposeProject)); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "  Site URL: %s\n", valueOrNA(data.SiteURL)); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "  WebSocket URL: %s\n", valueOrNA(data.WSPublicURL)); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "  Shared operation lock: %s\n", overviewLockText(data.SharedOperationLock)); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "  Maintenance lock: %s\n", overviewLockText(data.MaintenanceLock)); err != nil {
		return err
	}
	for _, service := range data.Services {
		line := fmt.Sprintf("  Service %s: %s", service.Name, service.Status)
		if strings.TrimSpace(service.Details) != "" {
			line += " | " + service.Details
		}
		if _, err := fmt.Fprintln(w, line); err != nil {
			return err
		}
	}
	return nil
}

func renderOverviewBackupSection(w io.Writer, data overviewusecase.BackupData) error {
	if _, err := fmt.Fprintf(w, "  Checksum verification: %s\n", enabledText(data.VerifyChecksum)); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "  Audit success: %t\n", data.AuditSuccess); err != nil {
		return err
	}
	selected := valueOrNA(strings.Trim(strings.Join([]string{data.SelectedPrefix, data.SelectedStamp}, " | "), " |"))
	if _, err := fmt.Fprintf(w, "  Selected set: %s\n", selected); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "  Total sets: %d\n", data.TotalSets); err != nil {
		return err
	}
	if data.LatestReady != nil {
		if _, err := fmt.Fprintf(w, "  Latest ready backup: %s\n", data.LatestReady.ID); err != nil {
			return err
		}
		if _, err := fmt.Fprintf(w, "  Latest ready origin: %s\n", valueOrNA(data.LatestReady.Origin.Label)); err != nil {
			return err
		}
		if _, err := fmt.Fprintf(w, "  Latest ready readiness: %s\n", readinessText(data.LatestReady.RestoreReadiness)); err != nil {
			return err
		}
		if _, err := fmt.Fprintf(w, "  Latest ready DB: %s\n", valueOrMissing(data.LatestReady.DB.File)); err != nil {
			return err
		}
		if _, err := fmt.Fprintf(w, "  Latest ready files: %s\n", valueOrMissing(data.LatestReady.Files.File)); err != nil {
			return err
		}
	}
	for _, finding := range data.WarningFindings {
		if _, err := fmt.Fprintf(w, "  Warning: %s: %s\n", finding.Subject, finding.Message); err != nil {
			return err
		}
	}
	for _, finding := range data.FailureFindings {
		if _, err := fmt.Fprintf(w, "  Failure: %s: %s\n", finding.Subject, finding.Message); err != nil {
			return err
		}
	}
	return nil
}

func renderOverviewLatestOperationSection(w io.Writer, data overviewusecase.LatestOperationData) error {
	if _, err := fmt.Fprintf(w, "  Returned: %d\n", data.Returned); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "  Journal files seen: %d\n", data.TotalFilesSeen); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "  Loaded entries: %d\n", data.LoadedEntries); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "  Skipped corrupt: %d\n", data.SkippedCorrupt); err != nil {
		return err
	}
	if data.Operation == nil {
		return nil
	}
	if _, err := fmt.Fprintf(w, "  %s\n", formatHistoryLine(*data.Operation)); err != nil {
		return err
	}
	return nil
}

func overviewSectionTitle(code string) string {
	switch code {
	case "context":
		return "Context"
	case "doctor":
		return "Doctor"
	case "runtime":
		return "Runtime"
	case "latest_operation":
		return "Latest Operation"
	case "backup":
		return "Backup"
	default:
		return strings.ReplaceAll(code, "_", " ")
	}
}

func sectionListText(items []string) string {
	if len(items) == 0 {
		return "none"
	}
	return strings.Join(items, ", ")
}

func overviewLockText(lock overviewusecase.LockSnapshot) string {
	parts := []string{lock.State}
	if strings.TrimSpace(lock.PID) != "" {
		parts = append(parts, "pid="+lock.PID)
	}
	if strings.TrimSpace(lock.MetadataPath) != "" {
		parts = append(parts, "path="+lock.MetadataPath)
	}
	if strings.TrimSpace(lock.Error) != "" {
		parts = append(parts, "error="+lock.Error)
	}
	if len(lock.StalePaths) != 0 {
		parts = append(parts, "stale="+strings.Join(lock.StalePaths, ", "))
	}
	return strings.Join(parts, " | ")
}

func overviewDoctorCheckText(check doctorusecase.Check) string {
	if strings.TrimSpace(check.Scope) == "" {
		return check.Summary
	}
	return check.Scope + ": " + check.Summary
}

func overviewFailureCode(err error) string {
	code, ok := apperr.CodeOf(err)
	if !ok {
		return "overview_failed"
	}
	return code
}

func overviewCodeError(err error) CodeError {
	return CodeError{
		Code:    codeForError(err, exitcode.ValidationError),
		Err:     err,
		ErrCode: overviewFailureCode(err),
	}
}
