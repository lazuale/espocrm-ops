package app

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	domainfailure "github.com/lazuale/espocrm-ops/internal/domain/failure"
	"github.com/lazuale/espocrm-ops/internal/model"
)

type RestoreDependencies struct {
	Runtime  model.RestoreRuntime
	Store    model.RestoreStore
	Snapshot BackupService
}

type RestoreService struct {
	runtime  model.RestoreRuntime
	store    model.RestoreStore
	snapshot BackupService
}

func NewRestoreService(deps RestoreDependencies) RestoreService {
	return RestoreService{
		runtime:  deps.Runtime,
		store:    deps.Store,
		snapshot: deps.Snapshot,
	}
}

type restoreRuntimeState struct {
	appServicesWereRunning bool
	stoppedAppServices     []string
	startedDBTemporarily   bool
}

func (s RestoreService) ExecuteRestore(ctx context.Context, req model.RestoreRequest) (result model.RestoreResult, err error) {
	req = normalizeRestoreRequest(req)
	result = model.NewRestoreResult(req)
	addRestoreWarnings(&result, req)

	if failure := s.validateRestoreRequest(req); failure != nil {
		result.Fail(*failure)
		return result, *failure
	}

	source, sourceErr := s.resolveRestoreSource(ctx, req)
	if sourceErr != nil {
		result.AddStep(model.RestoreStepSourceResolution, model.StatusFailed)
		blockRestoreAfter(&result, model.RestoreStepSourceResolution, false)
		failure := restoreFailureFromError(sourceErr)
		result.Fail(failure)
		return result, failure
	}
	result.ApplySource(source)
	result.AddStep(model.RestoreStepSourceResolution, model.StatusCompleted)

	runtimeState, runtimeErr := s.prepareRestoreRuntime(ctx, req, &result)
	if runtimeErr != nil {
		result.AddStep(model.StepRuntimePrepare, model.StatusFailed)
		blockRestoreAfter(&result, model.StepRuntimePrepare, false)
		failure := model.NewRestoreFailure(model.KindExternal, "runtime prepare завершился ошибкой", runtimeErr)
		result.Fail(failure)
		return result, failure
	}
	result.AddStep(model.StepRuntimePrepare, model.StatusCompleted)
	if req.DryRun {
		result.Details.StartedDBTemporarily = runtimeState.startedDBTemporarily
		return s.buildRestoreDryRun(req, runtimeState, &result), nil
	}

	if req.NoSnapshot {
		result.AddStep(model.RestoreStepSnapshot, model.StatusSkipped)
	} else {
		snapshotResult, snapshotErr := s.snapshot.ExecuteBackup(ctx, req.Snapshot)
		if snapshotErr != nil {
			result.AddStep(model.RestoreStepSnapshot, model.StatusFailed)
			blockRestoreAfter(&result, model.RestoreStepSnapshot, false)
			failure := restoreFailureFromError(snapshotErr)
			return s.failRestoreAfterRuntimeChange(ctx, req, runtimeState, &result, failure, false)
		}
		result.ApplySnapshot(snapshotResult)
		result.AddStep(model.RestoreStepSnapshot, model.StatusCompleted)
	}

	if req.SkipDB {
		result.AddStep(model.RestoreStepDBRestore, model.StatusSkipped)
	} else if err := s.runtime.RestoreDatabase(ctx, req.Target, source.DBBackup.Path); err != nil {
		result.AddStep(model.RestoreStepDBRestore, model.StatusFailed)
		blockRestoreAfter(&result, model.RestoreStepDBRestore, true)
		failure := model.NewRestoreFailure(model.KindExternal, "database restore завершился ошибкой", err)
		return s.failRestoreAfterRuntimeChange(ctx, req, runtimeState, &result, failure, true)
	} else {
		result.AddStep(model.RestoreStepDBRestore, model.StatusCompleted)
	}

	if req.SkipFiles {
		result.AddStep(model.RestoreStepFilesRestore, model.StatusSkipped)
		result.AddStep(model.RestoreStepPermission, model.StatusSkipped)
	} else if err := s.store.RestoreFilesArtifact(ctx, source.FilesBackup.Path, req.StorageDir, source.DirectFilesExact); err != nil {
		result.AddStep(model.RestoreStepFilesRestore, model.StatusFailed)
		blockRestoreAfter(&result, model.RestoreStepFilesRestore, true)
		failure := model.NewRestoreFailure(model.KindIO, "files restore завершился ошибкой", err)
		return s.failRestoreAfterRuntimeChange(ctx, req, runtimeState, &result, failure, true)
	} else {
		result.AddStep(model.RestoreStepFilesRestore, model.StatusCompleted)
		if err := s.runtime.ReconcileFilesPermissions(ctx, req.Target); err != nil {
			result.AddStep(model.RestoreStepPermission, model.StatusFailed)
			blockRestoreAfter(&result, model.RestoreStepPermission, false)
			failure := model.NewRestoreFailure(model.KindExternal, "permission reconciliation завершился ошибкой", err)
			return s.failRestoreAfterRuntimeChange(ctx, req, runtimeState, &result, failure, false)
		}
		result.AddStep(model.RestoreStepPermission, model.StatusCompleted)
	}

	if returnErr := s.returnRestoreRuntime(ctx, req, runtimeState, &result); returnErr != nil {
		result.AddStep(model.StepRuntimeReturn, model.StatusFailed)
		blockRestoreAfter(&result, model.StepRuntimeReturn, false)
		failure := model.NewRestoreFailure(model.KindExternal, "runtime return завершился ошибкой", returnErr)
		result.Fail(failure)
		return result, failure
	}

	if checkErr := s.postRestoreCheck(ctx, req, runtimeState); checkErr != nil {
		result.AddStep(model.RestoreStepPostCheck, model.StatusFailed)
		failure := model.NewRestoreFailure(model.KindExternal, "post-restore check завершился ошибкой", checkErr)
		result.Fail(failure)
		return result, failure
	}
	result.AddStep(model.RestoreStepPostCheck, model.StatusCompleted)

	result.Succeed()
	return result, nil
}

