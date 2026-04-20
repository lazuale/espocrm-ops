package migrate

import (
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/lazuale/espocrm-ops/internal/contract/apperr"
	platformconfig "github.com/lazuale/espocrm-ops/internal/platform/config"
	platformdocker "github.com/lazuale/espocrm-ops/internal/platform/docker"
	maintenanceusecase "github.com/lazuale/espocrm-ops/internal/usecase/maintenance"
	restoreusecase "github.com/lazuale/espocrm-ops/internal/usecase/restore"
)

const (
	MigrateStepStatusCompleted = "completed"
	MigrateStepStatusSkipped   = "skipped"
	MigrateStepStatusFailed    = "failed"
	MigrateStepStatusNotRun    = "not_run"

	defaultReadinessTimeoutSeconds = 300
)

var migrationAppServices = []string{
	"espocrm",
	"espocrm-daemon",
	"espocrm-websocket",
}

type ExecuteRequest struct {
	SourceScope string
	TargetScope string
	ProjectDir  string
	ComposeFile string
	DBBackup    string
	FilesBackup string
	SkipDB      bool
	SkipFiles   bool
	NoStart     bool
	LogWriter   io.Writer
}

type ExecuteStep struct {
	Code    string
	Status  string
	Summary string
	Details string
	Action  string
}

type ExecuteInfo struct {
	SourceScope            string
	TargetScope            string
	ProjectDir             string
	ComposeFile            string
	SourceEnvFile          string
	TargetEnvFile          string
	SourceBackupRoot       string
	TargetBackupRoot       string
	RequestedDBBackup      string
	RequestedFilesBackup   string
	RequestedSelectionMode string
	SelectionMode          string
	SelectedPrefix         string
	SelectedStamp          string
	ManifestTXTPath        string
	ManifestJSONPath       string
	DBBackupPath           string
	FilesBackupPath        string
	SkipDB                 bool
	SkipFiles              bool
	NoStart                bool
	StartedDBTemporarily   bool
	Warnings               []string
	Steps                  []ExecuteStep
}

type runtimePrepareInfo struct {
	StartedDBTemporarily bool
	StoppedAppServices   []string
}

type sourceSelection struct {
	SelectionMode string
	Prefix        string
	Stamp         string
	ManifestTXT   string
	ManifestJSON  string
	DBBackup      string
	FilesBackup   string
	Warnings      []string
}

type executeFailure struct {
	Summary string
	Action  string
	Err     error
}

func (e executeFailure) Error() string {
	if e.Err == nil {
		return ""
	}
	return e.Err.Error()
}

func (e executeFailure) Unwrap() error {
	return e.Err
}

