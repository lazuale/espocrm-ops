package update

import (
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/lazuale/espocrm-ops/internal/contract/apperr"
	platformconfig "github.com/lazuale/espocrm-ops/internal/platform/config"
	backupusecase "github.com/lazuale/espocrm-ops/internal/usecase/backup"
	doctorusecase "github.com/lazuale/espocrm-ops/internal/usecase/doctor"
	maintenanceusecase "github.com/lazuale/espocrm-ops/internal/usecase/maintenance"
	operationusecase "github.com/lazuale/espocrm-ops/internal/usecase/operation"
)

const (
	UpdateStepStatusCompleted = "completed"
	UpdateStepStatusSkipped   = "skipped"
	UpdateStepStatusFailed    = "failed"
	UpdateStepStatusNotRun    = "not_run"
)

type ExecuteRequest struct {
	Scope           string
	ProjectDir      string
	ComposeFile     string
	EnvFileOverride string
	EnvContourHint  string
	TimeoutSeconds  int
	SkipDoctor      bool
	SkipBackup      bool
	SkipPull        bool
	SkipHTTPProbe   bool
	LogWriter       io.Writer
	Recovery        *ExecuteRecovery
}

type ExecuteStep struct {
	Code    string
	Status  string
	Summary string
	Details string
	Action  string
}

type ExecuteInfo struct {
	Scope                  string
	ProjectDir             string
	ComposeFile            string
	EnvFile                string
	ComposeProject         string
	BackupRoot             string
	SiteURL                string
	TimeoutSeconds         int
	SkipDoctor             bool
	SkipBackup             bool
	SkipPull               bool
	SkipHTTPProbe          bool
	Warnings               []string
	Steps                  []ExecuteStep
	StartedDBTemporarily   bool
	CreatedAt              string
	ConsistentSnapshot     bool
	AppServicesWereRunning bool
	ManifestTXTPath        string
	ManifestJSONPath       string
	DBBackupPath           string
	FilesBackupPath        string
	DBSidecarPath          string
	FilesSidecarPath       string
	ServicesReady          []string
	Recovery               operationusecase.RecoveryInfo
}

