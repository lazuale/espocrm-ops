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
	updateusecase "github.com/lazuale/espocrm-ops/internal/usecase/update"
	"github.com/spf13/cobra"
)

func newUpdateCmd() *cobra.Command {
	var scope string
	var projectDir string
	var composeFile string
	var envFile string
	var timeoutSeconds int
	var skipDoctor bool
	var skipBackup bool
	var skipPull bool
	var skipHTTPProbe bool
	var dryRun bool
	var recoverOperation string
	var recoverMode string

	cmd := &cobra.Command{
		Use:   "update",
		Short: "Run the canonical update flow",
		Args:  noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			in := updateInput{
				scope:          scope,
				projectDir:     projectDir,
				composeFile:    composeFile,
				envFile:        envFile,
				timeoutSeconds: timeoutSeconds,
				skipDoctor:     skipDoctor,
				skipBackup:     skipBackup,
				skipPull:       skipPull,
				skipHTTPProbe:  skipHTTPProbe,
				dryRun:         dryRun,
				recoverID:      recoverOperation,
				recoverMode:    recoverMode,
			}
			if err := validateUpdateInput(cmd, &in); err != nil {
				return err
			}

			if in.dryRun {
				return runUpdateDryRun(cmd, in)
			}

			return runUpdateExecute(cmd, in)
		},
	}

	cmd.Flags().StringVar(&scope, "scope", "", "update contour")
	cmd.Flags().StringVar(&projectDir, "project-dir", ".", "project directory containing the compose file and env files")
	cmd.Flags().StringVar(&composeFile, "compose-file", "", "compose file path (defaults to project-dir/compose.yaml)")
	cmd.Flags().StringVar(&envFile, "env-file", "", "override env file path")
	cmd.Flags().IntVar(&timeoutSeconds, "timeout", 600, "shared readiness timeout in seconds")
	cmd.Flags().BoolVar(&skipDoctor, "skip-doctor", false, "skip the doctor step in the update flow")
	cmd.Flags().BoolVar(&skipBackup, "skip-backup", false, "skip the recovery-point step in the update flow")
	cmd.Flags().BoolVar(&skipPull, "skip-pull", false, "skip image pull in the runtime apply step")
	cmd.Flags().BoolVar(&skipHTTPProbe, "skip-http-probe", false, "skip the final HTTP probe in the runtime readiness step")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "show what update would do without making changes")
	cmd.Flags().StringVar(&recoverOperation, "recover-operation", "", "recover a failed or blocked update operation by id")
	cmd.Flags().StringVar(&recoverMode, "recover-mode", journalusecase.RecoveryModeAuto, "recovery mode: auto, retry, or resume")

	return cmd
}

type updateInput struct {
	scope          string
	projectDir     string
	composeFile    string
	envFile        string
	timeoutSeconds int
	skipDoctor     bool
	skipBackup     bool
	skipPull       bool
	skipHTTPProbe  bool
	dryRun         bool
	recoverID      string
	recoverMode    string
}

func validateUpdateInput(cmd *cobra.Command, in *updateInput) error {
	in.recoverID = strings.TrimSpace(in.recoverID)
	if cmd.Flags().Changed("recover-operation") && in.recoverID == "" {
		return requiredFlagError("--recover-operation")
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
			"timeout",
			"skip-doctor",
			"skip-backup",
			"skip-pull",
			"skip-http-probe",
		); err != nil {
			return err
		}
		return nil
	}

	planIn := updatePlanInput{
		scope:          in.scope,
		projectDir:     in.projectDir,
		composeFile:    in.composeFile,
		envFile:        in.envFile,
		timeoutSeconds: in.timeoutSeconds,
		skipDoctor:     in.skipDoctor,
		skipBackup:     in.skipBackup,
		skipPull:       in.skipPull,
		skipHTTPProbe:  in.skipHTTPProbe,
	}
	if err := validateUpdatePlanInput(cmd, &planIn); err != nil {
		return err
	}

	in.scope = planIn.scope
	in.projectDir = planIn.projectDir
	in.composeFile = planIn.composeFile
	in.envFile = planIn.envFile
	in.timeoutSeconds = planIn.timeoutSeconds

	return nil
}

