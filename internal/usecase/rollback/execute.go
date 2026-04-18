package rollback

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/lazuale/espocrm-ops/internal/contract/apperr"
	platformconfig "github.com/lazuale/espocrm-ops/internal/platform/config"
	platformdocker "github.com/lazuale/espocrm-ops/internal/platform/docker"
	backupusecase "github.com/lazuale/espocrm-ops/internal/usecase/backup"
	doctorusecase "github.com/lazuale/espocrm-ops/internal/usecase/doctor"
	maintenanceusecase "github.com/lazuale/espocrm-ops/internal/usecase/maintenance"
	restoreusecase "github.com/lazuale/espocrm-ops/internal/usecase/restore"
	updateusecase "github.com/lazuale/espocrm-ops/internal/usecase/update"
)

const (
	RollbackStepStatusCompleted = "completed"
	RollbackStepStatusSkipped   = "skipped"
	RollbackStepStatusFailed    = "failed"
	RollbackStepStatusNotRun    = "not_run"
)

var rollbackRuntimeServices = []string{
	"db",
	"espocrm",
	"espocrm-daemon",
	"espocrm-websocket",
}

var rollbackAppServices = []string{
	"espocrm",
	"espocrm-daemon",
	"espocrm-websocket",
}

type ExecuteRequest struct {
	Scope           string
	ProjectDir      string
	ComposeFile     string
	EnvFileOverride string
	EnvContourHint  string
	DBBackup        string
	FilesBackup     string
	NoSnapshot      bool
	NoStart         bool
	SkipHTTPProbe   bool
	TimeoutSeconds  int
	LogWriter       io.Writer
}

type ExecuteStep struct {
	Code    string
	Status  string
	Summary string
	Details string
	Action  string
}

type ExecuteInfo struct {
	Scope                 string
	ProjectDir            string
	ComposeFile           string
	EnvFile               string
	ComposeProject        string
	BackupRoot            string
	SiteURL               string
	SelectionMode         string
	SelectedPrefix        string
	SelectedStamp         string
	ManifestTXTPath       string
	ManifestJSONPath      string
	DBBackupPath          string
	FilesBackupPath       string
	TimeoutSeconds        int
	SnapshotEnabled       bool
	NoStart               bool
	SkipHTTPProbe         bool
	Warnings              []string
	Steps                 []ExecuteStep
	StartedDBTemporarily  bool
	SnapshotManifestTXT   string
	SnapshotManifestJSON  string
	SnapshotDBBackup      string
	SnapshotFilesBackup   string
	SnapshotDBChecksum    string
	SnapshotFilesChecksum string
	ServicesReady         []string
}

type runtimePrepareInfo struct {
	StartedDBTemporarily bool
	StoppedAppServices   []string
}

type runtimeReturnInfo struct {
	ServicesReady []string
}