func Execute(req ExecuteRequest) (ExecuteInfo, error) {
	info := ExecuteInfo{
		Scope:          strings.TrimSpace(req.Scope),
		ProjectDir:     filepath.Clean(req.ProjectDir),
		ComposeFile:    filepath.Clean(req.ComposeFile),
		TimeoutSeconds: req.TimeoutSeconds,
		SkipDoctor:     req.SkipDoctor,
		SkipBackup:     req.SkipBackup,
		SkipPull:       req.SkipPull,
		SkipHTTPProbe:  req.SkipHTTPProbe,
	}
	if req.Recovery != nil {
		info.Recovery = req.Recovery.Info
	}

	ctx, err := maintenanceusecase.PrepareOperation(maintenanceusecase.OperationContextRequest{
		Scope:           info.Scope,
		Operation:       "update",
		ProjectDir:      info.ProjectDir,
		EnvFileOverride: strings.TrimSpace(req.EnvFileOverride),
		EnvContourHint:  strings.TrimSpace(req.EnvContourHint),
		LogWriter:       req.LogWriter,
	})
	if err != nil {
		info.Steps = append(info.Steps,
			ExecuteStep{
				Code:    "operation_preflight",
				Status:  UpdateStepStatusFailed,
				Summary: "Update preflight failed",
				Details: err.Error(),
				Action:  "Resolve env, lock, or filesystem readiness before rerunning update.",
			},
			notRunUpdateStep("doctor", "Doctor did not run because update preflight failed"),
			notRunUpdateStep("backup_recovery_point", "Recovery-point creation did not run because update preflight failed"),
			notRunUpdateStep("runtime_apply", "Runtime apply did not run because update preflight failed"),
			notRunUpdateStep("runtime_readiness", "Runtime readiness checks did not run because update preflight failed"),
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
		Status:  UpdateStepStatusCompleted,
		Summary: "Update preflight completed",
		Details: fmt.Sprintf("Using %s for contour %s.", info.EnvFile, info.Scope),
	})

	if req.Recovery != nil && req.Recovery.shouldSkip("doctor") {
		logUpdate(req.LogWriter, "Doctor skipped because recovery is resuming after a completed source step")
		info.Steps = append(info.Steps, req.Recovery.skippedExecuteStep("doctor"))
	} else if req.SkipDoctor {
		logUpdate(req.LogWriter, "Doctor skipped because of --skip-doctor")
		info.Steps = append(info.Steps, ExecuteStep{
			Code:    "doctor",
			Status:  UpdateStepStatusSkipped,
			Summary: "Doctor skipped",
			Details: "The canonical doctor checks were skipped because of --skip-doctor.",
		})
	} else {
		logUpdate(req.LogWriter, "Running the canonical doctor checks")
		report, err := doctorusecase.Diagnose(doctorusecase.Request{
			Scope:                  info.Scope,
			ProjectDir:             info.ProjectDir,
			ComposeFile:            info.ComposeFile,
			EnvFileOverride:        info.EnvFile,
			EnvContourHint:         strings.TrimSpace(req.EnvContourHint),
			InheritedOperationLock: true,
			InheritedMaintenance:   true,
		})
		if err != nil {
			info.Steps = append(info.Steps,
				ExecuteStep{
					Code:    "doctor",
					Status:  UpdateStepStatusFailed,
					Summary: "Doctor execution failed",
					Details: err.Error(),
					Action:  "Inspect Docker and env resolution, then rerun update.",
				},
				notRunUpdateStep("backup_recovery_point", "Recovery-point creation did not run because doctor failed"),
				notRunUpdateStep("runtime_apply", "Runtime apply did not run because doctor failed"),
				notRunUpdateStep("runtime_readiness", "Runtime readiness checks did not run because doctor failed"),
			)
			return info, apperr.Wrap(apperr.KindInternal, "update_failed", err)
		}

		info.Warnings = append(info.Warnings, collectPlanWarnings(report.Checks, false)...)
		failures := failingDoctorChecks(report.Checks)
		if len(failures) != 0 {
			info.Steps = append(info.Steps,
				ExecuteStep{
					Code:    "doctor",
					Status:  UpdateStepStatusFailed,
					Summary: "Doctor stopped the update",
					Details: formatDoctorChecks(failures),
					Action:  firstDoctorAction(failures, "Resolve the reported doctor failures before rerunning update."),
				},
				notRunUpdateStep("backup_recovery_point", "Recovery-point creation did not run because doctor failed"),
				notRunUpdateStep("runtime_apply", "Runtime apply did not run because doctor failed"),
				notRunUpdateStep("runtime_readiness", "Runtime readiness checks did not run because doctor failed"),
			)
			return info, apperr.Wrap(apperr.KindValidation, "update_failed", errors.New("doctor found readiness failures"))
		}

		info.Steps = append(info.Steps, ExecuteStep{
			Code:    "doctor",
			Status:  UpdateStepStatusCompleted,
			Summary: "Doctor completed",
			Details: "The canonical doctor checks passed for this update run.",
		})
	}

	if req.Recovery != nil && req.Recovery.shouldSkip("backup_recovery_point") {
		logUpdate(req.LogWriter, "Recovery-point creation skipped because recovery is resuming after a completed source step")
		info.Steps = append(info.Steps, req.Recovery.skippedExecuteStep("backup_recovery_point"))
	} else if req.SkipBackup {
		logUpdate(req.LogWriter, "Recovery-point creation skipped because of --skip-backup")
		info.Steps = append(info.Steps, ExecuteStep{
			Code:    "backup_recovery_point",
			Status:  UpdateStepStatusSkipped,
			Summary: "Recovery-point creation skipped",
			Details: "The update did not create a pre-update recovery point because of --skip-backup.",
		})
	} else {
		logUpdate(req.LogWriter, "Creating the pre-update recovery point")
		backupInfo, err := ApplyBackup(buildExecuteBackupRequest(ctx, req))
		if err != nil {
			info.Steps = append(info.Steps,
				ExecuteStep{
					Code:    "backup_recovery_point",
					Status:  UpdateStepStatusFailed,
					Summary: "Recovery-point creation failed",
					Details: err.Error(),
					Action:  "Resolve the recovery-point failure before rerunning update.",
				},
				notRunUpdateStep("runtime_apply", "Runtime apply did not run because recovery-point creation failed"),
				notRunUpdateStep("runtime_readiness", "Runtime readiness checks did not run because recovery-point creation failed"),
			)
			return info, wrapExecuteError(err)
		}

		info.StartedDBTemporarily = backupInfo.StartedDBTemporarily
		info.CreatedAt = backupInfo.CreatedAt
		info.ConsistentSnapshot = backupInfo.ConsistentSnapshot
		info.AppServicesWereRunning = backupInfo.AppServicesWereRunning
		info.ManifestTXTPath = backupInfo.ManifestTXTPath
		info.ManifestJSONPath = backupInfo.ManifestJSONPath
		info.DBBackupPath = backupInfo.DBBackupPath
		info.FilesBackupPath = backupInfo.FilesBackupPath
		info.DBSidecarPath = backupInfo.DBSidecarPath
		info.FilesSidecarPath = backupInfo.FilesSidecarPath

		details := "The canonical Go recovery-point step completed."
		if info.ManifestJSONPath != "" {
			details = fmt.Sprintf("Created the canonical recovery point at %s.", info.ManifestJSONPath)
		}
		info.Steps = append(info.Steps, ExecuteStep{
			Code:    "backup_recovery_point",
			Status:  UpdateStepStatusCompleted,
			Summary: "Recovery-point creation completed",
			Details: details,
		})
	}

	if req.Recovery != nil && req.Recovery.shouldSkip("runtime_apply") {
		logUpdate(req.LogWriter, "Runtime apply skipped because recovery is resuming after a completed source step")
		info.Steps = append(info.Steps, req.Recovery.skippedExecuteStep("runtime_apply"))

		runtimeInfo, err := ApplyRuntimeReadiness(RuntimeReadinessRequest{
			ProjectDir:     info.ProjectDir,
			ComposeFile:    info.ComposeFile,
			EnvFile:        info.EnvFile,
			SiteURL:        info.SiteURL,
			TimeoutSeconds: req.TimeoutSeconds,
			SkipHTTPProbe:  req.SkipHTTPProbe,
		})
		if err != nil {
			info.ServicesReady = append([]string(nil), runtimeInfo.ServicesReady...)
			info.Steps = append(info.Steps, ExecuteStep{
				Code:    "runtime_readiness",
				Status:  UpdateStepStatusFailed,
				Summary: "Runtime readiness checks failed",
				Details: err.Error(),
				Action:  "Resolve the runtime readiness failure before rerunning update.",
			})
			return info, wrapExecuteError(err)
		}

		info.ServicesReady = append([]string(nil), runtimeInfo.ServicesReady...)
		info.Steps = append(info.Steps, ExecuteStep{
			Code:    "runtime_readiness",
			Status:  UpdateStepStatusCompleted,
			Summary: "Runtime readiness checks completed",
			Details: runtimeReadinessDetails(runtimeInfo, info.SiteURL),
		})
		return info, nil
	}

	runtimeInfo, err := ApplyRuntime(RuntimeApplyRequest{
		ProjectDir:     info.ProjectDir,
		ComposeFile:    info.ComposeFile,
		EnvFile:        info.EnvFile,
		SiteURL:        info.SiteURL,
		TimeoutSeconds: req.TimeoutSeconds,
		SkipPull:       req.SkipPull,
		SkipHTTPProbe:  req.SkipHTTPProbe,
	})
	if err != nil {
		appendRuntimeFailureSteps(&info, runtimeInfo, err)
		return info, wrapExecuteError(err)
	}

	info.ServicesReady = append([]string(nil), runtimeInfo.ServicesReady...)
	info.Steps = append(info.Steps,
		ExecuteStep{
			Code:    "runtime_apply",
			Status:  UpdateStepStatusCompleted,
			Summary: "Runtime apply completed",
			Details: runtimeApplyDetails(runtimeInfo),
		},
		ExecuteStep{
			Code:    "runtime_readiness",
			Status:  UpdateStepStatusCompleted,
			Summary: "Runtime readiness checks completed",
			Details: runtimeReadinessDetails(runtimeInfo, info.SiteURL),
		},
	)

	return info, nil
}

