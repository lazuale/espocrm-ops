package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	domainfailure "github.com/lazuale/espocrm-ops/internal/domain/failure"
	"github.com/lazuale/espocrm-ops/internal/model"
)

type MigrateDependencies struct {
	Runtime  model.MigrateRuntime
	Store    model.MigrateStore
	Snapshot BackupService
}

type MigrateService struct {
	runtime  model.MigrateRuntime
	store    model.MigrateStore
	snapshot BackupService
}

type migrateRuntimeState struct {
	appServicesWereRunning bool
	startedDBTemporarily   bool
	stoppedAppServices     []string
}

func NewMigrateService(deps MigrateDependencies) MigrateService {
	return MigrateService{
		runtime:  deps.Runtime,
		store:    deps.Store,
		snapshot: deps.Snapshot,
	}
}

func (s MigrateService) ExecuteMigrate(ctx context.Context, req model.MigrateRequest) (result model.MigrateResult, err error) {
	req = normalizeMigrateRequest(req)
	result = model.NewMigrateResult(req)
	addMigrateWarnings(&result, req)

	if failure := s.validateMigrateRequest(req); failure != nil {
		result.Fail(*failure)
		return result, *failure
	}

	source, sourceErr := s.resolveMigrateSource(ctx, req)
	if sourceErr != nil {
		result.AddStep(model.MigrateStepSourceSelection, model.StatusFailed)
		blockMigrateAfter(&result, model.MigrateStepSourceSelection)
		failure := migrateFailureFromError(sourceErr)
		result.Fail(failure)
		return result, failure
	}
	result.ApplySource(source)
	result.AddStep(model.MigrateStepSourceSelection, model.StatusCompleted)

	if compatibilityErr := validateMigrateCompatibility(req); compatibilityErr != nil {
		result.AddStep(model.MigrateStepCompatibility, model.StatusFailed)
		blockMigrateAfter(&result, model.MigrateStepCompatibility)
		failure := migrateFailureFromError(compatibilityErr)
		result.Fail(failure)
		return result, failure
	}
	result.AddStep(model.MigrateStepCompatibility, model.StatusCompleted)

	snapshotResult, snapshotErr := s.snapshot.ExecuteBackup(ctx, req.Snapshot)
	if snapshotErr != nil {
		result.AddStep(model.MigrateStepTargetSnapshot, model.StatusFailed)
		blockMigrateAfter(&result, model.MigrateStepTargetSnapshot)
		failure := migrateFailureFromError(snapshotErr)
		result.Fail(failure)
		return result, failure
	}
	result.ApplySnapshot(snapshotResult)
	result.AddStep(model.MigrateStepTargetSnapshot, model.StatusCompleted)

	_, runtimeErr := s.prepareMigrateRuntime(ctx, req, &result)
	if runtimeErr != nil {
		result.AddStep(model.StepRuntimePrepare, model.StatusFailed)
		blockMigrateAfter(&result, model.StepRuntimePrepare)
		failure := model.NewMigrateFailure(model.KindExternal, "runtime prepare завершился ошибкой", runtimeErr)
		result.Fail(failure)
		return result, failure
	}
	result.AddStep(model.StepRuntimePrepare, model.StatusCompleted)

	if req.SkipDB {
		result.AddStep(model.MigrateStepDBApply, model.StatusSkipped)
	} else if err := s.runtime.RestoreDatabase(ctx, req.Target, source.DBBackup.Path); err != nil {
		result.AddStep(model.MigrateStepDBApply, model.StatusFailed)
		blockMigrateAfter(&result, model.MigrateStepDBApply)
		failure := model.NewMigrateFailure(model.KindExternal, "database migrate завершился ошибкой", err)
		result.Fail(failure)
		return result, failure
	} else {
		result.AddStep(model.MigrateStepDBApply, model.StatusCompleted)
	}

	if req.SkipFiles {
		result.AddStep(model.MigrateStepFilesApply, model.StatusSkipped)
		result.AddStep(model.MigrateStepPermission, model.StatusSkipped)
	} else if err := s.store.RestoreFilesArtifact(ctx, source.FilesBackup.Path, req.StorageDir, source.DirectFilesExact); err != nil {
		result.AddStep(model.MigrateStepFilesApply, model.StatusFailed)
		blockMigrateAfter(&result, model.MigrateStepFilesApply)
		failure := model.NewMigrateFailure(model.KindIO, "files migrate завершился ошибкой", err)
		result.Fail(failure)
		return result, failure
	} else {
		result.AddStep(model.MigrateStepFilesApply, model.StatusCompleted)
		if err := s.runtime.ReconcileFilesPermissions(ctx, req.Target); err != nil {
			result.AddStep(model.MigrateStepPermission, model.StatusFailed)
			blockMigrateAfter(&result, model.MigrateStepPermission)
			failure := model.NewMigrateFailure(model.KindExternal, "permission reconciliation завершился ошибкой", err)
			result.Fail(failure)
			return result, failure
		}
		result.AddStep(model.MigrateStepPermission, model.StatusCompleted)
	}

	returnStatus, returnErr := s.returnMigrateRuntime(ctx, req)
	if returnErr != nil {
		result.AddStep(model.StepRuntimeReturn, model.StatusFailed)
		blockMigrateAfter(&result, model.StepRuntimeReturn)
		failure := model.NewMigrateFailure(model.KindExternal, "runtime return завершился ошибкой", returnErr)
		result.Fail(failure)
		return result, failure
	}
	result.AddStep(model.StepRuntimeReturn, returnStatus)

	if postCheckErr := s.postMigrateCheck(ctx, req); postCheckErr != nil {
		result.AddStep(model.MigrateStepPostCheck, model.StatusFailed)
		failure := model.NewMigrateFailure(model.KindExternal, "post-migrate check завершился ошибкой", postCheckErr)
		result.Fail(failure)
		return result, failure
	}
	result.AddStep(model.MigrateStepPostCheck, model.StatusCompleted)

	result.Succeed()
	return result, nil
}