func Execute(req ExecuteRequest) (ExecuteInfo, error) {
	info := ExecuteInfo{
		Scope:           strings.TrimSpace(req.Scope),
		ProjectDir:      filepath.Clean(req.ProjectDir),
		ComposeFile:     filepath.Clean(req.ComposeFile),
		TimeoutSeconds:  req.TimeoutSeconds,
		SnapshotEnabled: !req.NoSnapshot,
		NoStart:         req.NoStart,
		SkipHTTPProbe:   req.SkipHTTPProbe,
		Warnings:        flagWarnings(req),
	}
	readinessBudget := req.TimeoutSeconds

	ctx, err := maintenanceusecase.PrepareOperation(maintenanceusecase.OperationContextRequest{
		Scope:           info.Scope,
		Operation:       "rollback",
		ProjectDir:      info.ProjectDir,
		EnvFileOverride: strings.TrimSpace(req.EnvFileOverride),
		EnvContourHint:  strings.TrimSpace(req.EnvContourHint),
		LogWriter:       req.LogWriter,
	})
	if err != nil {
		info.Steps = append(info.Steps,
			ExecuteStep{
				Code:    "operation_preflight",
				Status:  RollbackStepStatusFailed,
				Summary: "Rollback preflight failed",
				Details: err.Error(),
				Action:  "Resolve env, lock, or filesystem readiness before rerunning rollback.",
			},
			notRunRollbackStep("doctor", "Doctor did not run because rollback preflight failed"),
			notRunRollbackStep("target_selection", "Rollback target selection did not run because rollback preflight failed"),
			notRunRollbackStep("runtime_prepare", "Runtime preparation did not run because rollback preflight failed"),
			notRunRollbackStep("snapshot_recovery_point", "Emergency recovery-point creation did not run because rollback preflight failed"),
			notRunRollbackStep("db_restore", "Database restore did not run because rollback preflight failed"),
			notRunRollbackStep("files_restore", "Files restore did not run because rollback preflight failed"),
			notRunRollbackStep("runtime_return", "Contour return did not run because rollback preflight failed"),
		)
		return info, wrapExecuteError(err)
	}
	defer func() {
		_ = ctx.Release()
	}()

	info.EnvFile = ctx.Env.FilePath
	info.ComposeProject = ctx.ComposeProject
	info.BackupRoot = ctx.BackupRoot
	info.SiteURL = strings.TrimSpace(ctx.Env.Value("SITE_URL"))

	info.Steps = append(info.Steps, ExecuteStep{
		Code:    "operation_preflight",
		Status:  RollbackStepStatusCompleted,
		Summary: "Rollback preflight completed",
		Details: fmt.Sprintf("Using %s for contour %s.", info.EnvFile, info.Scope),
	})

	logRollback(req.LogWriter, "Running the canonical rollback doctor checks")
	report, err := doctorusecase.Diagnose(doctorusecase.Request{
		Scope:                  info.Scope,
		ProjectDir:             info.ProjectDir,
		ComposeFile:            info.ComposeFile,
		EnvFileOverride:        info.EnvFile,
		EnvContourHint:         strings.TrimSpace(req.EnvContourHint),
		PathCheckMode:          doctorusecase.PathCheckModeReadOnly,
		InheritedOperationLock: true,
		InheritedMaintenance:   true,
	})
	if err != nil {
		info.Steps = append(info.Steps,
			ExecuteStep{
				Code:    "doctor",
				Status:  RollbackStepStatusFailed,
				Summary: "Doctor execution failed",
				Details: err.Error(),
				Action:  "Inspect Docker and env resolution, then rerun rollback.",
			},
			notRunRollbackStep("target_selection", "Rollback target selection did not run because doctor failed"),
			notRunRollbackStep("runtime_prepare", "Runtime preparation did not run because doctor failed"),
			notRunRollbackStep("snapshot_recovery_point", "Emergency recovery-point creation did not run because doctor failed"),
			notRunRollbackStep("db_restore", "Database restore did not run because doctor failed"),
			notRunRollbackStep("files_restore", "Files restore did not run because doctor failed"),
			notRunRollbackStep("runtime_return", "Contour return did not run because doctor failed"),
		)
		return info, apperr.Wrap(apperr.KindInternal, "rollback_failed", err)
	}

	info.Warnings = append(info.Warnings, collectDoctorWarnings(report.Checks)...)
	failures := blockingDoctorChecks(report.Checks)
	if len(failures) != 0 {
		info.Steps = append(info.Steps,
			ExecuteStep{
				Code:    "doctor",
				Status:  RollbackStepStatusFailed,
				Summary: "Doctor stopped the rollback",
				Details: formatDoctorChecks(failures),
				Action:  firstDoctorAction(failures, "Resolve the reported doctor failures before rerunning rollback."),
			},
			notRunRollbackStep("target_selection", "Rollback target selection did not run because doctor failed"),
			notRunRollbackStep("runtime_prepare", "Runtime preparation did not run because doctor failed"),
			notRunRollbackStep("snapshot_recovery_point", "Emergency recovery-point creation did not run because doctor failed"),
			notRunRollbackStep("db_restore", "Database restore did not run because doctor failed"),
			notRunRollbackStep("files_restore", "Files restore did not run because doctor failed"),
			notRunRollbackStep("runtime_return", "Contour return did not run because doctor failed"),
		)
		return info, apperr.Wrap(apperr.KindValidation, "rollback_failed", errors.New("doctor found blocking rollback failures"))
	}

	info.Steps = append(info.Steps, ExecuteStep{
		Code:    "doctor",
		Status:  RollbackStepStatusCompleted,
		Summary: "Doctor completed",
		Details: "The canonical rollback doctor checks passed.",
	})

	logRollback(req.LogWriter, "Selecting the rollback target")
	target, issues, selectionWarnings := resolveTarget(ctx.BackupRoot, nil, PlanRequest{
		DBBackup:    strings.TrimSpace(req.DBBackup),
		FilesBackup: strings.TrimSpace(req.FilesBackup),
	})
	info.Warnings = append(info.Warnings, selectionWarnings...)
	if len(issues) != 0 {
		info.Steps = append(info.Steps,
			ExecuteStep{
				Code:    "target_selection",
				Status:  RollbackStepStatusFailed,
				Summary: "Rollback target selection failed",
				Details: formatIssues(issues),
				Action:  firstIssueAction(issues, "Resolve the rollback target selection error before rerunning rollback."),
			},
			notRunRollbackStep("runtime_prepare", "Runtime preparation did not run because rollback target selection failed"),
			notRunRollbackStep("snapshot_recovery_point", "Emergency recovery-point creation did not run because rollback target selection failed"),
			notRunRollbackStep("db_restore", "Database restore did not run because rollback target selection failed"),
			notRunRollbackStep("files_restore", "Files restore did not run because rollback target selection failed"),
			notRunRollbackStep("runtime_return", "Contour return did not run because rollback target selection failed"),
		)
		return info, apperr.Wrap(apperr.KindValidation, "rollback_failed", errors.New("rollback target selection failed"))
	}

	info.SelectionMode = target.SelectionMode
	info.SelectedPrefix = target.Prefix
	info.SelectedStamp = target.Stamp
	info.ManifestTXTPath = target.ManifestTXT
	info.ManifestJSONPath = target.ManifestJSON
	info.DBBackupPath = target.DBBackup
	info.FilesBackupPath = target.FilesBackup
	info.Steps = append(info.Steps, ExecuteStep{
		Code:    "target_selection",
		Status:  RollbackStepStatusCompleted,
		Summary: targetSelectionSummary(info.SelectionMode),
		Details: targetSelectionDetails(info),
	})

	logRollback(req.LogWriter, "Preparing the runtime for rollback")
	runtimePrep, err := prepareRuntime(info.ProjectDir, info.ComposeFile, info.EnvFile, &readinessBudget)
	if err != nil {
		info.Steps = append(info.Steps,
			ExecuteStep{
				Code:    "runtime_prepare",
				Status:  RollbackStepStatusFailed,
				Summary: "Runtime preparation failed",
				Details: err.Error(),
				Action:  "Resolve the runtime preparation failure before rerunning rollback.",
			},
			notRunRollbackStep("snapshot_recovery_point", "Emergency recovery-point creation did not run because runtime preparation failed"),
			notRunRollbackStep("db_restore", "Database restore did not run because runtime preparation failed"),
			notRunRollbackStep("files_restore", "Files restore did not run because runtime preparation failed"),
			notRunRollbackStep("runtime_return", "Contour return did not run because runtime preparation failed"),
		)
		return info, wrapExecuteError(err)
	}
	info.StartedDBTemporarily = runtimePrep.StartedDBTemporarily
	info.Steps = append(info.Steps, ExecuteStep{
		Code:    "runtime_prepare",
		Status:  RollbackStepStatusCompleted,
		Summary: "Runtime preparation completed",
		Details: runtimePrepareDetails(runtimePrep, req.TimeoutSeconds),
	})

	if req.NoSnapshot {
		logRollback(req.LogWriter, "Emergency recovery-point creation skipped because of --no-snapshot")
		info.Steps = append(info.Steps, ExecuteStep{
			Code:    "snapshot_recovery_point",
			Status:  RollbackStepStatusSkipped,
			Summary: "Emergency recovery-point creation skipped",
			Details: "The emergency recovery point was skipped because of --no-snapshot.",
		})
	} else {
		logRollback(req.LogWriter, "Creating the emergency recovery point before rollback")
		snapshotInfo, err := updateusecase.ApplyBackup(buildSnapshotRequest(ctx, req))
		if err != nil {
			info.Steps = append(info.Steps,
				ExecuteStep{
					Code:    "snapshot_recovery_point",
					Status:  RollbackStepStatusFailed,
					Summary: "Emergency recovery-point creation failed",
					Details: err.Error(),
					Action:  "Resolve the emergency recovery-point failure before rerunning rollback.",
				},
				notRunRollbackStep("db_restore", "Database restore did not run because the emergency recovery point failed"),
				notRunRollbackStep("files_restore", "Files restore did not run because the emergency recovery point failed"),
				notRunRollbackStep("runtime_return", "Contour return did not run because the emergency recovery point failed"),
			)
			return info, wrapExecuteError(err)
		}

		info.SnapshotManifestTXT = snapshotInfo.ManifestTXTPath
		info.SnapshotManifestJSON = snapshotInfo.ManifestJSONPath
		info.SnapshotDBBackup = snapshotInfo.DBBackupPath
		info.SnapshotFilesBackup = snapshotInfo.FilesBackupPath
		info.SnapshotDBChecksum = snapshotInfo.DBSidecarPath
		info.SnapshotFilesChecksum = snapshotInfo.FilesSidecarPath
		info.Steps = append(info.Steps, ExecuteStep{
			Code:    "snapshot_recovery_point",
			Status:  RollbackStepStatusCompleted,
			Summary: "Emergency recovery-point creation completed",
			Details: snapshotDetails(snapshotInfo),
		})
	}

	cfg := platformdocker.ComposeConfig{
		ProjectDir:  info.ProjectDir,
		ComposeFile: info.ComposeFile,
		EnvFile:     info.EnvFile,
	}
	dbContainer, err := platformdocker.ComposeServiceContainerID(cfg, "db")
	if err != nil {
		info.Steps = append(info.Steps,
			ExecuteStep{
				Code:    "db_restore",
				Status:  RollbackStepStatusFailed,
				Summary: "Database restore failed",
				Details: err.Error(),
				Action:  "Resolve the db container inspection failure before rerunning rollback.",
			},
			notRunRollbackStep("files_restore", "Files restore did not run because the database restore failed"),
			notRunRollbackStep("runtime_return", "Contour return did not run because the database restore failed"),
		)
		return info, wrapExecuteError(err)
	}
	if strings.TrimSpace(dbContainer) == "" {
		err = apperr.Wrap(apperr.KindExternal, "rollback_failed", errors.New("could not resolve the db container for rollback"))
		info.Steps = append(info.Steps,
			ExecuteStep{
				Code:    "db_restore",
				Status:  RollbackStepStatusFailed,
				Summary: "Database restore failed",
				Details: err.Error(),
				Action:  "Start the db service for this contour and rerun rollback.",
			},
			notRunRollbackStep("files_restore", "Files restore did not run because the database restore failed"),
			notRunRollbackStep("runtime_return", "Contour return did not run because the database restore failed"),
		)
		return info, err
	}

	logRollback(req.LogWriter, "Restoring the selected database backup")
	if _, err := restoreusecase.RestoreDB(buildDBRestoreRequest(ctx, info, dbContainer)); err != nil {
		info.Steps = append(info.Steps,
			ExecuteStep{
				Code:    "db_restore",
				Status:  RollbackStepStatusFailed,
				Summary: "Database restore failed",
				Details: err.Error(),
				Action:  "Resolve the database restore failure before rerunning rollback.",
			},
			notRunRollbackStep("files_restore", "Files restore did not run because the database restore failed"),
			notRunRollbackStep("runtime_return", "Contour return did not run because the database restore failed"),
		)
		return info, wrapExecuteError(err)
	}
	info.Steps = append(info.Steps, ExecuteStep{
		Code:    "db_restore",
		Status:  RollbackStepStatusCompleted,
		Summary: "Database restore completed",
		Details: dbRestoreDetails(ctx, info),
	})

	logRollback(req.LogWriter, "Restoring the selected files backup")
	if _, err := restoreusecase.RestoreFiles(buildFilesRestoreRequest(ctx, info)); err != nil {
		info.Steps = append(info.Steps,
			ExecuteStep{
				Code:    "files_restore",
				Status:  RollbackStepStatusFailed,
				Summary: "Files restore failed",
				Details: err.Error(),
				Action:  "Resolve the files restore failure before rerunning rollback.",
			},
			notRunRollbackStep("runtime_return", "Contour return did not run because the files restore failed"),
		)
		return info, wrapExecuteError(err)
	}
	info.Steps = append(info.Steps, ExecuteStep{
		Code:    "files_restore",
		Status:  RollbackStepStatusCompleted,
		Summary: "Files restore completed",
		Details: filesRestoreDetails(ctx, info),
	})

	if req.NoStart {
		logRollback(req.LogWriter, "Contour return skipped because of --no-start")
		info.Steps = append(info.Steps, ExecuteStep{
			Code:    "runtime_return",
			Status:  RollbackStepStatusSkipped,
			Summary: "Contour return skipped",
			Details: "The contour was left stopped because of --no-start.",
		})
		info.Warnings = dedupeStrings(info.Warnings)
		return info, nil
	}

	logRollback(req.LogWriter, "Returning the contour to service")
	runtimeReturn, err := returnRuntime(info.ProjectDir, info.ComposeFile, info.EnvFile, info.SiteURL, req.SkipHTTPProbe, &readinessBudget)
	info.ServicesReady = append([]string(nil), runtimeReturn.ServicesReady...)
	if err != nil {
		info.Steps = append(info.Steps, ExecuteStep{
			Code:    "runtime_return",
			Status:  RollbackStepStatusFailed,
			Summary: "Contour return failed",
			Details: err.Error(),
			Action:  "Resolve the runtime readiness failure before rerunning rollback.",
		})
		return info, wrapExecuteError(err)
	}
	info.Steps = append(info.Steps, ExecuteStep{
		Code:    "runtime_return",
		Status:  RollbackStepStatusCompleted,
		Summary: "Contour return completed",
		Details: runtimeReturnDetails(info),
	})

	info.Warnings = dedupeStrings(info.Warnings)
	return info, nil
}