func buildExecuteBackupRequest(ctx maintenanceusecase.OperationContext, req ExecuteRequest) BackupApplyRequest {
	return BackupApplyRequest{
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

func (i ExecuteInfo) Counts() (completed, skipped, failed, notRun int) {
	for _, step := range i.Steps {
		switch step.Status {
		case UpdateStepStatusCompleted:
			completed++
		case UpdateStepStatusSkipped:
			skipped++
		case UpdateStepStatusFailed:
			failed++
		case UpdateStepStatusNotRun:
			notRun++
		}
	}

	return completed, skipped, failed, notRun
}

func (i ExecuteInfo) Ready() bool {
	for _, step := range i.Steps {
		if step.Status == UpdateStepStatusFailed {
			return false
		}
	}

	return true
}

func appendRuntimeFailureSteps(info *ExecuteInfo, runtimeInfo RuntimeApplyInfo, err error) {
	switch runtimeInfo.FailedStage {
	case RuntimeStageRuntimeReadiness, RuntimeStageHTTPProbe:
		info.ServicesReady = append([]string(nil), runtimeInfo.ServicesReady...)
		info.Steps = append(info.Steps,
			ExecuteStep{
				Code:    "runtime_apply",
				Status:  UpdateStepStatusCompleted,
				Summary: "Runtime apply completed",
				Details: runtimeApplyDetails(runtimeInfo),
			},
			ExecuteStep{
				Code:    "runtime_readiness",
				Status:  UpdateStepStatusFailed,
				Summary: "Runtime readiness checks failed",
				Details: err.Error(),
				Action:  "Resolve the runtime readiness failure before rerunning update.",
			},
		)
	default:
		info.Steps = append(info.Steps,
			ExecuteStep{
				Code:    "runtime_apply",
				Status:  UpdateStepStatusFailed,
				Summary: "Runtime apply failed",
				Details: err.Error(),
				Action:  "Resolve the runtime apply failure before rerunning update.",
			},
			notRunUpdateStep("runtime_readiness", "Runtime readiness checks did not run because runtime apply failed"),
		)
	}
}

func runtimeApplyDetails(info RuntimeApplyInfo) string {
	if info.SkipPull {
		return "Restarted the stack with the current configuration without pulling new images."
	}

	return "Pulled updated images and restarted the stack with the current configuration."
}

func runtimeReadinessDetails(info RuntimeApplyInfo, siteURL string) string {
	details := fmt.Sprintf("Confirmed readiness for %s.", strings.Join(info.ServicesReady, ", "))
	if info.SkipHTTPProbe {
		return details + " The final HTTP probe was skipped because of --skip-http-probe."
	}
	if strings.TrimSpace(siteURL) == "" {
		return details
	}
	return details + fmt.Sprintf(" The final HTTP probe succeeded for %s.", siteURL)
}

func notRunUpdateStep(code, summary string) ExecuteStep {
	return ExecuteStep{
		Code:    code,
		Status:  UpdateStepStatusNotRun,
		Summary: summary,
	}
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

func logUpdate(w io.Writer, message string) {
	if w == nil {
		return
	}

	_, _ = fmt.Fprintln(w, message)
}

func wrapExecuteError(err error) error {
	if kind, ok := apperr.KindOf(err); ok {
		return apperr.Wrap(kind, "update_failed", err)
	}

	return apperr.Wrap(apperr.KindInternal, "update_failed", err)
}