func (s RestoreService) validateRestoreRequest(req model.RestoreRequest) *model.BackupFailure {
	if s.store == nil {
		failure := model.NewRestoreFailure(model.KindValidation, "store не настроен", nil)
		return &failure
	}
	if s.runtime == nil {
		failure := model.NewRestoreFailure(model.KindValidation, "runtime не настроен", nil)
		return &failure
	}
	if !req.NoSnapshot && s.snapshot.store == nil {
		failure := model.NewRestoreFailure(model.KindValidation, "сервис аварийного snapshot не настроен", nil)
		return &failure
	}
	if strings.TrimSpace(req.Scope) == "" {
		failure := model.NewRestoreFailure(model.KindUsage, "scope обязателен", nil)
		return &failure
	}
	if strings.TrimSpace(req.StorageDir) == "" {
		failure := model.NewRestoreFailure(model.KindValidation, "storage dir обязателен", nil)
		return &failure
	}
	if req.SkipDB && req.SkipFiles {
		failure := model.NewRestoreFailure(model.KindUsage, "нельзя одновременно пропустить database и files restore", nil)
		return &failure
	}

	hasManifest := strings.TrimSpace(req.Manifest) != ""
	hasDB := strings.TrimSpace(req.DBBackup) != ""
	hasFiles := strings.TrimSpace(req.FilesBackup) != ""
	if hasManifest && (hasDB || hasFiles) {
		failure := model.NewRestoreFailure(model.KindUsage, "нужно выбрать manifest или прямые artifacts, но не оба источника", nil)
		return &failure
	}
	if hasManifest && (req.SkipDB || req.SkipFiles) {
		failure := model.NewRestoreFailure(model.KindValidation, "частичный restore через manifest не является v2 contract", nil)
		return &failure
	}

	switch {
	case hasManifest:
		return nil
	case req.SkipDB:
		if !hasFiles || hasDB {
			failure := model.NewRestoreFailure(model.KindUsage, "files-only restore требует только direct files artifact", nil)
			return &failure
		}
	case req.SkipFiles:
		if !hasDB || hasFiles {
			failure := model.NewRestoreFailure(model.KindUsage, "db-only restore требует только direct DB artifact", nil)
			return &failure
		}
	default:
		if !hasDB || !hasFiles {
			failure := model.NewRestoreFailure(model.KindUsage, "full direct restore требует DB и files artifacts", nil)
			return &failure
		}
	}
	return nil
}

