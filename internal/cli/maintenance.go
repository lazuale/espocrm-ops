package cli

import (
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/lazuale/espocrm-ops/internal/contract/exitcode"
	"github.com/lazuale/espocrm-ops/internal/contract/result"
	maintenanceusecase "github.com/lazuale/espocrm-ops/internal/usecase/maintenance"
	operationusecase "github.com/lazuale/espocrm-ops/internal/usecase/operation"
	"github.com/spf13/cobra"
)

func newMaintenanceCmd() *cobra.Command {
	var scope string
	var projectDir string
	var composeFile string
	var envFile string
	var apply bool
	var unattended bool
	var allowUnattendedApply bool
	var journalKeepDays int
	var journalKeepLast int
	var reportRetentionDays int
	var supportRetentionDays int
	var restoreDrillRetentionDays int

	cmd := &cobra.Command{
		Use:   "maintenance",
		Short: "Preview or apply canonical contour housekeeping and retention cleanup",
		Args:  noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			in := maintenanceInput{
				scope:                     scope,
				projectDir:                projectDir,
				composeFile:               composeFile,
				envFile:                   envFile,
				apply:                     apply,
				unattended:                unattended,
				allowUnattendedApply:      allowUnattendedApply,
				journalKeepDays:           journalKeepDays,
				journalKeepLast:           journalKeepLast,
				reportRetentionDays:       reportRetentionDays,
				supportRetentionDays:      supportRetentionDays,
				restoreDrillRetentionDays: restoreDrillRetentionDays,
			}
			if err := validateMaintenanceInput(cmd, &in); err != nil {
				return err
			}

			return runMaintenance(cmd, in)
		},
	}

	cmd.Flags().StringVar(&scope, "scope", "", "maintenance contour: dev or prod")
	cmd.Flags().StringVar(&projectDir, "project-dir", ".", "project directory containing the compose file and env files")
	cmd.Flags().StringVar(&composeFile, "compose-file", "", "compose file path (defaults to project-dir/compose.yaml)")
	cmd.Flags().StringVar(&envFile, "env-file", "", "override env file path")
	cmd.Flags().BoolVar(&apply, "apply", false, "perform cleanup instead of previewing it")
	cmd.Flags().BoolVar(&unattended, "unattended", false, "run with scheduler-safe non-interactive maintenance semantics")
	cmd.Flags().BoolVar(&allowUnattendedApply, "allow-unattended-apply", false, "allow destructive cleanup when --unattended and --apply are used together")
	cmd.Flags().IntVar(&journalKeepDays, "journal-keep-days", 30, "keep journal entries newer than N days")
	cmd.Flags().IntVar(&journalKeepLast, "journal-keep-last", 20, "always protect the most recent N journal entries")
	cmd.Flags().IntVar(&reportRetentionDays, "report-retention-days", -1, "override report retention days for this run")
	cmd.Flags().IntVar(&supportRetentionDays, "support-retention-days", -1, "override support bundle retention days for this run")
	cmd.Flags().IntVar(&restoreDrillRetentionDays, "restore-drill-retention-days", -1, "override restore-drill artifact retention days for this run")

	return cmd
}

type maintenanceInput struct {
	scope                     string
	projectDir                string
	composeFile               string
	envFile                   string
	apply                     bool
	unattended                bool
	allowUnattendedApply      bool
	journalKeepDays           int
	journalKeepLast           int
	reportRetentionDays       int
	supportRetentionDays      int
	restoreDrillRetentionDays int
	reportRetentionOverride   *int
	supportRetentionOverride  *int
	restoreDrillOverride      *int
}

