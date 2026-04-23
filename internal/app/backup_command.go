package app

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"time"

	operationapp "github.com/lazuale/espocrm-ops/internal/app/operation"
	domainenv "github.com/lazuale/espocrm-ops/internal/domain/env"
	domainfailure "github.com/lazuale/espocrm-ops/internal/domain/failure"
	"github.com/lazuale/espocrm-ops/internal/model"
	"github.com/lazuale/espocrm-ops/internal/opsconfig"
)

type BackupCommandDependencies struct {
	Operations operationapp.Service
	Core       BackupService
}

type BackupCommandService struct {
	operations operationapp.Service
	core       BackupService
}

type BackupCommandRequest struct {
	Scope           string
	ProjectDir      string
	ComposeFile     string
	EnvFileOverride string
	SkipDB          bool
	SkipFiles       bool
	NoStop          bool
	Now             func() time.Time
}

func NewBackupCommandService(deps BackupCommandDependencies) BackupCommandService {
	return BackupCommandService{
		operations: deps.Operations,
		core:       deps.Core,
	}
}

// Production CLI boundary: команда backup исполняется только через retained core.
// Operation/env/lock-контур остаётся общим command boundary, а не вторым
// implementation path.
func (s BackupCommandService) Execute(ctx context.Context, req BackupCommandRequest) (result model.BackupResult, err error) {
	createdAt := time.Now().UTC()
	if req.Now != nil {
		createdAt = req.Now().UTC()
	}

	result = model.NewBackupResult(model.BackupRequest{
		Scope:       req.Scope,
		ProjectDir:  req.ProjectDir,
		ComposeFile: req.ComposeFile,
		EnvFile:     req.EnvFileOverride,
		CreatedAt:   createdAt,
		SkipDB:      req.SkipDB,
		SkipFiles:   req.SkipFiles,
		NoStop:      req.NoStop,
	})

	opCtx, prepareErr := s.operations.PrepareOperation(operationapp.OperationContextRequest{
		Scope:           strings.TrimSpace(req.Scope),
		Operation:       "backup",
		ProjectDir:      filepath.Clean(req.ProjectDir),
		EnvFileOverride: strings.TrimSpace(req.EnvFileOverride),
	})
	if prepareErr != nil {
		failure := backupFailureFromError(prepareErr)
		result.Fail(failure)
		return result, failure
	}
	defer func() {
		if releaseErr := opCtx.Release(); releaseErr != nil {
			failure := model.NewBackupFailure(model.KindIO, "не удалось освободить locks после backup", releaseErr)
			if err == nil {
				result.Fail(failure)
				err = failure
				return
			}
			err = errors.Join(err, failure)
		}
	}()

	coreReq, buildErr := backupCoreRequest(opCtx, req, createdAt)
	if buildErr != nil {
		failure := backupFailureFromError(buildErr)
		result = model.NewBackupResult(coreReq)
		result.Fail(failure)
		return result, failure
	}

	return s.core.ExecuteBackup(ctx, coreReq)
}

func backupCoreRequest(opCtx operationapp.OperationContext, req BackupCommandRequest, createdAt time.Time) (model.BackupRequest, error) {
	retentionDays, err := domainenv.BackupRetentionDays(opCtx.Env)
	if err != nil {
		return model.BackupRequest{}, domainfailure.Failure{
			Kind: domainfailure.KindValidation,
			Code: model.BackupFailedCode,
			Err:  err,
		}
	}

	prepared := model.BackupRequest{
		Scope:         opCtx.Scope,
		ProjectDir:    opCtx.ProjectDir,
		ComposeFile:   filepath.Clean(req.ComposeFile),
		EnvFile:       opCtx.Env.FilePath,
		BackupRoot:    opCtx.BackupRoot,
		StorageDir:    opsconfig.ResolveProjectPath(opCtx.ProjectDir, opCtx.Env.ESPOStorageDir()),
		NamePrefix:    domainenv.BackupNamePrefix(opCtx.Env),
		RetentionDays: retentionDays,
		CreatedAt:     createdAt,
		DBService:     "db",
		DBUser:        strings.TrimSpace(opCtx.Env.Value("DB_USER")),
		DBPassword:    opCtx.Env.Value("DB_PASSWORD"),
		DBName:        strings.TrimSpace(opCtx.Env.Value("DB_NAME")),
		SkipDB:        req.SkipDB,
		SkipFiles:     req.SkipFiles,
		NoStop:        req.NoStop,
		HelperArchive: model.HelperArchiveContract{
			Image: strings.TrimSpace(opCtx.Env.Value("ESPO_HELPER_IMAGE")),
		},
		Metadata: model.BackupMetadata{
			ComposeProject: strings.TrimSpace(opCtx.ComposeProject),
			EnvFileName:    filepath.Base(opCtx.Env.FilePath),
			EspoCRMImage:   strings.TrimSpace(opCtx.Env.Value("ESPOCRM_IMAGE")),
			MariaDBTag:     strings.TrimSpace(opCtx.Env.Value("MARIADB_TAG")),
		},
	}

	if req.SkipDB {
		return prepared, nil
	}
	for _, field := range []struct {
		name  string
		value string
	}{
		{name: "DB_USER", value: prepared.DBUser},
		{name: "DB_PASSWORD", value: prepared.DBPassword},
		{name: "DB_NAME", value: prepared.DBName},
	} {
		if strings.TrimSpace(field.value) == "" {
			return model.BackupRequest{}, domainfailure.Failure{
				Kind: domainfailure.KindValidation,
				Code: model.BackupFailedCode,
				Err:  errors.New(field.name + " не задан в env-файле"),
			}
		}
	}

	return prepared, nil
}

func backupFailureFromError(err error) model.BackupFailure {
	var failure model.BackupFailure
	if errors.As(err, &failure) {
		return failure
	}

	var domainFailure domainfailure.Failure
	if errors.As(err, &domainFailure) {
		return model.NewBackupFailure(modelKindFromDomain(domainFailure.Kind), "backup завершился ошибкой", err)
	}

	return model.NewBackupFailure(model.KindIO, "backup завершился ошибкой", err)
}

func modelKindFromDomain(kind domainfailure.Kind) model.ErrorKind {
	switch kind {
	case domainfailure.KindValidation:
		return model.KindValidation
	case domainfailure.KindExternal:
		return model.KindExternal
	case domainfailure.KindIO:
		return model.KindIO
	default:
		return model.KindIO
	}
}
