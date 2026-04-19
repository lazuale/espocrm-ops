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
	statusreportusecase "github.com/lazuale/espocrm-ops/internal/usecase/statusreport"
	"github.com/spf13/cobra"
)

func newStatusReportCmd() *cobra.Command {
	var scope string
	var projectDir string
	var composeFile string
	var envFile string

	cmd := &cobra.Command{
		Use:   "status-report",
		Short: "Show a canonical detailed contour runtime and artifact report",
		Args:  noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			in := statusReportInput{
				scope:       scope,
				projectDir:  projectDir,
				composeFile: composeFile,
				envFile:     envFile,
			}
			if err := validateStatusReportInput(cmd, &in); err != nil {
				return err
			}

			info, err := statusreportusecase.Summarize(statusreportusecase.Request{
				Scope:           in.scope,
				ProjectDir:      in.projectDir,
				ComposeFile:     in.composeFile,
				EnvFileOverride: in.envFile,
				EnvContourHint:  envFileContourHint(),
				JournalDir:      appForCommand(cmd).options.JournalDir,
				Now:             appForCommand(cmd).runtime.Now(),
			})

			res := statusReportResult(info)
			if err == nil {
				return renderCommandResult(cmd, CommandSpec{
					Name:       "status-report",
					RenderText: renderStatusReportText,
				}, res)
			}

			errCode := statusReportCodeError(err)

			if appForCommand(cmd).JSONEnabled() {
				return ResultCodeError{
					CodeError: errCode,
					Result:    res,
				}
			}

			if renderErr := renderStatusReportText(cmd.OutOrStdout(), res); renderErr != nil {
				return renderErr
			}
			if renderErr := renderWarnings(cmd.OutOrStdout(), res.Warnings); renderErr != nil {
				return renderErr
			}
			return silentCodeError{CodeError: errCode}
		},
	}

	cmd.Flags().StringVar(&scope, "scope", "", "status-report contour: dev or prod")
	cmd.Flags().StringVar(&projectDir, "project-dir", ".", "project directory containing the compose file and env files")
	cmd.Flags().StringVar(&composeFile, "compose-file", "", "compose file path (defaults to project-dir/compose.yaml)")
	cmd.Flags().StringVar(&envFile, "env-file", "", "override env file path")

	return cmd
}

type statusReportInput struct {
	scope       string
	projectDir  string
	composeFile string
	envFile     string
}