func (s RestoreService) resolveRestoreSource(ctx context.Context, req model.RestoreRequest) (model.RestoreSource, error) {
	if req.Manifest != "" {
		manifest, err := s.store.LoadRestoreManifest(ctx, req.Manifest)
		if err != nil {
			return model.RestoreSource{}, err
		}
		dbPath := manifest.DBArtifactPath(req.Manifest)
		dbArtifact, err := s.store.VerifyDBArtifact(ctx, dbPath, manifest.Checksums.DBBackup)
		if err != nil {
			return model.RestoreSource{}, err
		}
		filesPath := manifest.FilesArtifactPath(req.Manifest)
		filesArtifact, err := s.store.VerifyFilesArtifact(ctx, filesPath, manifest.Checksums.FilesBackup)
		if err != nil {
			return model.RestoreSource{}, err
		}
		return model.RestoreSource{
			SelectionMode: model.RestoreSelectionManifest,
			SourceKind:    model.RestoreSourceManifest,
			ManifestPath:  req.Manifest,
			DBBackup:      dbArtifact,
			FilesBackup:   filesArtifact,
		}, nil
	}

	switch {
	case req.SkipDB:
		filesArtifact, err := s.store.VerifyFilesArtifact(ctx, req.FilesBackup, "")
		if err != nil {
			return model.RestoreSource{}, err
		}
		return model.RestoreSource{
			SelectionMode:    model.RestoreSelectionDirectFiles,
			SourceKind:       model.RestoreSourceDirect,
			FilesBackup:      filesArtifact,
			DirectFilesExact: true,
		}, nil
	case req.SkipFiles:
		dbArtifact, err := s.store.VerifyDBArtifact(ctx, req.DBBackup, "")
		if err != nil {
			return model.RestoreSource{}, err
		}
		return model.RestoreSource{
			SelectionMode: model.RestoreSelectionDirectDB,
			SourceKind:    model.RestoreSourceDirect,
			DBBackup:      dbArtifact,
		}, nil
	default:
		dbArtifact, err := s.store.VerifyDBArtifact(ctx, req.DBBackup, "")
		if err != nil {
			return model.RestoreSource{}, err
		}
		filesArtifact, err := s.store.VerifyFilesArtifact(ctx, req.FilesBackup, "")
		if err != nil {
			return model.RestoreSource{}, err
		}
		if err := validateRestoreDirectPair(dbArtifact.Path, filesArtifact.Path); err != nil {
			return model.RestoreSource{}, err
		}
		return model.RestoreSource{
			SelectionMode:    model.RestoreSelectionDirectPair,
			SourceKind:       model.RestoreSourceDirect,
			DBBackup:         dbArtifact,
			FilesBackup:      filesArtifact,
			DirectFilesExact: true,
		}, nil
	}
}

func (s RestoreService) prepareRestoreRuntime(ctx context.Context, req model.RestoreRequest, result *model.RestoreResult) (restoreRuntimeState, error) {
	running, err := s.runtime.RunningServices(ctx, req.Target)
	if err != nil {
		return restoreRuntimeState{}, err
	}
	state := restoreRuntimeState{
		stoppedAppServices: model.RunningApplicationServices(running),
	}
	state.appServicesWereRunning = len(state.stoppedAppServices) != 0
	state.startedDBTemporarily = !req.SkipDB && !restoreServiceRunning(running, "db")
	result.Details.AppServicesWereRunning = state.appServicesWereRunning
	result.Details.StartedDBTemporarily = state.startedDBTemporarily

	if req.DryRun || req.NoStop || len(state.stoppedAppServices) == 0 {
		return state, nil
	}
	if err := s.runtime.StopServices(ctx, req.Target, state.stoppedAppServices...); err != nil {
		return state, err
	}
	return state, nil
}