func Execute(req ExecuteRequest) (ExecuteInfo, error) {
	info := ExecuteInfo{
		SourceScope:            strings.TrimSpace(req.SourceScope),
		TargetScope:            strings.TrimSpace(req.TargetScope),
		ProjectDir:             filepath.Clean(req.ProjectDir),
		ComposeFile:            filepath.Clean(req.ComposeFile),
		RequestedDBBackup:      strings.TrimSpace(req.DBBackup),
		RequestedFilesBackup:   strings.TrimSpace(req.FilesBackup),
		RequestedSelectionMode: requestedSelectionMode(req),
		SkipDB:                 req.SkipDB,
		SkipFiles:              req.SkipFiles,
		NoStart:                req.NoStart,
		Warnings:               flagWarnings(req),
	}

	sourceEnv, err := platformconfig.LoadOperationEnv(info.ProjectDir, info.SourceScope, "")
	if err != nil {
		info.Steps = append(info.Steps,
			ExecuteStep{
				Code:    "source_preflight",
				Status:  MigrateStepStatusFailed,
				Summary: "Source contour preflight failed",
				Details: err.Error(),
				Action:  "Resolve the source env file or source backup root settings before rerunning migrate.",
			},
			notRunMigrateStep("target_preflight", "Target contour preflight did not run because source contour preflight failed"),
			notRunMigrateStep("source_selection", "Source backup selection did not run because source contour preflight failed"),
			notRunMigrateStep("compatibility", "Migration compatibility checks did not run because source contour preflight failed"),
			notRunMigrateStep("runtime_prepare", "Target runtime preparation did not run because source contour preflight failed"),
			notRunMigrateStep("db_restore", "Database restore did not run because source contour preflight failed"),
			notRunMigrateStep("files_restore", "Files restore did not run because source contour preflight failed"),
			notRunMigrateStep("target_start", "Target contour start did not run because source contour preflight failed"),
		)
		return info, wrapMigrationEnvError(err)
	}

	info.SourceEnvFile = sourceEnv.FilePath
	info.SourceBackupRoot = platformconfig.ResolveProjectPath(info.ProjectDir, sourceEnv.BackupRoot())
	info.Steps = append(info.Steps, ExecuteStep{
		Code:    "source_preflight",
		Status:  MigrateStepStatusCompleted,
		Summary: "Source contour preflight completed",
		Details: fmt.Sprintf("Using %s with backup root %s.", info.SourceEnvFile, info.SourceBackupRoot),
	})

	targetCtx, err := maintenanceusecase.PrepareOperation(maintenanceusecase.OperationContextRequest{
		Scope:      info.TargetScope,
		Operation:  "migrate",
		ProjectDir: info.ProjectDir,
		LogWriter:  req.LogWriter,
	})
	if err != nil {
		info.Steps = append(info.Steps,
			ExecuteStep{
				Code:    "target_preflight",
				Status:  MigrateStepStatusFailed,
				Summary: "Target contour preflight failed",
				Details: err.Error(),
				Action:  "Resolve env, lock, or filesystem readiness before rerunning migrate.",
			},
			notRunMigrateStep("source_selection", "Source backup selection did not run because target contour preflight failed"),
			notRunMigrateStep("compatibility", "Migration compatibility checks did not run because target contour preflight failed"),
			notRunMigrateStep("runtime_prepare", "Target runtime preparation did not run because target contour preflight failed"),
			notRunMigrateStep("db_restore", "Database restore did not run because target contour preflight failed"),
			notRunMigrateStep("files_restore", "Files restore did not run because target contour preflight failed"),
			notRunMigrateStep("target_start", "Target contour start did not run because target contour preflight failed"),
		)
		return info, wrapExecuteError(err)
	}
	defer func() {
		_ = targetCtx.Release()
	}()

	info.TargetEnvFile = targetCtx.Env.FilePath
	info.TargetBackupRoot = targetCtx.BackupRoot
	info.Steps = append(info.Steps, ExecuteStep{
		Code:    "target_preflight",
		Status:  MigrateStepStatusCompleted,
		Summary: "Target contour preflight completed",
		Details: fmt.Sprintf("Using %s with backup root %s.", info.TargetEnvFile, info.TargetBackupRoot),
	})

	selection, err := resolveSourceSelection(sourceEnv, req)
	if err != nil {
		info.Steps = append(info.Steps,
			ExecuteStep{
				Code:    "source_selection",
				Status:  MigrateStepStatusFailed,
				Summary: failureSummary(err, "Source backup selection failed"),
				Details: err.Error(),
				Action:  failureAction(err, "Resolve the source backup selection error before rerunning migrate."),
			},
			notRunMigrateStep("compatibility", "Migration compatibility checks did not run because source backup selection failed"),
			notRunMigrateStep("runtime_prepare", "Target runtime preparation did not run because source backup selection failed"),
			notRunMigrateStep("db_restore", "Database restore did not run because source backup selection failed"),
			notRunMigrateStep("files_restore", "Files restore did not run because source backup selection failed"),
			notRunMigrateStep("target_start", "Target contour start did not run because source backup selection failed"),
		)
		return info, apperr.Wrap(apperr.KindValidation, "migrate_failed", err)
	}

	info.SelectionMode = selection.SelectionMode
	info.SelectedPrefix = selection.Prefix
	info.SelectedStamp = selection.Stamp
	info.ManifestTXTPath = selection.ManifestTXT
	info.ManifestJSONPath = selection.ManifestJSON
	info.DBBackupPath = selection.DBBackup
	info.FilesBackupPath = selection.FilesBackup
	info.Warnings = append(info.Warnings, selection.Warnings...)
	info.Steps = append(info.Steps, ExecuteStep{
		Code:    "source_selection",
		Status:  MigrateStepStatusCompleted,
		Summary: sourceSelectionSummary(selection),
		Details: sourceSelectionDetails(selection),
	})

	if err := requireMigrationCompatibility(sourceEnv, targetCtx.Env, info.SourceScope, info.TargetScope); err != nil {
		info.Steps = append(info.Steps,
			ExecuteStep{
				Code:    "compatibility",
				Status:  MigrateStepStatusFailed,
				Summary: failureSummary(err, "Migration compatibility contract failed"),
				Details: err.Error(),
				Action:  failureAction(err, "Align the shared settings first and rerun espops doctor --scope all --project-dir <repo>."),
			},
			notRunMigrateStep("runtime_prepare", "Target runtime preparation did not run because migration compatibility checks failed"),
			notRunMigrateStep("db_restore", "Database restore did not run because migration compatibility checks failed"),
			notRunMigrateStep("files_restore", "Files restore did not run because migration compatibility checks failed"),
			notRunMigrateStep("target_start", "Target contour start did not run because migration compatibility checks failed"),
		)
		return info, apperr.Wrap(apperr.KindValidation, "migrate_failed", err)
	}
	info.Steps = append(info.Steps, ExecuteStep{
		Code:    "compatibility",
		Status:  MigrateStepStatusCompleted,
		Summary: "Migration compatibility contract passed",
		Details: fmt.Sprintf("The source contour %s and target contour %s match on the governed migration settings.", info.SourceScope, info.TargetScope),
	})

	runtimePrep, err := prepareRuntime(info.ProjectDir, info.ComposeFile, info.TargetEnvFile)
	if err != nil {
		info.Steps = append(info.Steps,
			ExecuteStep{
				Code:    "runtime_prepare",
				Status:  MigrateStepStatusFailed,
				Summary: "Target runtime preparation failed",
				Details: err.Error(),
				Action:  "Resolve the target runtime preparation failure before rerunning migrate.",
			},
			notRunMigrateStep("db_restore", "Database restore did not run because target runtime preparation failed"),
			notRunMigrateStep("files_restore", "Files restore did not run because target runtime preparation failed"),
			notRunMigrateStep("target_start", "Target contour start did not run because target runtime preparation failed"),
		)
		return info, wrapExecuteError(err)
	}
	info.StartedDBTemporarily = runtimePrep.StartedDBTemporarily
	info.Steps = append(info.Steps, ExecuteStep{
		Code:    "runtime_prepare",
		Status:  MigrateStepStatusCompleted,
		Summary: "Target runtime preparation completed",
		Details: runtimePrepareDetails(runtimePrep),
	})

	if req.SkipDB {
		info.Steps = append(info.Steps, ExecuteStep{
			Code:    "db_restore",
			Status:  MigrateStepStatusSkipped,
			Summary: "Database restore skipped",
			Details: "The database restore was skipped because of --skip-db.",
		})
	} else {
		dbContainer, err := resolveDBContainer(info.ProjectDir, info.ComposeFile, info.TargetEnvFile)
		if err != nil {
			info.Steps = append(info.Steps,
				ExecuteStep{
					Code:    "db_restore",
					Status:  MigrateStepStatusFailed,
					Summary: "Database restore failed",
					Details: err.Error(),
					Action:  "Resolve the target db container state before rerunning migrate.",
				},
				notRunMigrateStep("files_restore", "Files restore did not run because database restore failed"),
				notRunMigrateStep("target_start", "Target contour start did not run because database restore failed"),
			)
			return info, wrapExecuteError(err)
		}

		if _, err := restoreusecase.RestoreDB(buildDBRestoreRequest(targetCtx, info, dbContainer)); err != nil {
			info.Steps = append(info.Steps,
				ExecuteStep{
					Code:    "db_restore",
					Status:  MigrateStepStatusFailed,
					Summary: "Database restore failed",
					Details: err.Error(),
					Action:  "Resolve the database restore failure before rerunning migrate.",
				},
				notRunMigrateStep("files_restore", "Files restore did not run because database restore failed"),
				notRunMigrateStep("target_start", "Target contour start did not run because database restore failed"),
			)
			return info, wrapExecuteError(err)
		}

		info.Steps = append(info.Steps, ExecuteStep{
			Code:    "db_restore",
			Status:  MigrateStepStatusCompleted,
			Summary: "Database restore completed",
			Details: dbRestoreDetails(targetCtx, info),
		})
	}

	if req.SkipFiles {
		info.Steps = append(info.Steps, ExecuteStep{
			Code:    "files_restore",
			Status:  MigrateStepStatusSkipped,
			Summary: "Files restore skipped",
			Details: "The files restore was skipped because of --skip-files.",
		})
	} else {
		filesReq := buildFilesRestoreRequest(targetCtx, info)
		if _, err := restoreusecase.RestoreFiles(filesReq); err != nil {
			info.Steps = append(info.Steps,
				ExecuteStep{
					Code:    "files_restore",
					Status:  MigrateStepStatusFailed,
					Summary: "Files restore failed",
					Details: err.Error(),
					Action:  "Resolve the files restore failure before rerunning migrate.",
				},
				notRunMigrateStep("target_start", "Target contour start did not run because files restore failed"),
			)
			return info, wrapExecuteError(err)
		}
		if err := platformdocker.ReconcileEspoStoragePermissions(
			filesReq.TargetDir,
			strings.TrimSpace(targetCtx.Env.Value("MARIADB_TAG")),
			strings.TrimSpace(targetCtx.Env.Value("ESPOCRM_IMAGE")),
		); err != nil {
			info.Steps = append(info.Steps,
				ExecuteStep{
					Code:    "files_restore",
					Status:  MigrateStepStatusFailed,
					Summary: "Files restore failed",
					Details: fmt.Sprintf("Files were restored but runtime permission reconciliation failed: %v", err),
					Action:  "Resolve the permission reconciliation failure before rerunning migrate.",
				},
				notRunMigrateStep("target_start", "Target contour start did not run because files restore failed"),
			)
			return info, wrapExternalError(err)
		}

		info.Steps = append(info.Steps, ExecuteStep{
			Code:    "files_restore",
			Status:  MigrateStepStatusCompleted,
			Summary: "Files restore completed",
			Details: filesRestoreDetails(targetCtx, info),
		})
	}

	if req.NoStart {
		info.Steps = append(info.Steps, ExecuteStep{
			Code:    "target_start",
			Status:  MigrateStepStatusSkipped,
			Summary: "Target contour start skipped",
			Details: "The target application services were left stopped because of --no-start.",
		})
	} else {
		cfg := platformdocker.ComposeConfig{
			ProjectDir:  info.ProjectDir,
			ComposeFile: info.ComposeFile,
			EnvFile:     info.TargetEnvFile,
		}
		if err := platformdocker.ComposeUp(cfg); err != nil {
			info.Steps = append(info.Steps, ExecuteStep{
				Code:    "target_start",
				Status:  MigrateStepStatusFailed,
				Summary: "Target contour start failed",
				Details: err.Error(),
				Action:  "Resolve the target contour start failure before rerunning migrate.",
			})
			return info, wrapExternalError(err)
		}
		validatedServices := expectedStartedTargetServices()
		if err := platformdocker.WaitForServicesReady(cfg, defaultReadinessTimeoutSeconds, validatedServices...); err != nil {
			info.Steps = append(info.Steps, ExecuteStep{
				Code:    "target_start",
				Status:  MigrateStepStatusFailed,
				Summary: "Target contour start failed",
				Details: err.Error(),
				Action:  "Repair the target contour runtime health before treating this migration as successful.",
			})
			return info, wrapExternalError(err)
		}

		info.Steps = append(info.Steps, ExecuteStep{
			Code:    "target_start",
			Status:  MigrateStepStatusCompleted,
			Summary: "Target contour start completed",
			Details: fmt.Sprintf("Started the target contour %s with docker compose up -d and confirmed runtime health for: %s.", info.TargetScope, strings.Join(validatedServices, ", ")),
		})
	}

	info.Warnings = dedupeStrings(info.Warnings)
	return info, nil
}