func validateStatusReportInput(cmd *cobra.Command, in *statusReportInput) error {
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

func statusReportResult(info statusreportusecase.Info) result.Result {
	ok := len(info.FailedSections) == 0
	message := "status report completed"
	if !ok {
		message = "status report found issues"
	}

	return result.Result{
		Command:  "status-report",
		OK:       ok,
		Message:  message,
		Warnings: append([]string(nil), info.Warnings...),
		Details: result.StatusReportDetails{
			Scope:            info.Scope,
			GeneratedAt:      info.GeneratedAt,
			Sections:         len(info.Sections),
			Included:         len(info.IncludedSections),
			Omitted:          len(info.OmittedSections),
			Failed:           len(info.FailedSections),
			Warnings:         len(info.Warnings),
			IncludedSections: append([]string(nil), info.IncludedSections...),
			OmittedSections:  append([]string(nil), info.OmittedSections...),
			FailedSections:   append([]string(nil), info.FailedSections...),
		},
		Artifacts: result.StatusReportArtifacts{
			ProjectDir:  info.ProjectDir,
			ComposeFile: info.ComposeFile,
			EnvFile:     info.EnvFile,
			BackupRoot:  info.BackupRoot,
			ReportsDir:  info.ReportsDir,
			SupportDir:  info.SupportDir,
		},
		Items: statusReportSectionItems(info.Sections),
	}
}

func statusReportSectionItems(sections []statusreportusecase.Section) []any {
	items := make([]any, 0, len(sections))
	for _, section := range sections {
		items = append(items, section)
	}
	return items
}

func renderStatusReportText(w io.Writer, res result.Result) error {
	details, ok := res.Details.(result.StatusReportDetails)
	if !ok {
		return result.Render(w, res, false)
	}
	artifacts, ok := res.Artifacts.(result.StatusReportArtifacts)
	if !ok {
		return fmt.Errorf("unexpected status-report artifacts type %T", res.Artifacts)
	}

	if _, err := fmt.Fprintln(w, "EspoCRM status report"); err != nil {
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

	if _, err := fmt.Fprintln(w, "\nSummary:"); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "  Included: %d\n", details.Included); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "  Omitted: %d\n", details.Omitted); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "  Failed: %d\n", details.Failed); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "  Included sections: %s\n", sectionListText(details.IncludedSections)); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "  Omitted sections: %s\n", sectionListText(details.OmittedSections)); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "  Failed sections: %s\n", sectionListText(details.FailedSections)); err != nil {
		return err
	}

	for _, raw := range res.Items {
		section, ok := raw.(statusreportusecase.Section)
		if !ok {
			return fmt.Errorf("unexpected status-report item type %T", raw)
		}

		if _, err := fmt.Fprintf(w, "\n%s:\n", statusReportSectionTitle(section.Code)); err != nil {
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
		if err := renderStatusReportSectionBody(w, section); err != nil {
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

func renderStatusReportSectionBody(w io.Writer, section statusreportusecase.Section) error {
	switch {
	case section.Context != nil:
		return renderStatusReportContextSection(w, *section.Context)
	case section.Doctor != nil:
		return renderStatusReportDoctorSection(w, *section.Doctor)
	case section.Runtime != nil:
		return renderStatusReportRuntimeSection(w, *section.Runtime)
	case section.LatestOperation != nil:
		return renderStatusReportLatestOperationSection(w, *section.LatestOperation)
	case section.Artifacts != nil:
		return renderStatusReportArtifactsSection(w, *section.Artifacts)
	default:
		return nil
	}
}

func renderStatusReportContextSection(w io.Writer, data statusreportusecase.ContextData) error {
	if _, err := fmt.Fprintf(w, "  Contour: %s\n", valueOrNA(data.Contour)); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "  Compose project: %s\n", valueOrNA(data.ComposeProject)); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "  Site URL: %s\n", valueOrNA(data.SiteURL)); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "  WebSocket URL: %s\n", valueOrNA(data.WSPublicURL)); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "  EspoCRM image: %s\n", valueOrNA(data.EspoCRMImage)); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "  Backup retention days: %s\n", valueOrNA(optionalIntText(data.Retention.BackupDays))); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "  Report retention days: %d\n", data.Retention.ReportDays); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "  Support retention days: %d\n", data.Retention.SupportDays); err != nil {
		return err
	}
	for _, line := range []struct {
		label string
		path  statusreportusecase.PathData
	}{
		{label: "DB storage", path: data.Storage.DB},
		{label: "EspoCRM storage", path: data.Storage.Espo},
		{label: "Backup root", path: data.Storage.BackupRoot},
		{label: "Reports directory", path: data.Storage.ReportsDir},
		{label: "Support directory", path: data.Storage.SupportDir},
	} {
		state := "missing"
		if line.path.Exists {
			state = "exists"
		}
		if _, err := fmt.Fprintf(w, "  %s: %s | %s | %s\n", line.label, line.path.Path, state, line.path.SizeHuman); err != nil {
			return err
		}
	}
	return nil
}

func renderStatusReportDoctorSection(w io.Writer, data statusreportusecase.DoctorData) error {
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
		if _, err := fmt.Fprintf(w, "  Warning: %s\n", statusReportDoctorCheckText(check)); err != nil {
			return err
		}
	}
	for _, check := range data.FailedChecks {
		if _, err := fmt.Fprintf(w, "  Failure: %s\n", statusReportDoctorCheckText(check)); err != nil {
			return err
		}
	}
	return nil
}

