package app

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"time"

	operationapp "github.com/lazuale/espocrm-ops/internal/app/operation"
	envport "github.com/lazuale/espocrm-ops/internal/app/ports/envport"
	domainenv "github.com/lazuale/espocrm-ops/internal/domain/env"
	domainfailure "github.com/lazuale/espocrm-ops/internal/domain/failure"
	domainruntime "github.com/lazuale/espocrm-ops/internal/domain/runtime"
	"github.com/lazuale/espocrm-ops/internal/model"
	"github.com/lazuale/espocrm-ops/internal/opsconfig"
)

type RestoreCommandDependencies struct {
	Operations operationapp.Service
	Env        envport.Loader
	Core       RestoreService
}

type RestoreCommandService struct {
	operations operationapp.Service
	env        envport.Loader
	core       RestoreService
}

type RestoreCommandRequest struct {
	Scope           string
	ProjectDir      string
	ComposeFile     string
	EnvFileOverride string
	ManifestPath    string
	DBBackup        string
	FilesBackup     string
	SkipDB          bool
	SkipFiles       bool
	NoSnapshot      bool
	NoStop          bool
	NoStart         bool
	DryRun          bool
	Now             func() time.Time
}

func NewRestoreCommandService(deps RestoreCommandDependencies) RestoreCommandService {
	return RestoreCommandService{
		operations: deps.Operations,
		env:        deps.Env,
		core:       deps.Core,
	}
}

// Production CLI boundary: destructive restore command surface идёт через retained core.
// Общий operation/env/lock-контур остаётся command boundary, а не вторым
// restore engine.
func (s RestoreCommandService) Execute(ctx context.Context, req RestoreCommandRequest) (result model.RestoreResult, err error) {
	createdAt := time.Now().UTC()
	if req.Now != nil {
		createdAt = req.Now().UTC()
	}

	result = model.NewRestoreResult(model.RestoreRequest{
		Scope:       req.Scope,
		ProjectDir:  req.ProjectDir,
		ComposeFile: req.ComposeFile,
		EnvFile:     req.EnvFileOverride,
		Manifest:    req.ManifestPath,
		DBBackup:    req.DBBackup,
		FilesBackup: req.FilesBackup,
		SkipDB:      req.SkipDB,
		SkipFiles:   req.SkipFiles,
		NoSnapshot:  req.NoSnapshot,
		NoStop:      req.NoStop,
		NoStart:     req.NoStart,
		DryRun:      req.DryRun,
	})

	opCtx, prepareErr := s.operations.PrepareOperation(operationapp.OperationContextRequest{
		Scope:           strings.TrimSpace(req.Scope),
		Operation:       "restore",
		ProjectDir:      filepath.Clean(req.ProjectDir),
		EnvFileOverride: strings.TrimSpace(req.EnvFileOverride),
	})
	if prepareErr != nil {
		failure := restoreFailureFromError(prepareErr)
		result.Fail(failure)
		return result, failure
	}
	defer func() {
		if releaseErr := opCtx.Release(); releaseErr != nil {
			failure := model.NewRestoreFailure(model.KindIO, "не удалось освободить locks после restore", releaseErr)
			if err == nil {
				result.Fail(failure)
				err = failure
				return
			}
			err = errors.Join(err, failure)
		}
	}()

	coreReq, buildErr := s.restoreCoreRequest(opCtx, req, createdAt)
	if buildErr != nil {
		failure := restoreFailureFromError(buildErr)
		result = model.NewRestoreResult(coreReq)
		result.Fail(failure)
		return result, failure
	}

	return s.core.ExecuteRestore(ctx, coreReq)
}

func (s RestoreCommandService) restoreCoreRequest(opCtx operationapp.OperationContext, req RestoreCommandRequest, createdAt time.Time) (model.RestoreRequest, error) {
	retentionDays, err := domainenv.BackupRetentionDays(opCtx.Env)
	if err != nil {
		return model.RestoreRequest{}, domainfailure.Failure{
			Kind: domainfailure.KindValidation,
			Code: model.RestoreFailedCode,
			Err:  err,
		}
	}
	runtimeContract, err := opCtx.Env.RuntimeContract()
	if err != nil {
		return model.RestoreRequest{}, domainfailure.Failure{
			Kind: domainfailure.KindValidation,
			Code: model.RestoreFailedCode,
			Err:  err,
		}
	}

	prepared := model.RestoreRequest{
		Scope:       opCtx.Scope,
		ProjectDir:  opCtx.ProjectDir,
		ComposeFile: filepath.Clean(req.ComposeFile),
		EnvFile:     opCtx.Env.FilePath,
		BackupRoot:  opCtx.BackupRoot,
		StorageDir:  opsconfig.ResolveProjectPath(opCtx.ProjectDir, opCtx.Env.ESPOStorageDir()),
		Manifest:    strings.TrimSpace(req.ManifestPath),
		DBBackup:    strings.TrimSpace(req.DBBackup),
		FilesBackup: strings.TrimSpace(req.FilesBackup),
		SkipDB:      req.SkipDB,
		SkipFiles:   req.SkipFiles,
		NoSnapshot:  req.NoSnapshot,
		NoStop:      req.NoStop,
		NoStart:     req.NoStart,
		DryRun:      req.DryRun,
		Target: model.RuntimeTarget{
			ProjectDir:       opCtx.ProjectDir,
			ComposeFile:      filepath.Clean(req.ComposeFile),
			EnvFile:          opCtx.Env.FilePath,
			StorageDir:       opsconfig.ResolveProjectPath(opCtx.ProjectDir, opCtx.Env.ESPOStorageDir()),
			DBService:        "db",
			DBUser:           strings.TrimSpace(opCtx.Env.Value("DB_USER")),
			DBName:           strings.TrimSpace(opCtx.Env.Value("DB_NAME")),
			HelperImage:      runtimeContract.HelperImage,
			RuntimeUID:       runtimeContract.UID,
			RuntimeGID:       runtimeContract.GID,
			ReadinessTimeout: domainruntime.DefaultReadinessTimeoutSeconds,
		},
		Snapshot: model.BackupRequest{
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
				Image: runtimeContract.HelperImage,
			},
			Metadata: model.BackupMetadata{
				ComposeProject: strings.TrimSpace(opCtx.ComposeProject),
				EnvFileName:    filepath.Base(opCtx.Env.FilePath),
				EspoCRMImage:   strings.TrimSpace(opCtx.Env.Value("ESPOCRM_IMAGE")),
				MariaDBTag:     strings.TrimSpace(opCtx.Env.Value("MARIADB_TAG")),
			},
		},
	}

	if prepared.SkipDB || req.DryRun {
		return prepared, nil
	}

	rootPassword, err := s.env.ResolveDBRootPassword(envport.DBPasswordRequest{
		Container:    "db",
		Name:         prepared.Target.DBName,
		User:         "root",
		Password:     opCtx.Env.Value("DB_ROOT_PASSWORD"),
		PasswordFile: opCtx.Env.Value("DB_ROOT_PASSWORD_FILE"),
	})
	if err != nil {
		return prepared, err
	}
	prepared.Target.DBRootPassword = rootPassword

	return prepared, nil
}