func (i ExecuteInfo) Counts() (completed, skipped, failed, notRun int) {
	for _, step := range i.Steps {
		switch step.Status {
		case MigrateStepStatusCompleted:
			completed++
		case MigrateStepStatusSkipped:
			skipped++
		case MigrateStepStatusFailed:
			failed++
		case MigrateStepStatusNotRun:
			notRun++
		}
	}

	return completed, skipped, failed, notRun
}

func (i ExecuteInfo) Ready() bool {
	for _, step := range i.Steps {
		if step.Status == MigrateStepStatusFailed {
			return false
		}
	}

	return true
}

func requireMigrationCompatibility(sourceEnv, targetEnv platformconfig.OperationEnv, sourceScope, targetScope string) error {
	rawMismatches := platformconfig.MigrationCompatibilityMismatches(sourceEnv, targetEnv)
	if len(rawMismatches) == 0 {
		return nil
	}

	mismatches := make([]string, 0, len(rawMismatches))
	for _, mismatch := range rawMismatches {
		mismatches = append(mismatches, fmt.Sprintf("%s ('%s' vs '%s')", mismatch.Name, mismatch.LeftValue, mismatch.RightValue))
	}

	return executeFailure{
		Summary: "Migration compatibility contract failed",
		Action:  "Align the shared settings first and rerun espops doctor --scope all --project-dir <repo>.",
		Err: fmt.Errorf(
			"configs %q and %q conflict with the migration compatibility contract: %s",
			sourceScope,
			targetScope,
			strings.Join(mismatches, "; "),
		),
	}
}