func (i ExecuteInfo) Counts() (completed, skipped, failed, notRun int) {
	for _, step := range i.Steps {
		switch step.Status {
		case RollbackStepStatusCompleted:
			completed++
		case RollbackStepStatusSkipped:
			skipped++
		case RollbackStepStatusFailed:
			failed++
		case RollbackStepStatusNotRun:
			notRun++
		}
	}

	return completed, skipped, failed, notRun
}

func (i ExecuteInfo) Ready() bool {
	for _, step := range i.Steps {
		if step.Status == RollbackStepStatusFailed {
			return false
		}
	}

	return true
}

func prepareRuntime(projectDir, composeFile, envFile string, timeoutBudget *int) (runtimePrepareInfo, error) {
	info := runtimePrepareInfo{}
	cfg := platformdocker.ComposeConfig{
		ProjectDir:  projectDir,
		ComposeFile: composeFile,
		EnvFile:     envFile,
	}

	dbState, err := platformdocker.ComposeServiceStateFor(cfg, "db")
	if err != nil {
		return info, apperr.Wrap(apperr.KindExternal, "rollback_failed", err)
	}

	if dbState.Status != "running" && dbState.Status != "healthy" {
		info.StartedDBTemporarily = true
		if err := platformdocker.ComposeUp(cfg, "db"); err != nil {
			return info, apperr.Wrap(apperr.KindExternal, "rollback_failed", err)
		}
	}

	if err := waitForServiceReadyWithSharedTimeout(timeoutBudget, cfg, "db", "rollback"); err != nil {
		return info, apperr.Wrap(apperr.KindExternal, "rollback_failed", err)
	}

	runningServices, err := platformdocker.ComposeRunningServices(cfg)
	if err != nil {
		return info, apperr.Wrap(apperr.KindExternal, "rollback_failed", err)
	}
	if rollbackAppServicesRunning(runningServices) {
		info.StoppedAppServices = runningAppServices(runningServices)
		if err := platformdocker.ComposeStop(cfg, rollbackAppServices...); err != nil {
			return info, apperr.Wrap(apperr.KindExternal, "rollback_failed", err)
		}
	}

	return info, nil
}