func runUpdateDryRun(cmd *cobra.Command, in updateInput) error {
	spec := CommandSpec{
		Name:       "update",
		ErrorCode:  "update_failed",
		ExitCode:   exitcode.InternalError,
		RenderText: renderUpdatePlanText,
	}
	exec := operationusecase.Begin(
		appForCommand(cmd).runtime,
		appForCommand(cmd).journalWriterFactory(appForCommand(cmd).options.JournalDir),
		spec.Name,
	)

	var (
		plan updateusecase.UpdatePlan
		err  error
	)
	if in.recoverID != "" {
		report, selection, err := loadUpdateRecovery(cmd, in)
		if err != nil {
			return commandFailure(cmd, appForCommand(cmd), spec, commandRunModeWithJournal, exec, result.Result{}, err)
		}
		plan, err = updateusecase.RecoverPlan(report, selection)
		if err != nil {
			return commandFailure(cmd, appForCommand(cmd), spec, commandRunModeWithJournal, exec, result.Result{}, err)
		}
	} else {
		plan, err = updateusecase.BuildPlan(updateusecase.PlanRequest{
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
	}
	if err != nil {
		return commandFailure(cmd, appForCommand(cmd), spec, commandRunModeWithJournal, exec, result.Result{}, err)
	}

	res := updatePlanResult(plan)
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
			Err:     apperr.Wrap(apperr.KindValidation, "update_plan_blocked", errors.New("update dry-run plan found blocking conditions")),
			ErrCode: "update_plan_blocked",
		},
	)
}