func renderStatusReportRuntimeSection(w io.Writer, data statusreportusecase.RuntimeData) error {
	if _, err := fmt.Fprintf(w, "  Compose project: %s\n", valueOrNA(data.ComposeProject)); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "  Site URL: %s\n", valueOrNA(data.SiteURL)); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "  WebSocket URL: %s\n", valueOrNA(data.WSPublicURL)); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "  Shared operation lock: %s\n", statusReportLockText(data.SharedOperationLock)); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "  Maintenance lock: %s\n", statusReportLockText(data.MaintenanceLock)); err != nil {
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

func renderStatusReportLatestOperationSection(w io.Writer, data statusreportusecase.LatestOperationData) error {
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

func renderStatusReportArtifactsSection(w io.Writer, data statusreportusecase.ArtifactsData) error {
	if _, err := fmt.Fprintf(w, "  Checksum verification: %s\n", enabledText(data.VerifyChecksum)); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "  Total backup sets: %d\n", data.TotalBackupSets); err != nil {
		return err
	}
	if data.LatestReadyBackup != nil {
		if _, err := fmt.Fprintf(w, "  Latest ready backup: %s\n", data.LatestReadyBackup.ID); err != nil {
			return err
		}
		if _, err := fmt.Fprintf(w, "  Latest ready origin: %s\n", valueOrNA(data.LatestReadyBackup.Origin.Label)); err != nil {
			return err
		}
		if _, err := fmt.Fprintf(w, "  Latest ready readiness: %s\n", readinessText(data.LatestReadyBackup.RestoreReadiness)); err != nil {
			return err
		}
	}
	for _, line := range []struct {
		label string
		file  *statusreportusecase.ArtifactFile
	}{
		{label: "Latest DB backup", file: data.LatestDBBackup},
		{label: "Latest files backup", file: data.LatestFilesBackup},
		{label: "Latest JSON manifest", file: data.LatestManifestJSON},
		{label: "Latest text manifest", file: data.LatestManifestTXT},
		{label: "Latest text report", file: data.LatestReportTXT},
		{label: "Latest JSON report", file: data.LatestReportJSON},
		{label: "Latest support bundle", file: data.LatestSupportBundle},
	} {
		if _, err := fmt.Fprintf(w, "  %s: %s\n", line.label, artifactFileText(line.file)); err != nil {
			return err
		}
	}
	return nil
}

func artifactFileText(file *statusreportusecase.ArtifactFile) string {
	if file == nil {
		return "n/a"
	}
	return fmt.Sprintf("%s | %d bytes | %s", file.Path, file.SizeBytes, file.ModifiedAt)
}

func optionalIntText(value *int) string {
	if value == nil {
		return ""
	}
	return fmt.Sprintf("%d", *value)
}

func statusReportSectionTitle(code string) string {
	switch code {
	case "context":
		return "Context"
	case "doctor":
		return "Doctor"
	case "runtime":
		return "Runtime"
	case "latest_operation":
		return "Latest Operation"
	case "artifacts":
		return "Artifacts"
	default:
		return strings.ReplaceAll(code, "_", " ")
	}
}

func statusReportLockText(lock statusreportusecase.LockSnapshot) string {
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

func statusReportDoctorCheckText(check doctorusecase.Check) string {
	if strings.TrimSpace(check.Scope) == "" {
		return check.Summary
	}
	return check.Scope + ": " + check.Summary
}

func statusReportFailureCode(err error) string {
	code, ok := apperr.CodeOf(err)
	if !ok {
		return "status_report_failed"
	}
	return code
}

func statusReportCodeError(err error) CodeError {
	return CodeError{
		Code:    codeForError(err, exitcode.ValidationError),
		Err:     err,
		ErrCode: statusReportFailureCode(err),
	}
}