func returnRuntime(projectDir, composeFile, envFile, siteURL string, skipHTTPProbe bool, timeoutBudget *int) (runtimeReturnInfo, error) {
	info := runtimeReturnInfo{}
	cfg := platformdocker.ComposeConfig{
		ProjectDir:  projectDir,
		ComposeFile: composeFile,
		EnvFile:     envFile,
	}

	if err := platformdocker.ComposeUp(cfg); err != nil {
		return info, apperr.Wrap(apperr.KindExternal, "rollback_failed", err)
	}

	for _, service := range rollbackRuntimeServices {
		if err := waitForServiceReadyWithSharedTimeout(timeoutBudget, cfg, service, "rollback"); err != nil {
			return info, apperr.Wrap(apperr.KindExternal, "rollback_failed", err)
		}
		info.ServicesReady = append(info.ServicesReady, service)
	}

	if !skipHTTPProbe {
		if err := rollbackHTTPProbe(siteURL); err != nil {
			return info, apperr.Wrap(apperr.KindExternal, "rollback_failed", err)
		}
	}

	return info, nil
}

func waitForServiceReadyWithSharedTimeout(timeoutBudget *int, cfg platformdocker.ComposeConfig, service, scope string) error {
	if timeoutBudget == nil {
		return errors.New("shared readiness timeout budget is not configured")
	}
	if *timeoutBudget <= 0 {
		return fmt.Errorf("shared readiness timeout for %s was exhausted before service '%s'", scope, service)
	}

	started := time.Now().UTC()
	if err := waitForServiceReady(cfg, service, *timeoutBudget); err != nil {
		return err
	}

	elapsed := int(time.Since(started).Seconds())
	*timeoutBudget -= elapsed
	if *timeoutBudget < 0 {
		*timeoutBudget = 0
	}

	return nil
}