func (s MigrateService) validateMigrateRequest(req model.MigrateRequest) *model.BackupFailure {
	if s.store == nil {
		failure := model.NewMigrateFailure(model.KindValidation, "store не настроен", nil)
		return &failure
	}
	if s.runtime == nil {
		failure := model.NewMigrateFailure(model.KindValidation, "runtime не настроен", nil)
		return &failure
	}
	if s.snapshot.store == nil {
		failure := model.NewMigrateFailure(model.KindValidation, "сервис target snapshot не настроен", nil)
		return &failure
	}
	if strings.TrimSpace(req.SourceScope) == "" || strings.TrimSpace(req.TargetScope) == "" {
		failure := model.NewMigrateFailure(model.KindUsage, "source и target scope обязательны", nil)
		return &failure
	}
	if req.SourceScope == req.TargetScope {
		failure := model.NewMigrateFailure(model.KindUsage, "source и target scope должны различаться", nil)
		return &failure
	}
	if strings.TrimSpace(req.SourceBackupRoot) == "" {
		failure := model.NewMigrateFailure(model.KindValidation, "source backup root обязателен", nil)
		return &failure
	}
	if strings.TrimSpace(req.TargetBackupRoot) == "" {
		failure := model.NewMigrateFailure(model.KindValidation, "target backup root обязателен", nil)
		return &failure
	}
	if strings.TrimSpace(req.StorageDir) == "" {
		failure := model.NewMigrateFailure(model.KindValidation, "target storage dir обязателен", nil)
		return &failure
	}
	if req.SkipDB && req.SkipFiles {
		failure := model.NewMigrateFailure(model.KindUsage, "нельзя одновременно пропустить database и files migrate", nil)
		return &failure
	}

	hasDB := strings.TrimSpace(req.DBBackup) != ""
	hasFiles := strings.TrimSpace(req.FilesBackup) != ""
	switch {
	case req.SkipDB:
		if !hasFiles || hasDB {
			failure := model.NewMigrateFailure(model.KindUsage, "files-only migrate требует только direct files artifact", nil)
			return &failure
		}
	case req.SkipFiles:
		if !hasDB || hasFiles {
			failure := model.NewMigrateFailure(model.KindUsage, "db-only migrate требует только direct DB artifact", nil)
			return &failure
		}
	}
	return nil
}