func prepareRuntime(projectDir, composeFile, envFile string) (runtimePrepareInfo, error) {
	info := runtimePrepareInfo{}
	cfg := platformdocker.ComposeConfig{
		ProjectDir:  projectDir,
		ComposeFile: composeFile,
		EnvFile:     envFile,
	}

	dbState, err := platformdocker.ComposeServiceStateFor(cfg, "db")
	if err != nil {
		return info, wrapExternalError(err)
	}

	if dbState.Status != "running" && dbState.Status != "healthy" {
		info.StartedDBTemporarily = true
		if err := platformdocker.ComposeUp(cfg, "db"); err != nil {
			return info, wrapExternalError(err)
		}
	}

	if err := platformdocker.WaitForServicesReady(cfg, defaultReadinessTimeoutSeconds, "db"); err != nil {
		return info, wrapExternalError(err)
	}

	runningServices, err := platformdocker.ComposeRunningServices(cfg)
	if err != nil {
		return info, wrapExternalError(err)
	}
	if migrationAppServicesRunning(runningServices) {
		info.StoppedAppServices = runningAppServices(runningServices)
		if err := platformdocker.ComposeStop(cfg, migrationAppServices...); err != nil {
			return info, wrapExternalError(err)
		}
	}

	return info, nil
}

func expectedStartedTargetServices() []string {
	return append([]string{"db"}, migrationAppServices...)
}

