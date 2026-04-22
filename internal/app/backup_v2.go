package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/lazuale/espocrm-ops/internal/model"
)

type BackupDependencies struct {
	Runtime model.Runtime
	Store   model.Store
}

type BackupService struct {
	runtime model.Runtime
	store   model.Store
}

func NewBackupService(deps BackupDependencies) BackupService {
	return BackupService{
		runtime: deps.Runtime,
		store:   deps.Store,
	}
}

func (s BackupService) ExecuteBackup(ctx context.Context, req model.BackupRequest) (result model.BackupResult, err error) {
	result = model.NewBackupResult(req)

	includeDB := !req.SkipDB
	includeFiles := !req.SkipFiles
	includeManifest := includeDB && includeFiles

	if err := validateBackupRequest(req); err != nil {
		failure := err.(model.BackupFailure)
		result.Fail(failure)
		return result, failure
	}
	if s.runtime == nil {
		failure := model.NewBackupFailure(model.KindValidation, "runtime не настроен", nil)
		result.Fail(failure)
		return result, failure
	}
	if s.store == nil {
		failure := model.NewBackupFailure(model.KindValidation, "store не настроен", nil)
		result.Fail(failure)
		return result, failure
	}

	layout := model.NewBackupLayout(req.BackupRoot, req.NamePrefix, req.CreatedAt)
	result.SetLayout(layout, includeDB, includeFiles, includeManifest)
	result.Details.CreatedAt = req.CreatedAt.UTC().Format(time.RFC3339)

	finalized := false
	if cleanupErr := s.store.CleanupLayoutTemps(ctx, layout); cleanupErr != nil {
		failure := model.NewBackupFailure(model.KindIO, "не удалось очистить временные backup-файлы перед стартом", cleanupErr)
		result.Fail(failure)
		return result, failure
	}
	defer func() {
		cleanupErr := s.store.CleanupLayoutTemps(ctx, layout)
		if err != nil && !finalized {
			removeErr := s.store.RemoveBackupSet(ctx, layout)
			if removeErr != nil {
				err = errors.Join(err, model.NewBackupFailure(model.KindIO, "не удалось убрать незавершённый backup-set", removeErr))
			}
		}
		if err == nil && cleanupErr != nil {
			failure := model.NewBackupFailure(model.KindIO, "после backup остались временные файлы", cleanupErr)
			result.Fail(failure)
			err = failure
		}
	}()

	if exists, existsErr := s.store.AnyExists(ctx, layout.SelectedPaths(includeDB, includeFiles, includeManifest)); existsErr != nil {
		result.AddStep(model.StepArtifactAllocation, model.StatusFailed)
		blockAfter(&result, model.StepArtifactAllocation)
		failure := model.NewBackupFailure(model.KindIO, "не удалось проверить целевые backup artifacts", existsErr)
		result.Fail(failure)
		return result, failure
	} else if exists {
		result.AddStep(model.StepArtifactAllocation, model.StatusFailed)
		blockAfter(&result, model.StepArtifactAllocation)
		failure := model.NewBackupFailure(model.KindIO, "целевой backup-set уже существует", nil)
		result.Fail(failure)
		return result, failure
	}
	if allocErr := s.store.EnsureLayout(ctx, layout); allocErr != nil {
		result.AddStep(model.StepArtifactAllocation, model.StatusFailed)
		blockAfter(&result, model.StepArtifactAllocation)
		failure := model.NewBackupFailure(model.KindIO, "не удалось подготовить canonical backup layout", allocErr)
		result.Fail(failure)
		return result, failure
	}
	result.AddStep(model.StepArtifactAllocation, model.StatusCompleted)

	target := model.RuntimeTarget{
		ProjectDir:  req.ProjectDir,
		ComposeFile: req.ComposeFile,
		EnvFile:     req.EnvFile,
		StorageDir:  req.StorageDir,
		DBService:   req.DBService,
		DBUser:      req.DBUser,
		DBPassword:  req.DBPassword,
		DBName:      req.DBName,
	}
	var appServicesWereRunning bool
	var stoppedAppServices []string
	runtimeNeedsReturn := false
	runtimeReturnRecorded := false
	defer func() {
		if err == nil || !runtimeNeedsReturn || runtimeReturnRecorded {
			return
		}
		if returnErr := s.runtime.StartServices(ctx, target, stoppedAppServices...); returnErr != nil {
			result.AddStep(model.StepRuntimeReturn, model.StatusFailed)
			err = errors.Join(err, model.NewBackupFailure(model.KindExternal, "runtime не удалось вернуть после ошибки backup", returnErr))
			return
		}
		result.AddStep(model.StepRuntimeReturn, model.StatusCompleted)
		runtimeReturnRecorded = true
	}()

	if req.NoStop {
		result.AddStep(model.StepRuntimePrepare, model.StatusSkipped)
	} else {
		runningServices, prepareErr := s.runtime.RunningServices(ctx, target)
		if prepareErr != nil {
			result.AddStep(model.StepRuntimePrepare, model.StatusFailed)
			blockAfter(&result, model.StepRuntimePrepare)
			failure := model.NewBackupFailure(model.KindExternal, "runtime prepare завершился ошибкой", prepareErr)
			result.Fail(failure)
			return result, failure
		}
		stoppedAppServices = model.RunningApplicationServices(runningServices)
		appServicesWereRunning = len(stoppedAppServices) != 0
		result.Details.AppServicesWereRunning = appServicesWereRunning
		if appServicesWereRunning {
			if stopErr := s.runtime.StopServices(ctx, target, stoppedAppServices...); stopErr != nil {
				result.AddStep(model.StepRuntimePrepare, model.StatusFailed)
				blockAfter(&result, model.StepRuntimePrepare)
				failure := model.NewBackupFailure(model.KindExternal, "runtime stop завершился ошибкой", stopErr)
				result.Fail(failure)
				return result, failure
			}
			runtimeNeedsReturn = true
		}
		result.AddStep(model.StepRuntimePrepare, model.StatusCompleted)
	}

	var dbArtifact model.Artifact
	if req.SkipDB {
		result.AddStep(model.StepDBBackup, model.StatusSkipped)
	} else {
		dbArtifact, err = s.writeRuntimeArtifact(ctx, layout.DBArtifact, func() (io.ReadCloser, error) {
			return s.runtime.DumpDatabase(ctx, target)
		})
		if err != nil {
			result.AddStep(model.StepDBBackup, model.StatusFailed)
			blockAfter(&result, model.StepDBBackup)
			failure := model.NewBackupFailure(model.KindExternal, "database backup завершился ошибкой", err)
			result.Fail(failure)
			return result, failure
		}
		result.AddStep(model.StepDBBackup, model.StatusCompleted)
	}

	var filesArtifact model.Artifact
	if req.SkipFiles {
		result.AddStep(model.StepFilesBackup, model.StatusSkipped)
	} else {
		filesArtifact, err = s.writeRuntimeArtifact(ctx, layout.FilesArtifact, func() (io.ReadCloser, error) {
			return s.runtime.ArchiveFiles(ctx, target)
		})
		if err != nil {
			result.AddStep(model.StepFilesBackup, model.StatusFailed)
			blockAfter(&result, model.StepFilesBackup)
			failure := model.NewBackupFailure(model.KindExternal, "files backup завершился ошибкой", err)
			result.Fail(failure)
			return result, failure
		}
		result.AddStep(model.StepFilesBackup, model.StatusCompleted)
	}

	if includeDB {
		if checksumErr := s.store.WriteChecksum(ctx, dbArtifact); checksumErr != nil {
			result.AddStep(model.StepFinalize, model.StatusFailed)
			blockAfter(&result, model.StepFinalize)
			failure := model.NewBackupFailure(model.KindIO, "не удалось записать checksum для database artifact", checksumErr)
			result.Fail(failure)
			return result, failure
		}
	}
	if includeFiles {
		if checksumErr := s.store.WriteChecksum(ctx, filesArtifact); checksumErr != nil {
			result.AddStep(model.StepFinalize, model.StatusFailed)
			blockAfter(&result, model.StepFinalize)
			failure := model.NewBackupFailure(model.KindIO, "не удалось записать checksum для files artifact", checksumErr)
			result.Fail(failure)
			return result, failure
		}
	}
	if includeManifest {
		manifest, manifestErr := model.NewCompleteManifest(req, dbArtifact, filesArtifact, appServicesWereRunning)
		if manifestErr != nil {
			result.AddStep(model.StepFinalize, model.StatusFailed)
			blockAfter(&result, model.StepFinalize)
			failure := model.NewBackupFailure(model.KindValidation, "manifest полного backup-set не прошёл проверку", manifestErr)
			result.Fail(failure)
			return result, failure
		}
		if manifestErr := s.store.WriteManifestText(ctx, layout.ManifestText, manifest); manifestErr != nil {
			result.AddStep(model.StepFinalize, model.StatusFailed)
			blockAfter(&result, model.StepFinalize)
			failure := model.NewBackupFailure(model.KindIO, "не удалось записать text manifest", manifestErr)
			result.Fail(failure)
			return result, failure
		}
		if manifestErr := s.store.WriteManifestJSON(ctx, layout.ManifestJSON, manifest); manifestErr != nil {
			result.AddStep(model.StepFinalize, model.StatusFailed)
			blockAfter(&result, model.StepFinalize)
			failure := model.NewBackupFailure(model.KindIO, "не удалось записать json manifest", manifestErr)
			result.Fail(failure)
			return result, failure
		}
	}
	finalized = true
	result.AddStep(model.StepFinalize, model.StatusCompleted)

	if retentionErr := s.cleanupRetention(ctx, req.BackupRoot, req.RetentionDays, req.CreatedAt); retentionErr != nil {
		result.AddStep(model.StepRetention, model.StatusFailed)
		failure := model.NewBackupFailure(model.KindIO, "retention завершился ошибкой", retentionErr)
		result.Fail(failure)
		return result, failure
	}
	result.AddStep(model.StepRetention, model.StatusCompleted)

	if runtimeNeedsReturn {
		runtimeReturnRecorded = true
		if returnErr := s.runtime.StartServices(ctx, target, stoppedAppServices...); returnErr != nil {
			result.AddStep(model.StepRuntimeReturn, model.StatusFailed)
			failure := model.NewBackupFailure(model.KindExternal, "runtime return завершился ошибкой", returnErr)
			result.Fail(failure)
			return result, failure
		}
		result.AddStep(model.StepRuntimeReturn, model.StatusCompleted)
	} else {
		runtimeReturnRecorded = true
		result.AddStep(model.StepRuntimeReturn, model.StatusSkipped)
	}

	result.Succeed()
	return result, nil
}