func (s MigrateService) resolveMigrateSource(ctx context.Context, req model.MigrateRequest) (model.MigrateSource, error) {
	switch {
	case req.SkipDB:
		return s.resolveMigrateFilesOnly(ctx, req)
	case req.SkipFiles:
		return s.resolveMigrateDBOnly(ctx, req)
	}

	dbPath := strings.TrimSpace(req.DBBackup)
	filesPath := strings.TrimSpace(req.FilesBackup)
	switch {
	case dbPath == "" && filesPath == "":
		return s.resolveMigrateLatestComplete(ctx, req.SourceBackupRoot)
	case dbPath != "" && filesPath != "":
		return s.resolveMigrateDirectPair(ctx, req.SourceBackupRoot, dbPath, filesPath, model.MigrateSelectionExplicitPair)
	default:
		failure := model.NewMigrateFailure(model.KindUsage, "полный migrate требует либо latest complete backup-set, либо explicit direct pair", nil)
		return model.MigrateSource{}, failure
	}
}

func (s MigrateService) resolveMigrateLatestComplete(ctx context.Context, backupRoot string) (model.MigrateSource, error) {
	groups, err := s.store.ListBackupGroups(ctx, backupRoot)
	if err != nil {
		return model.MigrateSource{}, err
	}
	for i := len(groups) - 1; i >= 0; i-- {
		group := groups[i]
		layout := model.NewBackupLayoutForStamp(backupRoot, group.Prefix, group.Stamp)
		dbArtifact, dbErr := s.store.VerifyDBArtifact(ctx, layout.DBArtifact, "")
		if dbErr != nil {
			continue
		}
		filesArtifact, filesErr := s.store.VerifyFilesArtifact(ctx, layout.FilesArtifact, "")
		if filesErr != nil {
			continue
		}
		source := model.MigrateSource{
			SelectionMode:    model.MigrateSelectionLatestFull,
			SourceKind:       model.MigrateSourceBackupRoot,
			DBBackup:         dbArtifact,
			FilesBackup:      filesArtifact,
			DirectFilesExact: true,
		}
		if err := s.attachMatchingMigrateManifest(ctx, backupRoot, group, &source); err != nil {
			return model.MigrateSource{}, err
		}
		return source, nil
	}
	return model.MigrateSource{}, model.BackupVerifyArtifactError{Label: "source selection", Err: fmt.Errorf("в source backup root нет complete verified backup-set")}
}

func (s MigrateService) resolveMigrateDirectPair(ctx context.Context, backupRoot, dbPath, filesPath, mode string) (model.MigrateSource, error) {
	dbArtifact, err := s.store.VerifyDBArtifact(ctx, dbPath, "")
	if err != nil {
		return model.MigrateSource{}, err
	}
	filesArtifact, err := s.store.VerifyFilesArtifact(ctx, filesPath, "")
	if err != nil {
		return model.MigrateSource{}, err
	}
	if err := model.ValidateMigrateDirectPair(dbArtifact.Path, filesArtifact.Path); err != nil {
		return model.MigrateSource{}, err
	}
	source := model.MigrateSource{
		SelectionMode:    mode,
		SourceKind:       model.MigrateSourceDirect,
		DBBackup:         dbArtifact,
		FilesBackup:      filesArtifact,
		DirectFilesExact: true,
	}
	group, ok := model.ParseBackupGroupName(filepath.Base(dbArtifact.Path))
	if ok {
		if err := s.attachMatchingMigrateManifest(ctx, backupRoot, group, &source); err != nil {
			return model.MigrateSource{}, err
		}
	}
	return source, nil
}

func (s MigrateService) resolveMigrateDBOnly(ctx context.Context, req model.MigrateRequest) (model.MigrateSource, error) {
	explicit := strings.TrimSpace(req.DBBackup)
	dbArtifact, err := s.store.VerifyDBArtifact(ctx, explicit, "")
	if err != nil {
		return model.MigrateSource{}, err
	}
	return model.MigrateSource{
		SelectionMode: model.MigrateSelectionExplicitDB,
		SourceKind:    model.MigrateSourceDirect,
		DBBackup:      dbArtifact,
	}, nil
}