func (s RestoreService) buildRestoreDryRun(req model.RestoreRequest, state restoreRuntimeState, result *model.RestoreResult) model.RestoreResult {
	if req.NoSnapshot {
		result.AddStep(model.RestoreStepSnapshot, model.StatusSkipped)
	} else {
		result.AddStep(model.RestoreStepSnapshot, model.StatusPlanned)
	}

	if req.SkipDB {
		result.AddStep(model.RestoreStepDBRestore, model.StatusSkipped)
	} else {
		result.AddStep(model.RestoreStepDBRestore, model.StatusPlanned)
	}

	if req.SkipFiles {
		result.AddStep(model.RestoreStepFilesRestore, model.StatusSkipped)
		result.AddStep(model.RestoreStepPermission, model.StatusSkipped)
	} else {
		result.AddStep(model.RestoreStepFilesRestore, model.StatusPlanned)
		result.AddStep(model.RestoreStepPermission, model.StatusPlanned)
	}

	result.AddStep(model.StepRuntimeReturn, dryRunRestoreRuntimeReturnStatus(req, state))
	result.Succeed()
	return *result
}

func (s RestoreService) returnRestoreRuntime(ctx context.Context, req model.RestoreRequest, state restoreRuntimeState, result *model.RestoreResult) error {
	if len(state.stoppedAppServices) == 0 {
		result.AddStep(model.StepRuntimeReturn, model.StatusSkipped)
		return nil
	}
	if req.NoStart {
		result.AddStep(model.StepRuntimeReturn, model.StatusSkipped)
		return nil
	}
	if err := s.runtime.StartServices(ctx, req.Target, state.stoppedAppServices...); err != nil {
		return err
	}
	result.AddStep(model.StepRuntimeReturn, model.StatusCompleted)
	return nil
}

func (s RestoreService) failRestoreAfterRuntimeChange(ctx context.Context, req model.RestoreRequest, state restoreRuntimeState, result *model.RestoreResult, failure model.BackupFailure, allowRuntimeReturn bool) (model.RestoreResult, error) {
	if allowRuntimeReturn {
		if returnErr := s.returnRestoreRuntime(ctx, req, state, result); returnErr != nil {
			result.AddStep(model.StepRuntimeReturn, model.StatusFailed)
			failure = model.NewRestoreFailure(model.KindExternal, "runtime return после ошибки restore завершился ошибкой", errors.Join(failure, returnErr))
		}
	}
	result.Fail(failure)
	return *result, failure
}

func (s RestoreService) postRestoreCheck(ctx context.Context, req model.RestoreRequest, state restoreRuntimeState) error {
	services := expectedRestoreServices(req, state)
	return s.runtime.PostRestoreCheck(ctx, req.Target, services...)
}

func expectedRestoreServices(req model.RestoreRequest, state restoreRuntimeState) []string {
	var services []string
	if req.NoStop {
		services = append(services, state.stoppedAppServices...)
	} else if !req.NoStart {
		services = append(services, state.stoppedAppServices...)
	}
	if !req.SkipDB {
		services = append(services, "db")
	}

	seen := map[string]struct{}{}
	out := make([]string, 0, len(services))
	for _, service := range services {
		service = strings.TrimSpace(service)
		if service == "" {
			continue
		}
		if _, ok := seen[service]; ok {
			continue
		}
		seen[service] = struct{}{}
		out = append(out, service)
	}
	return out
}

func dryRunRestoreRuntimeReturnStatus(req model.RestoreRequest, state restoreRuntimeState) string {
	switch {
	case req.NoStop && len(state.stoppedAppServices) != 0:
		return model.StatusSkipped
	case len(state.stoppedAppServices) == 0 && !state.startedDBTemporarily:
		return model.StatusSkipped
	case len(state.stoppedAppServices) != 0 && req.NoStart:
		return model.StatusSkipped
	default:
		return model.StatusPlanned
	}
}

func restoreServiceRunning(running []string, want string) bool {
	want = strings.TrimSpace(want)
	if want == "" {
		return false
	}
	for _, service := range running {
		if strings.TrimSpace(service) == want {
			return true
		}
	}
	return false
}