func validateBackupRequest(req model.BackupRequest) error {
	switch {
	case strings.TrimSpace(req.Scope) == "":
		return model.NewBackupFailure(model.KindUsage, "scope обязателен", nil)
	case req.SkipDB && req.SkipFiles:
		return model.NewBackupFailure(model.KindUsage, "нельзя одновременно задавать --skip-db и --skip-files", nil)
	case strings.TrimSpace(req.BackupRoot) == "":
		return model.NewBackupFailure(model.KindValidation, "BACKUP_ROOT обязателен", nil)
	case strings.TrimSpace(req.NamePrefix) == "":
		return model.NewBackupFailure(model.KindValidation, "backup name prefix обязателен", nil)
	case strings.TrimSpace(req.ProjectDir) == "":
		return model.NewBackupFailure(model.KindValidation, "project_dir обязателен", nil)
	case strings.TrimSpace(req.ComposeFile) == "":
		return model.NewBackupFailure(model.KindValidation, "compose_file обязателен", nil)
	case strings.TrimSpace(req.EnvFile) == "":
		return model.NewBackupFailure(model.KindValidation, "env_file обязателен", nil)
	case !req.SkipFiles && strings.TrimSpace(req.StorageDir) == "":
		return model.NewBackupFailure(model.KindValidation, "storage_dir обязателен для files backup", nil)
	case !req.SkipDB && strings.TrimSpace(req.DBService) == "":
		return model.NewBackupFailure(model.KindValidation, "db service обязателен для database backup", nil)
	case !req.SkipDB && strings.TrimSpace(req.DBUser) == "":
		return model.NewBackupFailure(model.KindValidation, "DB_USER обязателен для database backup", nil)
	case !req.SkipDB && strings.TrimSpace(req.DBPassword) == "":
		return model.NewBackupFailure(model.KindValidation, "DB_PASSWORD обязателен для database backup", nil)
	case !req.SkipDB && strings.TrimSpace(req.DBName) == "":
		return model.NewBackupFailure(model.KindValidation, "DB_NAME обязателен для database backup", nil)
	case req.CreatedAt.IsZero():
		return model.NewBackupFailure(model.KindValidation, "created_at обязателен", nil)
	case req.RetentionDays < 0:
		return model.NewBackupFailure(model.KindValidation, "retention_days не может быть отрицательным", nil)
	case !req.SkipDB && !req.SkipFiles && strings.TrimSpace(req.Metadata.ComposeProject) == "":
		return model.NewBackupFailure(model.KindValidation, "compose_project обязателен для полного backup", nil)
	case !req.SkipDB && !req.SkipFiles && strings.TrimSpace(req.Metadata.EnvFileName) == "":
		return model.NewBackupFailure(model.KindValidation, "env_file metadata обязателен для полного backup", nil)
	case !req.SkipDB && !req.SkipFiles && strings.TrimSpace(req.Metadata.EspoCRMImage) == "":
		return model.NewBackupFailure(model.KindValidation, "espocrm_image обязателен для полного backup", nil)
	case !req.SkipDB && !req.SkipFiles && strings.TrimSpace(req.Metadata.MariaDBTag) == "":
		return model.NewBackupFailure(model.KindValidation, "mariadb_tag обязателен для полного backup", nil)
	default:
		return nil
	}
}