func (s MigrateService) resolveMigrateFilesOnly(ctx context.Context, req model.MigrateRequest) (model.MigrateSource, error) {
	explicit := strings.TrimSpace(req.FilesBackup)
	filesArtifact, err := s.store.VerifyFilesArtifact(ctx, explicit, "")
	if err != nil {
		return model.MigrateSource{}, err
	}
	return model.MigrateSource{
		SelectionMode:    model.MigrateSelectionExplicitFile,
		SourceKind:       model.MigrateSourceDirect,
		FilesBackup:      filesArtifact,
		DirectFilesExact: true,
	}, nil
}

func (s MigrateService) attachMatchingMigrateManifest(ctx context.Context, backupRoot string, group model.BackupGroup, source *model.MigrateSource) error {
	if source == nil || source.DBBackup.Path == "" || source.FilesBackup.Path == "" {
		return nil
	}
	layout := model.NewBackupLayoutForStamp(backupRoot, group.Prefix, group.Stamp)
	if filepath.Clean(source.DBBackup.Path) != filepath.Clean(layout.DBArtifact) || filepath.Clean(source.FilesBackup.Path) != filepath.Clean(layout.FilesArtifact) {
		return nil
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if _, err := os.Stat(layout.ManifestJSON); os.IsNotExist(err) {
		return nil
	} else if err != nil {
		return model.BackupVerifyArtifactError{Label: "matching manifest", Err: err}
	}
	manifest, err := s.store.LoadRestoreManifest(ctx, layout.ManifestJSON)
	if err != nil {
		return model.BackupVerifyArtifactError{Label: "matching manifest", Err: fmt.Errorf("matching manifest backup-set неконсистентен: %w", err)}
	}
	if err := model.ValidateBackupVerifyManifestCoherence(layout.ManifestJSON, manifest); err != nil {
		return model.BackupVerifyArtifactError{Label: "matching manifest", Err: fmt.Errorf("matching manifest backup-set неконсистентен: %w", err)}
	}
	source.ManifestPath = layout.ManifestJSON
	return nil
}

func (s MigrateService) prepareMigrateRuntime(ctx context.Context, req model.MigrateRequest, result *model.MigrateResult) (migrateRuntimeState, error) {
	running, err := s.runtime.RunningServices(ctx, req.Target)
	if err != nil {
		return migrateRuntimeState{}, err
	}
	state := migrateRuntimeState{
		stoppedAppServices: model.RunningApplicationServices(running),
	}
	state.appServicesWereRunning = len(state.stoppedAppServices) != 0
	state.startedDBTemporarily = !migrateServiceRunning(running, "db")
	result.Details.AppServicesWereRunning = state.appServicesWereRunning
	result.Details.StartedDBTemporarily = state.startedDBTemporarily

	if state.startedDBTemporarily {
		if err := s.runtime.StartServices(ctx, req.Target, "db"); err != nil {
			return state, err
		}
	}
	if len(state.stoppedAppServices) != 0 {
		if err := s.runtime.StopServices(ctx, req.Target, state.stoppedAppServices...); err != nil {
			return state, err
		}
	}
	return state, nil
}

func (s MigrateService) returnMigrateRuntime(ctx context.Context, req model.MigrateRequest) (string, error) {
	if req.NoStart {
		return model.StatusSkipped, nil
	}
	if err := s.runtime.StartServices(ctx, req.Target, model.ApplicationServices()...); err != nil {
		return "", err
	}
	return model.StatusCompleted, nil
}

func (s MigrateService) postMigrateCheck(ctx context.Context, req model.MigrateRequest) error {
	services := []string{"db"}
	if !req.NoStart {
		services = append(services, model.ApplicationServices()...)
	}
	return s.runtime.PostRestoreCheck(ctx, req.Target, services...)
}

func normalizeMigrateRequest(req model.MigrateRequest) model.MigrateRequest {
	req.SourceScope = strings.TrimSpace(req.SourceScope)
	req.TargetScope = strings.TrimSpace(req.TargetScope)
	req.ProjectDir = filepath.Clean(strings.TrimSpace(req.ProjectDir))
	req.ComposeFile = filepath.Clean(strings.TrimSpace(req.ComposeFile))
	req.SourceEnvFile = strings.TrimSpace(req.SourceEnvFile)
	req.TargetEnvFile = strings.TrimSpace(req.TargetEnvFile)
	req.SourceBackupRoot = strings.TrimSpace(req.SourceBackupRoot)
	req.TargetBackupRoot = strings.TrimSpace(req.TargetBackupRoot)
	req.DBBackup = strings.TrimSpace(req.DBBackup)
	req.FilesBackup = strings.TrimSpace(req.FilesBackup)
	req.StorageDir = strings.TrimSpace(req.StorageDir)
	return req
}

func addMigrateWarnings(result *model.MigrateResult, req model.MigrateRequest) {
	if req.SkipDB {
		result.AddWarning("migrate пропустит database apply из-за skip_db")
	}
	if req.SkipFiles {
		result.AddWarning("migrate пропустит files apply из-за skip_files")
	}
	if req.NoStart {
		result.AddWarning("migrate оставит target application services остановленными из-за no_start")
	}
}

func validateMigrateCompatibility(req model.MigrateRequest) error {
	mismatches := model.MigrateCompatibilityMismatches(req.SourceSettings, req.TargetSettings)
	if len(mismatches) == 0 {
		return nil
	}
	parts := make([]string, 0, len(mismatches))
	for _, mismatch := range mismatches {
		parts = append(parts, fmt.Sprintf("%s ('%s' vs '%s')", mismatch.Name, mismatch.SourceValue, mismatch.TargetValue))
	}
	return domainfailure.Failure{
		Kind: domainfailure.KindValidation,
		Code: model.MigrateFailedCode,
		Err: fmt.Errorf(
			"source и target conflict with the migration compatibility contract: %s",
			strings.Join(parts, "; "),
		),
	}
}

func migrateFailureFromError(err error) model.BackupFailure {
	var failure model.BackupFailure
	if errors.As(err, &failure) {
		return model.NewMigrateFailure(failure.Kind, failure.Message, err)
	}
	var domainErr domainfailure.Failure
	if errors.As(err, &domainErr) {
		return model.NewMigrateFailure(modelKindFromDomain(domainErr.Kind), "migrate завершился ошибкой", err)
	}
	var artifactErr model.BackupVerifyArtifactError
	if errors.As(err, &artifactErr) {
		return model.NewMigrateFailure(model.KindValidation, "migrate source не прошёл проверку", err)
	}
	var manifestErr model.BackupVerifyManifestError
	if errors.As(err, &manifestErr) {
		return model.NewCommandFailure(model.KindManifest, model.ManifestInvalidCode, "manifest migrate source не прошёл проверку", err)
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return model.NewMigrateFailure(model.KindExternal, "migrate прерван", err)
	}
	return model.NewMigrateFailure(model.KindIO, "migrate завершился ошибкой", err)
}

func blockMigrateAfter(result *model.MigrateResult, failedStep string) {
	steps := []string{
		model.MigrateStepSourceSelection,
		model.MigrateStepCompatibility,
		model.MigrateStepTargetSnapshot,
		model.StepRuntimePrepare,
		model.MigrateStepDBApply,
		model.MigrateStepFilesApply,
		model.MigrateStepPermission,
		model.StepRuntimeReturn,
		model.MigrateStepPostCheck,
	}
	seen := false
	for _, step := range steps {
		if step == failedStep {
			seen = true
			continue
		}
		if !seen || migrateHasStep(*result, step) {
			continue
		}
		result.AddStep(step, model.StatusBlocked)
	}
}

func migrateHasStep(result model.MigrateResult, code string) bool {
	for _, step := range result.Items {
		if step.Code == code {
			return true
		}
	}
	return false
}

func migrateServiceRunning(running []string, want string) bool {
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
