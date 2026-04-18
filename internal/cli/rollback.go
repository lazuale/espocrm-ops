package cli

import (
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/lazuale/espocrm-ops/internal/contract/apperr"
	"github.com/lazuale/espocrm-ops/internal/contract/exitcode"
	"github.com/lazuale/espocrm-ops/internal/contract/result"
	journalusecase "github.com/lazuale/espocrm-ops/internal/usecase/journal"
	operationusecase "github.com/lazuale/espocrm-ops/internal/usecase/operation"
	rollbackusecase "github.com/lazuale/espocrm-ops/internal/usecase/rollback"
	"github.com/spf13/cobra"
)

func newRollbackCmd() *cobra.Command {
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
	var dryRun bool
	var force bool
	var confirmProd string
	var recoverOperation string
	var recoverMode string

	cmd := &cobra.Command{
		Use:   "rollback",
		Short: "Run the canonical rollback flow",
		Args:  noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			in := rollbackInput{
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
				dryRun:         dryRun,
				force:          force,
				confirmProd:    confirmProd,
				recoverID:      recoverOperation,
				recoverMode:    recoverMode,
			}
			if err := validateRollbackInput(cmd, &in); err != nil {
				return err
			}

			if in.dryRun {
				return runRollbackDryRun(cmd, in)
			}

			return runRollbackExecute(cmd, in)
		},
	}

	cmd.Flags().StringVar(&scope, "scope", "", "rollback contour")
	cmd.Flags().StringVar(&projectDir, "project-dir", ".", "project directory containing the compose file and env files")
	cmd.Flags().StringVar(&composeFile, "compose-file", "", "compose file path (defaults to project-dir/compose.yaml)")
	cmd.Flags().StringVar(&envFile, "env-file", "", "override env file path")
	cmd.Flags().StringVar(&dbBackup, "db-backup", "", "explicit db backup path for rollback target selection")
	cmd.Flags().StringVar(&filesBackup, "files-backup", "", "explicit files backup path for rollback target selection")
	cmd.Flags().BoolVar(&noSnapshot, "no-snapshot", false, "skip the emergency recovery-point step")
	cmd.Flags().BoolVar(&noStart, "no-start", false, "leave the contour stopped after rollback")
	cmd.Flags().BoolVar(&skipHTTPProbe, "skip-http-probe", false, "skip the final HTTP probe after rollback")
	cmd.Flags().IntVar(&timeoutSeconds, "timeout", 600, "shared readiness timeout in seconds")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "show what rollback would do without making changes")
	cmd.Flags().BoolVar(&force, "force", false, "confirm that the destructive rollback should run")
	cmd.Flags().StringVar(&confirmProd, "confirm-prod", "", "confirm destructive prod rollback by passing the literal value `prod`")
	cmd.Flags().StringVar(&recoverOperation, "recover-operation", "", "recover a failed or blocked rollback operation by id")
	cmd.Flags().StringVar(&recoverMode, "recover-mode", journalusecase.RecoveryModeAuto, "recovery mode: auto, retry, or resume")

	return cmd
}

type rollbackInput struct {
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
	dryRun         bool
	force          bool
	confirmProd    string
	recoverID      string
	recoverMode    string
}

func validateRollbackInput(cmd *cobra.Command, in *rollbackInput) error {
	in.recoverID = strings.TrimSpace(in.recoverID)
	if cmd.Flags().Changed("recover-operation") && in.recoverID == "" {
		return requiredFlagError("--recover-operation")
	}
	in.confirmProd = strings.TrimSpace(in.confirmProd)
	if in.confirmProd != "" && in.confirmProd != "prod" {
		return usageError(fmt.Errorf("--confirm-prod accepts only the value prod"))
	}

	if in.recoverID != "" {
		if err := normalizeRecoveryModeFlag(cmd, "recover-mode", &in.recoverMode); err != nil {
			return err
		}
		if err := rejectRecoveryOverrides(cmd, "--recover-operation",
			"scope",
			"project-dir",
			"compose-file",
			"env-file",
			"db-backup",
			"files-backup",
			"no-snapshot",
			"no-start",
			"skip-http-probe",
			"timeout",
		); err != nil {
			return err
		}
		if in.dryRun {
			return nil
		}
		if !in.force {
			return usageError(fmt.Errorf("rollback recovery requires an explicit --force flag"))
		}
		return nil
	}

	planIn := rollbackPlanInput{
		scope:          in.scope,
		projectDir:     in.projectDir,
		composeFile:    in.composeFile,
		envFile:        in.envFile,
		dbBackup:       in.dbBackup,
		filesBackup:    in.filesBackup,
		noSnapshot:     in.noSnapshot,
		noStart:        in.noStart,
		skipHTTPProbe:  in.skipHTTPProbe,
		timeoutSeconds: in.timeoutSeconds,
	}
	if err := validateRollbackPlanInput(cmd, &planIn); err != nil {
		return err
	}

	in.scope = planIn.scope
	in.projectDir = planIn.projectDir
	in.composeFile = planIn.composeFile
	in.envFile = planIn.envFile
	in.dbBackup = planIn.dbBackup
	in.filesBackup = planIn.filesBackup
	in.timeoutSeconds = planIn.timeoutSeconds
	if in.dryRun {
		return nil
	}
	if !in.force {
		return usageError(fmt.Errorf("rollback requires an explicit --force flag"))
	}
	if in.scope == "prod" && in.confirmProd != "prod" {
		return usageError(fmt.Errorf("prod rollback also requires --confirm-prod prod"))
	}

	return nil
}