func validateRestoreDirectPair(dbPath, filesPath string) error {
	dbGroup, dbOK := model.ParseBackupGroupName(filepath.Base(dbPath))
	filesGroup, filesOK := model.ParseBackupGroupName(filepath.Base(filesPath))
	if !dbOK {
		return model.BackupVerifyArtifactError{Label: "db backup", Err: fmt.Errorf("имя DB artifact не canonical")}
	}
	if !filesOK {
		return model.BackupVerifyArtifactError{Label: "files backup", Err: fmt.Errorf("имя files artifact не canonical")}
	}
	if dbGroup != filesGroup {
		return model.BackupVerifyArtifactError{Label: "direct pair", Err: fmt.Errorf("DB и files artifacts относятся к разным backup-set")}
	}
	return nil
}

func restoreFailureFromError(err error) model.BackupFailure {
	var failure model.BackupFailure
	if errors.As(err, &failure) {
		return model.NewRestoreFailure(failure.Kind, failure.Message, err)
	}
	var domainFailure domainfailure.Failure
	if errors.As(err, &domainFailure) {
		return model.NewRestoreFailure(modelKindFromDomain(domainFailure.Kind), "restore завершился ошибкой", err)
	}
	var manifestErr model.BackupVerifyManifestError
	if errors.As(err, &manifestErr) {
		return model.NewCommandFailure(model.KindManifest, model.ManifestInvalidCode, "manifest restore source не прошёл проверку", err)
	}
	var artifactErr model.BackupVerifyArtifactError
	if errors.As(err, &artifactErr) {
		return model.NewRestoreFailure(model.KindValidation, "restore artifact не прошёл проверку", err)
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return model.NewRestoreFailure(model.KindExternal, "restore прерван", err)
	}
	return model.NewRestoreFailure(model.KindIO, "restore завершился ошибкой", err)
}

func blockRestoreAfter(result *model.RestoreResult, failedStep string, allowRuntimeReturn bool) {
	steps := []string{
		model.RestoreStepSourceResolution,
		model.StepRuntimePrepare,
		model.RestoreStepSnapshot,
		model.RestoreStepDBRestore,
		model.RestoreStepFilesRestore,
		model.RestoreStepPermission,
		model.StepRuntimeReturn,
		model.RestoreStepPostCheck,
	}
	seen := false
	for _, step := range steps {
		if step == failedStep {
			seen = true
			continue
		}
		if !seen || restoreHasStep(*result, step) {
			continue
		}
		if allowRuntimeReturn && step == model.StepRuntimeReturn {
			continue
		}
		result.AddStep(step, model.StatusBlocked)
	}
}

func restoreHasStep(result model.RestoreResult, code string) bool {
	for _, step := range result.Items {
		if step.Code == code {
			return true
		}
	}
	return false
}

func normalizeRestoreRequest(req model.RestoreRequest) model.RestoreRequest {
	req.Scope = strings.TrimSpace(req.Scope)
	req.ProjectDir = strings.TrimSpace(req.ProjectDir)
	req.ComposeFile = strings.TrimSpace(req.ComposeFile)
	req.EnvFile = strings.TrimSpace(req.EnvFile)
	req.BackupRoot = strings.TrimSpace(req.BackupRoot)
	req.StorageDir = strings.TrimSpace(req.StorageDir)
	req.Manifest = strings.TrimSpace(req.Manifest)
	req.DBBackup = strings.TrimSpace(req.DBBackup)
	req.FilesBackup = strings.TrimSpace(req.FilesBackup)
	if req.Target.ProjectDir == "" {
		req.Target.ProjectDir = req.ProjectDir
	}
	if req.Target.ComposeFile == "" {
		req.Target.ComposeFile = req.ComposeFile
	}
	if req.Target.EnvFile == "" {
		req.Target.EnvFile = req.EnvFile
	}
	if req.Target.StorageDir == "" {
		req.Target.StorageDir = req.StorageDir
	}
	return req
}

func addRestoreWarnings(result *model.RestoreResult, req model.RestoreRequest) {
	if req.NoSnapshot {
		result.AddWarning("Restore выполняется без аварийного snapshot.")
	}
	if req.NoStop {
		result.AddWarning("Restore выполняется без остановки application services.")
	}
	if req.NoStart {
		result.AddWarning("Application services останутся остановленными после restore.")
	}
	if req.SkipDB {
		result.AddWarning("Database restore пропущен.")
	}
	if req.SkipFiles {
		result.AddWarning("Files restore пропущен.")
	}
}