func validateMaintenanceInput(cmd *cobra.Command, in *maintenanceInput) error {
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
	if in.journalKeepDays < 0 {
		return usageError(fmt.Errorf("--journal-keep-days must be non-negative"))
	}
	if in.journalKeepLast < 0 {
		return usageError(fmt.Errorf("--journal-keep-last must be non-negative"))
	}
	if in.journalKeepDays == 0 && in.journalKeepLast == 0 {
		return usageError(fmt.Errorf("maintenance requires --journal-keep-days or --journal-keep-last"))
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

	if in.reportRetentionOverride, err = optionalNonNegativeFlag(cmd, "report-retention-days", in.reportRetentionDays); err != nil {
		return err
	}
	if in.supportRetentionOverride, err = optionalNonNegativeFlag(cmd, "support-retention-days", in.supportRetentionDays); err != nil {
		return err
	}
	if in.restoreDrillOverride, err = optionalNonNegativeFlag(cmd, "restore-drill-retention-days", in.restoreDrillRetentionDays); err != nil {
		return err
	}
	if in.allowUnattendedApply && !in.unattended {
		return usageError(fmt.Errorf("--allow-unattended-apply requires --unattended"))
	}

	return nil
}

func optionalNonNegativeFlag(cmd *cobra.Command, flag string, value int) (*int, error) {
	if !cmd.Flags().Changed(flag) {
		return nil, nil
	}
	if value < 0 {
		return nil, usageError(fmt.Errorf("--%s must be non-negative", flag))
	}
	return &value, nil
}

func runMaintenance(cmd *cobra.Command, in maintenanceInput) error {
	spec := CommandSpec{
		Name:       "maintenance",
		ErrorCode:  "maintenance_failed",
		ExitCode:   exitcode.InternalError,
		RenderText: renderMaintenanceText,
	}
	exec := operationusecase.Begin(
		appForCommand(cmd).runtime,
		appForCommand(cmd).journalWriterFactory(appForCommand(cmd).options.JournalDir),
		spec.Name,
	)

	info, err := maintenanceusecase.Run(maintenanceusecase.Request{
		Scope:                     in.scope,
		ProjectDir:                in.projectDir,
		ComposeFile:               in.composeFile,
		EnvFileOverride:           in.envFile,
		EnvContourHint:            envFileContourHint(),
		JournalDir:                appForCommand(cmd).options.JournalDir,
		Now:                       appForCommand(cmd).runtime.Now(),
		Apply:                     in.apply,
		Unattended:                in.unattended,
		AllowUnattendedApply:      in.allowUnattendedApply,
		JournalKeepDays:           in.journalKeepDays,
		JournalKeepLast:           in.journalKeepLast,
		ReportRetentionDays:       in.reportRetentionOverride,
		SupportRetentionDays:      in.supportRetentionOverride,
		RestoreDrillRetentionDays: in.restoreDrillOverride,
	})

	res := maintenanceResult(info)
	if err != nil {
		return finishJournaledCommandFailure(cmd, spec, exec, res, err)
	}

	return finishJournaledCommandSuccess(cmd, spec, exec, res)
}

func maintenanceResult(info maintenanceusecase.Info) result.Result {
	ok := len(info.FailedSections) == 0
	message := maintenanceOutcomeMessage(info.Outcome)

	items := make([]any, 0, len(info.Sections))
	for _, section := range info.Sections {
		items = append(items, section)
	}

	return result.Result{
		Command:  "maintenance",
		OK:       ok,
		Message:  message,
		Warnings: append([]string(nil), info.Warnings...),
		DryRun:   info.DryRun,
		Details: result.MaintenanceDetails{
			Scope:          info.Scope,
			GeneratedAt:    info.GeneratedAt,
			Mode:           info.Mode,
			Unattended:     info.Unattended,
			Outcome:        info.Outcome,
			DryRun:         info.DryRun,
			CheckedItems:   info.CheckedItems,
			CandidateItems: info.CandidateItems,
			KeptItems:      info.KeptItems,
			ProtectedItems: info.ProtectedItems,
			RemovedItems:   info.RemovedItems,
			FailedItems:    info.FailedItems,
			SectionSummary: result.NewSectionSummary(len(info.Sections), len(info.Warnings), info.IncludedSections, info.OmittedSections, info.FailedSections),
		},
		Artifacts: result.MaintenanceArtifacts{
			ProjectDir:             info.ProjectDir,
			ComposeFile:            info.ComposeFile,
			EnvFile:                info.EnvFile,
			JournalDir:             info.JournalDir,
			BackupRoot:             info.BackupRoot,
			ReportsDir:             info.ReportsDir,
			SupportDir:             info.SupportDir,
			RestoreDrillEnvDir:     info.RestoreDrillEnvDir,
			RestoreDrillStorageDir: info.RestoreDrillStorageDir,
			RestoreDrillBackupDir:  info.RestoreDrillBackupDir,
		},
		Items: items,
	}
}

func renderMaintenanceText(w io.Writer, res result.Result) error {
	details, ok := res.Details.(result.MaintenanceDetails)
	if !ok {
		return result.Render(w, res, false)
	}
	artifacts, ok := res.Artifacts.(result.MaintenanceArtifacts)
	if !ok {
		return fmt.Errorf("unexpected maintenance artifacts type %T", res.Artifacts)
	}

	if _, err := fmt.Fprintln(w, "EspoCRM maintenance run"); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "Scope: %s\n", details.Scope); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "Mode: %s\n", details.Mode); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "Unattended: %s\n", yesNo(details.Unattended)); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "Outcome: %s\n", details.Outcome); err != nil {
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
	if _, err := fmt.Fprintf(w, "Journal dir: %s\n", valueOrNA(artifacts.JournalDir)); err != nil {
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
	if _, err := fmt.Fprintf(w, "Restore-drill env dir: %s\n", valueOrNA(artifacts.RestoreDrillEnvDir)); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "Restore-drill storage dir: %s\n", valueOrNA(artifacts.RestoreDrillStorageDir)); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "Restore-drill backup dir: %s\n", valueOrNA(artifacts.RestoreDrillBackupDir)); err != nil {
		return err
	}

	if err := renderOperatorSummaryBlock(w, details.SectionSummary, operatorSummaryRenderOptions{
		IncludeWarnings: true,
		ExtraLines: []operatorSummaryLine{
			{Label: "Checked items", Value: details.CheckedItems},
			{Label: "Candidate items", Value: details.CandidateItems},
			{Label: "Kept items", Value: details.KeptItems},
			{Label: "Protected items", Value: details.ProtectedItems},
			{Label: "Removed items", Value: details.RemovedItems},
			{Label: "Failed items", Value: details.FailedItems},
		},
	}); err != nil {
		return err
	}

	for _, raw := range res.Items {
		section, ok := raw.(maintenanceusecase.Section)
		if !ok {
			return fmt.Errorf("unexpected maintenance item type %T", raw)
		}

		if _, err := fmt.Fprintf(w, "\n%s:\n", maintenanceSectionTitle(section.Code)); err != nil {
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
		if err := renderMaintenanceSectionBody(w, section); err != nil {
			return err
		}
		if strings.TrimSpace(section.Action) != "" {
			if _, err := fmt.Fprintf(w, "  Action: %s\n", section.Action); err != nil {
				return err
			}
		}
		if strings.TrimSpace(section.FailureCode) != "" {
			if _, err := fmt.Fprintf(w, "  Failure code: %s\n", section.FailureCode); err != nil {
				return err
			}
		}
	}

	return nil
}