func resolveDBContainer(projectDir, composeFile, envFile string) (string, error) {
	cfg := platformdocker.ComposeConfig{
		ProjectDir:  projectDir,
		ComposeFile: composeFile,
		EnvFile:     envFile,
	}

	container, err := platformdocker.ComposeServiceContainerID(cfg, "db")
	if err != nil {
		return "", wrapExternalError(err)
	}
	if strings.TrimSpace(container) == "" {
		return "", wrapExternalError(fmt.Errorf("could not resolve the db container after target runtime preparation"))
	}

	return container, nil
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

func sourceSelectionSummary(selection sourceSelection) string {
	switch selection.SelectionMode {
	case "explicit_pair":
		return "Explicit source backup pair selection completed"
	case "paired_from_db":
		return "Source backup pairing from the explicit database backup completed"
	case "paired_from_files":
		return "Source backup pairing from the explicit files backup completed"
	case "explicit_db_only":
		return "Explicit database backup selection completed"
	case "explicit_files_only":
		return "Explicit files backup selection completed"
	case "auto_latest_db":
		return "Automatic database backup selection completed"
	case "auto_latest_files":
		return "Automatic files backup selection completed"
	default:
		return "Automatic source backup selection completed"
	}
}

func sourceSelectionDetails(selection sourceSelection) string {
	switch selection.SelectionMode {
	case "explicit_db_only", "auto_latest_db":
		return fmt.Sprintf("Selected database backup %s.", selection.DBBackup)
	case "explicit_files_only", "auto_latest_files":
		return fmt.Sprintf("Selected files backup %s.", selection.FilesBackup)
	case "explicit_pair", "paired_from_db", "paired_from_files":
		details := fmt.Sprintf("Selected DB backup %s and files backup %s.", selection.DBBackup, selection.FilesBackup)
		if strings.TrimSpace(selection.ManifestJSON) != "" {
			details += fmt.Sprintf(" Matching manifest %s is available for the same backup set.", selection.ManifestJSON)
		}
		return details
	default:
		details := fmt.Sprintf("Selected prefix %s at %s with DB backup %s and files backup %s.", selection.Prefix, selection.Stamp, selection.DBBackup, selection.FilesBackup)
		if strings.TrimSpace(selection.ManifestJSON) != "" {
			details += fmt.Sprintf(" Matching manifest %s is available for the same backup set.", selection.ManifestJSON)
		}
		return details
	}
}

func runtimePrepareDetails(info runtimePrepareInfo) string {
	dbMode := "The target db service was already running and ready."
	if info.StartedDBTemporarily {
		dbMode = "The target db service was started temporarily and confirmed ready."
	}
	appMode := "Target application services were already stopped."
	if len(info.StoppedAppServices) != 0 {
		appMode = fmt.Sprintf("Stopped the target application services: %s.", strings.Join(info.StoppedAppServices, ", "))
	}

	return dbMode + " " + appMode
}

func dbRestoreDetails(ctx maintenanceusecase.OperationContext, info ExecuteInfo) string {
	return fmt.Sprintf("Restored database %s for target contour %s from %s.", strings.TrimSpace(ctx.Env.Value("DB_NAME")), info.TargetScope, migrateSourceLabel(info))
}

func filesRestoreDetails(ctx maintenanceusecase.OperationContext, info ExecuteInfo) string {
	targetDir := platformconfig.ResolveProjectPath(ctx.ProjectDir, ctx.Env.ESPOStorageDir())
	return fmt.Sprintf("Restored %s for target contour %s from %s.", targetDir, info.TargetScope, migrateSourceLabel(info))
}

func migrateSourceLabel(info ExecuteInfo) string {
	if strings.TrimSpace(info.ManifestJSONPath) != "" {
		return info.ManifestJSONPath
	}
	switch {
	case strings.TrimSpace(info.DBBackupPath) != "" && strings.TrimSpace(info.FilesBackupPath) != "":
		return fmt.Sprintf("%s and %s", info.DBBackupPath, info.FilesBackupPath)
	case strings.TrimSpace(info.DBBackupPath) != "":
		return info.DBBackupPath
	default:
		return info.FilesBackupPath
	}
}

func requestedSelectionMode(req ExecuteRequest) string {
	dbBackup := strings.TrimSpace(req.DBBackup)
	filesBackup := strings.TrimSpace(req.FilesBackup)

	switch {
	case req.SkipDB:
		if filesBackup != "" {
			return "explicit_files_only"
		}
		return "auto_latest_files"
	case req.SkipFiles:
		if dbBackup != "" {
			return "explicit_db_only"
		}
		return "auto_latest_db"
	case dbBackup == "" && filesBackup == "":
		return "auto_latest_complete"
	case dbBackup != "" && filesBackup != "":
		return "explicit_pair"
	case dbBackup != "":
		return "paired_from_db"
	default:
		return "paired_from_files"
	}
}

func flagWarnings(req ExecuteRequest) []string {
	warnings := []string{}
	if req.SkipDB {
		warnings = append(warnings, "Backup migration will skip the database restore because of --skip-db.")
	}
	if req.SkipFiles {
		warnings = append(warnings, "Backup migration will skip the files restore because of --skip-files.")
	}
	if req.NoStart {
		warnings = append(warnings, "Backup migration will leave the target application services stopped because of --no-start.")
	}

	return warnings
}

func migrationAppServicesRunning(services []string) bool {
	for _, service := range services {
		for _, appService := range migrationAppServices {
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

	items := make([]string, 0, len(migrationAppServices))
	for _, service := range migrationAppServices {
		if _, ok := set[service]; ok {
			items = append(items, service)
		}
	}

	return items
}

func notRunMigrateStep(code, summary string) ExecuteStep {
	return ExecuteStep{
		Code:    code,
		Status:  MigrateStepStatusNotRun,
		Summary: summary,
	}
}

func failureSummary(err error, fallback string) string {
	var failure executeFailure
	if errors.As(err, &failure) && strings.TrimSpace(failure.Summary) != "" {
		return failure.Summary
	}

	return fallback
}

func failureAction(err error, fallback string) string {
	var failure executeFailure
	if errors.As(err, &failure) && strings.TrimSpace(failure.Action) != "" {
		return failure.Action
	}

	return fallback
}

func wrapMigrationEnvError(err error) error {
	switch err.(type) {
	case platformconfig.MissingEnvFileError, platformconfig.InvalidEnvFileError, platformconfig.EnvParseError, platformconfig.MissingEnvValueError, platformconfig.UnsupportedContourError:
		return apperr.Wrap(apperr.KindValidation, "migrate_failed", err)
	default:
		return apperr.Wrap(apperr.KindIO, "migrate_failed", err)
	}
}

func wrapExecuteError(err error) error {
	if kind, ok := apperr.KindOf(err); ok {
		return apperr.Wrap(kind, "migrate_failed", err)
	}

	return apperr.Wrap(apperr.KindInternal, "migrate_failed", err)
}

func wrapExternalError(err error) error {
	return apperr.Wrap(apperr.KindExternal, "migrate_failed", err)
}