func runRollbackDryRun(cmd *cobra.Command, in rollbackInput) error {
	spec := CommandSpec{
		Name:       "rollback",
		ErrorCode:  "rollback_failed",
		ExitCode:   exitcode.InternalError,
		RenderText: renderRollbackPlanText,
	}
	exec := operationusecase.Begin(
		appForCommand(cmd).runtime,
		appForCommand(cmd).journalWriterFactory(appForCommand(cmd).options.JournalDir),
		spec.Name,
	)

	var (
		plan rollbackusecase.RollbackPlan
		err  error
	)
	if in.recoverID != "" {
		report, selection, recoveryErr := loadRollbackRecovery(cmd, in)
		if recoveryErr != nil {
			return commandFailure(cmd, appForCommand(cmd), spec, commandRunModeWithJournal, exec, result.Result{}, recoveryErr)
		}
		plan, err = rollbackusecase.RecoverPlan(report, selection)
	} else {
		plan, err = rollbackusecase.BuildPlan(rollbackusecase.PlanRequest{
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
	}
	if err != nil {
		return commandFailure(cmd, appForCommand(cmd), spec, commandRunModeWithJournal, exec, result.Result{}, err)
	}

	res := rollbackPlanResult(plan)
	res.Command = spec.Name

	if plan.Ready() {
		return finishJournaledCommandSuccess(cmd, spec, exec, res)
	}

	return finishJournaledCommandFailure(
		cmd,
		spec,
		exec,
		res,
		CodeError{
			Code:    exitcode.ValidationError,
			Err:     apperr.Wrap(apperr.KindValidation, "rollback_plan_blocked", errors.New("rollback dry-run plan found blocking conditions")),
			ErrCode: "rollback_plan_blocked",
		},
	)
}

func runRollbackExecute(cmd *cobra.Command, in rollbackInput) error {
	spec := CommandSpec{
		Name:       "rollback",
		ErrorCode:  "rollback_failed",
		ExitCode:   exitcode.InternalError,
		RenderText: renderRollbackText,
	}
	exec := operationusecase.Begin(
		appForCommand(cmd).runtime,
		appForCommand(cmd).journalWriterFactory(appForCommand(cmd).options.JournalDir),
		spec.Name,
	)

	var (
		info rollbackusecase.ExecuteInfo
		err  error
	)
	if in.recoverID != "" {
		report, selection, recoveryErr := loadRollbackRecovery(cmd, in)
		if recoveryErr != nil {
			return commandFailure(cmd, appForCommand(cmd), spec, commandRunModeWithJournal, exec, result.Result{}, recoveryErr)
		}
		info, err = rollbackusecase.RecoverExecute(report, selection, cmd.ErrOrStderr())
	} else {
		info, err = rollbackusecase.Execute(rollbackusecase.ExecuteRequest{
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
			LogWriter:       cmd.ErrOrStderr(),
		})
	}

	res := rollbackResult(info)
	res.Command = spec.Name

	if err != nil {
		return finishJournaledCommandFailure(cmd, spec, exec, res, err)
	}

	return finishJournaledCommandSuccess(cmd, spec, exec, res)
}

func loadRollbackRecovery(cmd *cobra.Command, in rollbackInput) (journalusecase.OperationReport, journalusecase.RecoverySelection, error) {
	app := appForCommand(cmd)

	operation, err := journalusecase.ShowOperation(journalusecase.ShowOperationInput{
		JournalDir: app.options.JournalDir,
		ID:         in.recoverID,
	})
	if err != nil {
		return journalusecase.OperationReport{}, journalusecase.RecoverySelection{}, err
	}

	report := journalusecase.Explain(operation.Entry)
	selection, err := journalusecase.ResolveRecoverySelection(report, in.recoverMode)
	if err != nil {
		return journalusecase.OperationReport{}, journalusecase.RecoverySelection{}, apperr.Wrap(apperr.KindValidation, "rollback_recovery_refused", err)
	}

	if !in.dryRun && report.Scope == "prod" && in.confirmProd != "prod" {
		return journalusecase.OperationReport{}, journalusecase.RecoverySelection{}, usageError(fmt.Errorf("prod rollback recovery also requires --confirm-prod prod"))
	}

	return report, selection, nil
}

func rollbackResult(info rollbackusecase.ExecuteInfo) result.Result {
	completed, skipped, failed, notRun := info.Counts()
	message := "rollback completed"
	if info.Recovery.Active() {
		message = "rollback recovery completed"
	}
	if !info.Ready() {
		message = "rollback failed"
		if info.Recovery.Active() {
			message = "rollback recovery failed"
		}
	}

	items := make([]any, 0, len(info.Steps))
	for _, step := range info.Steps {
		items = append(items, result.RollbackItem{
			Code:    step.Code,
			Status:  step.Status,
			Summary: step.Summary,
			Details: step.Details,
			Action:  step.Action,
		})
	}

	return result.Result{
		Command:  "rollback",
		OK:       info.Ready(),
		Message:  message,
		Warnings: append([]string(nil), info.Warnings...),
		Details: result.RollbackDetails{
			Scope:                  info.Scope,
			Ready:                  info.Ready(),
			SelectionMode:          info.SelectionMode,
			RequestedSelectionMode: info.RequestedSelectionMode,
			Steps:                  len(info.Steps),
			Completed:              completed,
			Skipped:                skipped,
			Failed:                 failed,
			NotRun:                 notRun,
			Warnings:               len(info.Warnings),
			TimeoutSeconds:         info.TimeoutSeconds,
			SnapshotEnabled:        info.SnapshotEnabled,
			NoStart:                info.NoStart,
			SkipHTTPProbe:          info.SkipHTTPProbe,
			StartedDBTemporarily:   info.StartedDBTemporarily,
			ServicesReady:          append([]string(nil), info.ServicesReady...),
			Recovery:               recoveryResultDetails(info.Recovery),
		},
		Artifacts: result.RollbackArtifacts{
			ProjectDir:            info.ProjectDir,
			ComposeFile:           info.ComposeFile,
			EnvFile:               info.EnvFile,
			ComposeProject:        info.ComposeProject,
			BackupRoot:            info.BackupRoot,
			SiteURL:               info.SiteURL,
			RequestedDBBackup:     info.RequestedDBBackup,
			RequestedFilesBackup:  info.RequestedFilesBackup,
			SelectedPrefix:        info.SelectedPrefix,
			SelectedStamp:         info.SelectedStamp,
			ManifestTXT:           info.ManifestTXTPath,
			ManifestJSON:          info.ManifestJSONPath,
			DBBackup:              info.DBBackupPath,
			FilesBackup:           info.FilesBackupPath,
			SnapshotManifestTXT:   info.SnapshotManifestTXT,
			SnapshotManifestJSON:  info.SnapshotManifestJSON,
			SnapshotDBBackup:      info.SnapshotDBBackup,
			SnapshotFilesBackup:   info.SnapshotFilesBackup,
			SnapshotDBChecksum:    info.SnapshotDBChecksum,
			SnapshotFilesChecksum: info.SnapshotFilesChecksum,
		},
		Items: items,
	}
}

func renderRollbackText(w io.Writer, res result.Result) error {
	details, ok := res.Details.(result.RollbackDetails)
	if !ok {
		return result.Render(w, res, false)
	}

	artifacts, ok := res.Artifacts.(result.RollbackArtifacts)
	if !ok {
		return fmt.Errorf("unexpected rollback artifacts type %T", res.Artifacts)
	}

	if _, err := fmt.Fprintln(w, "EspoCRM rollback"); err != nil {
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
	if _, err := fmt.Fprintf(w, "  Completed:    %d\n", details.Completed); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "  Skipped:      %d\n", details.Skipped); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "  Failed:       %d\n", details.Failed); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "  Not run:      %d\n", details.NotRun); err != nil {
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

	if _, err := fmt.Fprintln(w, "\nSteps:"); err != nil {
		return err
	}
	for _, rawItem := range res.Items {
		item, ok := rawItem.(result.RollbackItem)
		if !ok {
			return fmt.Errorf("unexpected rollback item type %T", rawItem)
		}
		if _, err := fmt.Fprintf(w, "[%s] %s\n", strings.ToUpper(item.Status), item.Summary); err != nil {
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

	if strings.TrimSpace(artifacts.SnapshotManifestJSON) != "" || strings.TrimSpace(artifacts.SnapshotDBBackup) != "" || strings.TrimSpace(artifacts.SnapshotFilesBackup) != "" {
		if _, err := fmt.Fprintln(w, "\nEmergency Snapshot:"); err != nil {
			return err
		}
		lines := []struct {
			label string
			value string
		}{
			{label: "Manifest txt", value: artifacts.SnapshotManifestTXT},
			{label: "Manifest json", value: artifacts.SnapshotManifestJSON},
			{label: "DB backup", value: artifacts.SnapshotDBBackup},
			{label: "Files backup", value: artifacts.SnapshotFilesBackup},
			{label: "DB checksum", value: artifacts.SnapshotDBChecksum},
			{label: "Files checksum", value: artifacts.SnapshotFilesChecksum},
		}
		for _, line := range lines {
			if strings.TrimSpace(line.value) == "" {
				continue
			}
			if _, err := fmt.Fprintf(w, "  %-17s %s\n", line.label+":", line.value); err != nil {
				return err
			}
		}
	}

	return nil
}