func renderMaintenanceSectionBody(w io.Writer, section maintenanceusecase.Section) error {
	if section.Context != nil {
		if err := renderMaintenanceContext(w, *section.Context); err != nil {
			return err
		}
	}
	if section.Cleanup != nil {
		if err := renderMaintenanceCleanup(w, *section.Cleanup); err != nil {
			return err
		}
	}
	return nil
}

func renderMaintenanceContext(w io.Writer, data maintenanceusecase.ContextData) error {
	if _, err := fmt.Fprintf(w, "  Contour: %s\n", valueOrNA(data.Contour)); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "  Mode: %s\n", valueOrNA(data.Mode)); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "  Unattended: %s\n", yesNo(data.Unattended)); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "  Compose project: %s\n", valueOrNA(data.ComposeProject)); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "  Env file: %s\n", valueOrNA(data.EnvFile)); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "  Backup root: %s\n", valueOrNA(data.BackupRoot)); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "  Reports dir: %s\n", valueOrNA(data.ReportsDir)); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "  Support dir: %s\n", valueOrNA(data.SupportDir)); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "  Restore-drill env dir: %s\n", valueOrNA(data.RestoreDrillEnvDir)); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "  Restore-drill storage dir: %s\n", valueOrNA(data.RestoreDrillStorageDir)); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "  Restore-drill backup dir: %s\n", valueOrNA(data.RestoreDrillBackupDir)); err != nil {
		return err
	}
	return nil
}