func (s BackupService) writeRuntimeArtifact(ctx context.Context, path string, open func() (io.ReadCloser, error)) (model.Artifact, error) {
	stream, err := open()
	if err != nil {
		return model.Artifact{}, err
	}
	artifact, err := s.store.WriteArtifact(ctx, path, stream)
	closeErr := stream.Close()
	if err != nil {
		return model.Artifact{}, err
	}
	if closeErr != nil {
		return model.Artifact{}, closeErr
	}
	return artifact, nil
}

func (s BackupService) cleanupRetention(ctx context.Context, root string, retentionDays int, now time.Time) error {
	groups, err := s.store.ListBackupGroups(ctx, root)
	if err != nil {
		return err
	}
	cutoff := now.UTC().Add(-time.Duration(retentionDays+1) * 24 * time.Hour)
	for _, group := range groups {
		stampTime, parseErr := model.ParseStamp(group.Stamp)
		if parseErr != nil {
			return fmt.Errorf("разобрать timestamp backup-set %s_%s: %w", group.Prefix, group.Stamp, parseErr)
		}
		if !stampTime.Before(cutoff) {
			continue
		}
		layout := model.NewBackupLayoutForStamp(root, group.Prefix, group.Stamp)
		if removeErr := s.store.RemoveBackupSet(ctx, layout); removeErr != nil {
			return removeErr
		}
	}
	return nil
}

func blockAfter(result *model.BackupResult, failedStep string) {
	steps := []string{
		model.StepArtifactAllocation,
		model.StepRuntimePrepare,
		model.StepDBBackup,
		model.StepFilesBackup,
		model.StepFinalize,
		model.StepRetention,
	}
	seen := false
	for _, step := range steps {
		if step == failedStep {
			seen = true
			continue
		}
		if !seen {
			continue
		}
		if hasStep(*result, step) {
			continue
		}
		result.AddStep(step, model.StatusBlocked)
	}
}

func hasStep(result model.BackupResult, code string) bool {
	for _, step := range result.Items {
		if step.Code == code {
			return true
		}
	}
	return false
}