func runUpdateExecute(cmd *cobra.Command, in updateInput) error {
	spec := CommandSpec{
		Name:       "update",
		ErrorCode:  "update_failed",
		ExitCode:   exitcode.InternalError,
		RenderText: renderUpdateText,
	}
	exec := operationusecase.Begin(
		appForCommand(cmd).runtime,
		appForCommand(cmd).journalWriterFactory(appForCommand(cmd).options.JournalDir),
		spec.Name,
	)

	var (
		info updateusecase.ExecuteInfo
		err  error
	)
	if in.recoverID != "" {
		report, selection, recoveryErr := loadUpdateRecovery(cmd, in)
		if recoveryErr != nil {
			return commandFailure(cmd, appForCommand(cmd), spec, commandRunModeWithJournal, exec, result.Result{}, recoveryErr)
		}
		info, err = updateusecase.RecoverExecute(report, selection, cmd.ErrOrStderr())
	} else {
		info, err = updateusecase.Execute(updateusecase.ExecuteRequest{
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
	}

	res := updateResult(info)
	res.Command = spec.Name

	if err != nil {
		return finishJournaledCommandFailure(cmd, spec, exec, res, err)
	}

	return finishJournaledCommandSuccess(cmd, spec, exec, res)
}

func loadUpdateRecovery(cmd *cobra.Command, in updateInput) (journalusecase.OperationReport, journalusecase.RecoverySelection, error) {
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
		return journalusecase.OperationReport{}, journalusecase.RecoverySelection{}, apperr.Wrap(apperr.KindValidation, "update_recovery_refused", err)
	}

	return report, selection, nil
}

func updateResult(info updateusecase.ExecuteInfo) result.Result {
	completed, skipped, failed, notRun := info.Counts()
	message := "update completed"
	if info.Recovery.Active() {
		message = "update recovery completed"
	}
	if !info.Ready() {
		message = "update failed"
		if info.Recovery.Active() {
			message = "update recovery failed"
		}
	}

	return result.Result{
		Command:  "update",
		OK:       info.Ready(),
		Message:  message,
		Warnings: append([]string(nil), info.Warnings...),
		Details: result.UpdateDetails{
			Scope:          info.Scope,
			Ready:          info.Ready(),
			Steps:          len(info.Steps),
			Completed:      completed,
			Skipped:        skipped,
			Failed:         failed,
			NotRun:         notRun,
			TimeoutSeconds: info.TimeoutSeconds,
			SkipDoctor:     info.SkipDoctor,
			SkipBackup:     info.SkipBackup,
			SkipPull:       info.SkipPull,
			SkipHTTPProbe:  info.SkipHTTPProbe,
			Recovery:       recoveryResultDetails(info.Recovery),
		},
		Artifacts: result.UpdateArtifacts{
			ProjectDir:     info.ProjectDir,
			ComposeFile:    info.ComposeFile,
			EnvFile:        info.EnvFile,
			ComposeProject: info.ComposeProject,
			BackupRoot:     info.BackupRoot,
			SiteURL:        info.SiteURL,
			ManifestTXT:    info.ManifestTXTPath,
			ManifestJSON:   info.ManifestJSONPath,
			DBBackup:       info.DBBackupPath,
			FilesBackup:    info.FilesBackupPath,
			DBChecksum:     info.DBSidecarPath,
			FilesChecksum:  info.FilesSidecarPath,
		},
		Items: updateItems(info.Steps),
	}
}

func renderUpdateText(w io.Writer, res result.Result) error {
	details, ok := res.Details.(result.UpdateDetails)
	if !ok {
		return result.Render(w, res, false)
	}

	artifacts, ok := res.Artifacts.(result.UpdateArtifacts)
	if !ok {
		return fmt.Errorf("unexpected update artifacts type %T", res.Artifacts)
	}

	if _, err := fmt.Fprintln(w, "EspoCRM update"); err != nil {
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

	if err := renderRecoveryAttemptSection(w, details.Recovery); err != nil {
		return err
	}

	if err := renderStepItemsBlock(w, res.Items, updateItem, stepRenderOptions{
		Title:      "Steps",
		StatusText: upperStatusText,
	}); err != nil {
		return err
	}

	artifactLines := []struct {
		label string
		value string
	}{
		{label: "Manifest txt", value: artifacts.ManifestTXT},
		{label: "Manifest json", value: artifacts.ManifestJSON},
		{label: "DB backup", value: artifacts.DBBackup},
		{label: "Files backup", value: artifacts.FilesBackup},
		{label: "DB checksum", value: artifacts.DBChecksum},
		{label: "Files checksum", value: artifacts.FilesChecksum},
	}
	artifactCount := 0
	for _, artifact := range artifactLines {
		if strings.TrimSpace(artifact.value) != "" {
			artifactCount++
		}
	}
	if artifactCount != 0 {
		if _, err := fmt.Fprintln(w, "\nArtifacts:"); err != nil {
			return err
		}
		for _, artifact := range artifactLines {
			if strings.TrimSpace(artifact.value) == "" {
				continue
			}
			if _, err := fmt.Fprintf(w, "  %-13s %s\n", artifact.label+":", artifact.value); err != nil {
				return err
			}
		}
	}

	return nil
}

func finishJournaledCommandSuccess(cmd *cobra.Command, spec CommandSpec, exec operationusecase.Execution, res result.Result) error {
	finished, err := exec.FinishSuccess(res)
	if err != nil {
		return err
	}

	return renderCommandResult(cmd, spec, finished)
}

func finishJournaledCommandFailure(cmd *cobra.Command, spec CommandSpec, exec operationusecase.Execution, res result.Result, err error) error {
	res.Command = spec.Name
	errCode := errorCodeForError(err, spec.ErrorCode)

	if journalErr := exec.FinishFailure(res, err, errCode); journalErr != nil {
		message := fmt.Sprintf("failed to write journal entry: %v", journalErr)
		if appForCommand(cmd).JSONEnabled() {
			res.Warnings = append(res.Warnings, message)
		} else if _, writeErr := fmt.Fprintf(cmd.ErrOrStderr(), "WARNING: %s\n", message); writeErr != nil {
			res.Warnings = append(res.Warnings, fmt.Sprintf("failed to render warning %q: %v", message, writeErr))
		}
	}

	codeErr := CodeError{
		Code:     codeForError(err, spec.ExitCode),
		Err:      err,
		ErrCode:  errCode,
		Warnings: warningMessages(err),
	}

	if appForCommand(cmd).JSONEnabled() {
		return ResultCodeError{
			CodeError: codeErr,
			Result:    res,
		}
	}

	if spec.RenderText != nil {
		if err := spec.RenderText(cmd.OutOrStdout(), res); err != nil {
			return err
		}
	} else if err := result.Render(cmd.OutOrStdout(), res, false); err != nil {
		return err
	}

	if err := renderWarnings(cmd.OutOrStdout(), res.Warnings); err != nil {
		return err
	}

	return silentCodeError{CodeError: codeErr}
}