func renderMaintenanceCleanup(w io.Writer, data maintenanceusecase.CleanupData) error {
	if data.KeepDays != nil {
		if _, err := fmt.Fprintf(w, "  Keep days: %d\n", *data.KeepDays); err != nil {
			return err
		}
	}
	if data.KeepLast != nil {
		if _, err := fmt.Fprintf(w, "  Keep last: %d\n", *data.KeepLast); err != nil {
			return err
		}
	}
	if data.RetentionDays != nil {
		if _, err := fmt.Fprintf(w, "  Retention days: %d\n", *data.RetentionDays); err != nil {
			return err
		}
	}
	if data.TotalFilesSeen != 0 || data.LoadedEntries != 0 || data.SkippedCorrupt != 0 {
		if _, err := fmt.Fprintf(w, "  Journal read: seen=%d loaded=%d skipped_corrupt=%d\n", data.TotalFilesSeen, data.LoadedEntries, data.SkippedCorrupt); err != nil {
			return err
		}
	}
	if strings.TrimSpace(data.LatestOperationID) != "" {
		if _, err := fmt.Fprintf(w, "  Latest operation id: %s\n", data.LatestOperationID); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintf(w, "  Counts: checked=%d candidates=%d kept=%d protected=%d removed=%d failed=%d\n", data.Checked, data.Candidates, data.Kept, data.Protected, data.Removed, data.Failed); err != nil {
		return err
	}

	for _, item := range data.Items {
		if _, err := fmt.Fprintf(w, "  %s\n", formatMaintenanceItem(item, data.DryRun)); err != nil {
			return err
		}
	}

	return nil
}

func formatMaintenanceItem(item maintenanceusecase.CleanupItem, dryRun bool) string {
	decision := strings.ToUpper(item.Decision)
	if dryRun && item.Decision == "remove" {
		decision = "WOULD_REMOVE"
	}

	parts := []string{decision}
	if item.Operation != nil {
		parts = append(parts, formatHistoryLine(*item.Operation))
	} else if strings.TrimSpace(item.Kind) != "" {
		parts = append(parts, item.Kind)
	}
	if strings.TrimSpace(item.Path) != "" {
		parts = append(parts, "path="+item.Path)
	}
	if len(item.Reasons) != 0 {
		parts = append(parts, "reasons="+strings.Join(item.Reasons, ","))
	}
	if strings.TrimSpace(item.ModifiedAt) != "" {
		parts = append(parts, "modified_at="+item.ModifiedAt)
	}
	return strings.Join(parts, "  ")
}

func maintenanceSectionTitle(code string) string {
	switch code {
	case "context":
		return "Context"
	case "journal":
		return "Journal"
	case "reports":
		return "Reports"
	case "support":
		return "Support"
	case "restore_drill":
		return "Restore Drill"
	default:
		return strings.ReplaceAll(code, "_", " ")
	}
}

func maintenanceOutcomeMessage(outcome string) string {
	switch outcome {
	case "nothing_to_do":
		return "maintenance found nothing to do"
	case "preview_found_cleanup_candidates":
		return "maintenance preview found cleanup candidates"
	case "apply_removed_items":
		return "maintenance removed cleanup candidates"
	case "partial_failure":
		return "maintenance run partially failed"
	case "blocked":
		return "maintenance run blocked"
	default:
		return "maintenance run completed"
	}
}

func yesNo(value bool) string {
	if value {
		return "yes"
	}
	return "no"
}