func waitForServiceReady(cfg platformdocker.ComposeConfig, service string, timeoutSeconds int) error {
	deadline := time.Now().UTC().Add(time.Duration(timeoutSeconds) * time.Second)

	for {
		state, err := platformdocker.ComposeServiceStateFor(cfg, service)
		if err != nil {
			return err
		}

		switch state.Status {
		case "healthy", "running":
			return nil
		case "exited", "dead":
			return fmt.Errorf("service '%s' crashed while waiting for readiness", service)
		case "unhealthy":
			if strings.TrimSpace(state.HealthMessage) != "" {
				return fmt.Errorf("service '%s' became unhealthy while waiting for readiness: %s", service, state.HealthMessage)
			}
			return fmt.Errorf("service '%s' became unhealthy while waiting for readiness", service)
		}

		remaining := time.Until(deadline)
		if remaining <= 0 {
			return fmt.Errorf("timed out while waiting for service readiness '%s' (%d sec.)", service, timeoutSeconds)
		}

		sleepFor := rollbackReadinessPollInterval
		if remaining < sleepFor {
			sleepFor = remaining
		}
		time.Sleep(sleepFor)
	}
}

func rollbackHTTPProbe(siteURL string) error {
	if strings.TrimSpace(siteURL) == "" {
		return errors.New("SITE_URL is required for the rollback HTTP probe")
	}

	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	resp, err := client.Get(siteURL)
	if err != nil {
		return fmt.Errorf("http probe failed: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusBadRequest {
		return fmt.Errorf("http probe failed: unexpected status %s", resp.Status)
	}

	return nil
}

const rollbackReadinessPollInterval = 5 * time.Second

func buildSnapshotRequest(ctx maintenanceusecase.OperationContext, req ExecuteRequest) updateusecase.BackupApplyRequest {
	return updateusecase.BackupApplyRequest{
		TimeoutSeconds: req.TimeoutSeconds,
		LogWriter:      req.LogWriter,
		Backup: backupusecase.ExecuteRequest{
			Scope:          ctx.Scope,
			ProjectDir:     ctx.ProjectDir,
			ComposeFile:    filepath.Clean(req.ComposeFile),
			EnvFile:        ctx.Env.FilePath,
			BackupRoot:     ctx.BackupRoot,
			StorageDir:     platformconfig.ResolveProjectPath(ctx.ProjectDir, ctx.Env.ESPOStorageDir()),
			NamePrefix:     resolvedBackupNamePrefix(ctx.Env),
			RetentionDays:  resolvedRetentionDays(ctx.Env),
			ComposeProject: ctx.ComposeProject,
			DBUser:         strings.TrimSpace(ctx.Env.Value("DB_USER")),
			DBPassword:     ctx.Env.Value("DB_PASSWORD"),
			DBName:         strings.TrimSpace(ctx.Env.Value("DB_NAME")),
			EspoCRMImage:   strings.TrimSpace(ctx.Env.Value("ESPOCRM_IMAGE")),
			MariaDBTag:     strings.TrimSpace(ctx.Env.Value("MARIADB_TAG")),
			LogWriter:      req.LogWriter,
			ErrWriter:      req.LogWriter,
		},
	}
}

func buildDBRestoreRequest(ctx maintenanceusecase.OperationContext, info ExecuteInfo, dbContainer string) restoreusecase.RestoreDBRequest {
	req := restoreusecase.RestoreDBRequest{
		DBContainer:    dbContainer,
		DBName:         strings.TrimSpace(ctx.Env.Value("DB_NAME")),
		DBUser:         strings.TrimSpace(ctx.Env.Value("DB_USER")),
		DBPassword:     ctx.Env.Value("DB_PASSWORD"),
		DBRootPassword: ctx.Env.Value("DB_ROOT_PASSWORD"),
	}

	if strings.TrimSpace(info.ManifestJSONPath) != "" {
		req.ManifestPath = info.ManifestJSONPath
		return req
	}

	req.DBBackup = info.DBBackupPath
	return req
}

func buildFilesRestoreRequest(ctx maintenanceusecase.OperationContext, info ExecuteInfo) restoreusecase.RestoreFilesRequest {
	req := restoreusecase.RestoreFilesRequest{
		TargetDir: platformconfig.ResolveProjectPath(ctx.ProjectDir, ctx.Env.ESPOStorageDir()),
	}

	if strings.TrimSpace(info.ManifestJSONPath) != "" {
		req.ManifestPath = info.ManifestJSONPath
		return req
	}

	req.FilesBackup = info.FilesBackupPath
	return req
}

func collectDoctorWarnings(checks []doctorusecase.Check) []string {
	warnings := []string{}
	for _, check := range checks {
		switch check.Status {
		case "warn":
			warnings = append(warnings, formatCheckWarning("", check))
		case "fail":
			if !doctorCheckBlocksRollback(check) {
				warnings = append(warnings, formatCheckWarning("Current state is degraded: ", check))
			}
		}
	}

	return warnings
}

func flagWarnings(req ExecuteRequest) []string {
	warnings := []string{}
	if req.NoSnapshot {
		warnings = append(warnings, "Rollback will skip the emergency recovery point because of --no-snapshot.")
	}
	if req.NoStart {
		warnings = append(warnings, "Rollback will leave the contour stopped because of --no-start.")
	}
	if req.SkipHTTPProbe {
		warnings = append(warnings, "Rollback will skip the final HTTP probe because of --skip-http-probe.")
	}

	return warnings
}

func notRunRollbackStep(code, summary string) ExecuteStep {
	return ExecuteStep{
		Code:    code,
		Status:  RollbackStepStatusNotRun,
		Summary: summary,
	}
}

func runtimePrepareDetails(info runtimePrepareInfo, timeoutSeconds int) string {
	dbMode := "The db service was already running and ready."
	if info.StartedDBTemporarily {
		dbMode = "The db service was started temporarily and confirmed ready."
	}
	appMode := "Application services were already stopped."
	if len(info.StoppedAppServices) != 0 {
		appMode = fmt.Sprintf("Stopped the application services: %s.", strings.Join(info.StoppedAppServices, ", "))
	}

	return fmt.Sprintf("%s %s Readiness waits are sharing a %d second timeout budget.", dbMode, appMode, timeoutSeconds)
}

func snapshotDetails(info updateusecase.BackupApplyInfo) string {
	if strings.TrimSpace(info.ManifestJSONPath) == "" {
		return "Created the emergency recovery point before rollback."
	}

	return fmt.Sprintf("Created the emergency recovery point at %s.", info.ManifestJSONPath)
}

func targetSelectionSummary(mode string) string {
	if mode == "explicit" {
		return "Explicit rollback target selection completed"
	}

	return "Automatic rollback target selection completed"
}

func targetSelectionDetails(info ExecuteInfo) string {
	if info.SelectionMode == "explicit" {
		details := fmt.Sprintf("Selected DB backup %s and files backup %s.", info.DBBackupPath, info.FilesBackupPath)
		if strings.TrimSpace(info.ManifestJSONPath) != "" {
			details += fmt.Sprintf(" Matching manifest %s will anchor the restore contract.", info.ManifestJSONPath)
		}
		return details
	}

	return fmt.Sprintf("Selected prefix %s at %s with manifest %s.", info.SelectedPrefix, info.SelectedStamp, info.ManifestJSONPath)
}

func dbRestoreDetails(ctx maintenanceusecase.OperationContext, info ExecuteInfo) string {
	return fmt.Sprintf("Restored database %s from the selected rollback target for %s.", strings.TrimSpace(ctx.Env.Value("DB_NAME")), rollbackSourceLabel(info))
}

func filesRestoreDetails(ctx maintenanceusecase.OperationContext, info ExecuteInfo) string {
	targetDir := platformconfig.ResolveProjectPath(ctx.ProjectDir, ctx.Env.ESPOStorageDir())
	return fmt.Sprintf("Restored %s from the selected rollback target for %s.", targetDir, rollbackSourceLabel(info))
}

func rollbackSourceLabel(info ExecuteInfo) string {
	if strings.TrimSpace(info.ManifestJSONPath) != "" {
		return info.ManifestJSONPath
	}
	return fmt.Sprintf("%s and %s", info.DBBackupPath, info.FilesBackupPath)
}

func runtimeReturnDetails(info ExecuteInfo) string {
	details := fmt.Sprintf("Confirmed readiness for %s.", strings.Join(info.ServicesReady, ", "))
	if info.SkipHTTPProbe {
		return details + " The final HTTP probe was skipped because of --skip-http-probe."
	}
	if strings.TrimSpace(info.SiteURL) == "" {
		return details
	}
	return details + fmt.Sprintf(" The final HTTP probe succeeded for %s.", info.SiteURL)
}

func rollbackAppServicesRunning(services []string) bool {
	for _, service := range services {
		for _, appService := range rollbackAppServices {
			if service == appService {
				return true
			}
		}
	}

	return false
}

func runningAppServices(services []string) []string {
	set := map[string]struct{}{}
	for _, service := range services {
		set[service] = struct{}{}
	}

	items := make([]string, 0, len(rollbackAppServices))
	for _, service := range rollbackAppServices {
		if _, ok := set[service]; ok {
			items = append(items, service)
		}
	}

	return items
}

func resolvedBackupNamePrefix(env platformconfig.OperationEnv) string {
	namePrefix := strings.TrimSpace(env.Value("BACKUP_NAME_PREFIX"))
	if namePrefix == "" {
		namePrefix = strings.TrimSpace(env.ComposeProject())
	}
	return namePrefix
}

func resolvedRetentionDays(env platformconfig.OperationEnv) int {
	raw := strings.TrimSpace(env.Value("BACKUP_RETENTION_DAYS"))
	if raw == "" {
		return 7
	}

	value, err := strconv.Atoi(raw)
	if err != nil || value < 0 {
		return 7
	}

	return value
}

func logRollback(w io.Writer, message string) {
	if w == nil {
		return
	}

	_, _ = fmt.Fprintln(w, message)
}

func wrapExecuteError(err error) error {
	if kind, ok := apperr.KindOf(err); ok {
		return apperr.Wrap(kind, "rollback_failed", err)
	}

	return apperr.Wrap(apperr.KindInternal, "rollback_failed", err)
}
