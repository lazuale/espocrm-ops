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

type MigrateCommandDependencies struct {
	Operations operationapp.Service
	Env        envport.Loader
	Core       MigrateService
}

type MigrateCommandService struct {
	operations operationapp.Service
	env        envport.Loader
	core       MigrateService
}

type MigrateCommandRequest struct {
	SourceScope string
	TargetScope string
	ProjectDir  string
	ComposeFile string
	DBBackup    string
	FilesBackup string
	SkipDB      bool
	SkipFiles   bool
	NoStart     bool
	Now         func() time.Time
}

func NewMigrateCommandService(deps MigrateCommandDependencies) MigrateCommandService {
	return MigrateCommandService{
		operations: deps.Operations,
		env:        deps.Env,
		core:       deps.Core,
	}
}

// Переходный CLI boundary: migrate должен входить в retained core через один
// internal/app owner. CLI не собирает nested request руками и не держит
// собственную destructive semantics.
func (s MigrateCommandService) Execute(ctx context.Context, req MigrateCommandRequest) (result model.MigrateResult, err error) {
	createdAt := time.Now().UTC()
	if req.Now != nil {
		createdAt = req.Now().UTC()
	}

	baseReq := model.MigrateRequest{
		SourceScope: req.SourceScope,
		TargetScope: req.TargetScope,
		ProjectDir:  req.ProjectDir,
		ComposeFile: req.ComposeFile,
		DBBackup:    req.DBBackup,
		FilesBackup: req.FilesBackup,
		SkipDB:      req.SkipDB,
		SkipFiles:   req.SkipFiles,
		NoStart:     req.NoStart,
	}
	result = model.NewMigrateResult(baseReq)

	sourceEnv, sourceErr := s.env.LoadOperationEnv(filepath.Clean(req.ProjectDir), strings.TrimSpace(req.SourceScope), "")
	if sourceErr != nil {
		failure := migrateFailureFromError(sourceErr)
		result.Fail(failure)
		return result, failure
	}

	opCtx, prepareErr := s.operations.PrepareOperation(operationapp.OperationContextRequest{
		Scope:      strings.TrimSpace(req.TargetScope),
		Operation:  "migrate",
		ProjectDir: filepath.Clean(req.ProjectDir),
	})
	if prepareErr != nil {
		failure := migrateFailureFromError(prepareErr)
		result.Fail(failure)
		return result, failure
	}
	defer func() {
		if releaseErr := opCtx.Release(); releaseErr != nil {
			failure := model.NewMigrateFailure(model.KindIO, "не удалось освободить locks после migrate", releaseErr)
			if err == nil {
				result.Fail(failure)
				err = failure
				return
			}
			err = errors.Join(err, failure)
		}
	}()

	coreReq, buildErr := s.migrateCoreRequest(sourceEnv, opCtx, req, createdAt)
	if buildErr != nil {
		failure := migrateFailureFromError(buildErr)
		result = model.NewMigrateResult(coreReq)
		result.Fail(failure)
		return result, failure
	}

	return s.core.ExecuteMigrate(ctx, coreReq)
}

func (s MigrateCommandService) migrateCoreRequest(sourceEnv domainenv.OperationEnv, opCtx operationapp.OperationContext, req MigrateCommandRequest, createdAt time.Time) (model.MigrateRequest, error) {
	retentionDays, err := domainenv.BackupRetentionDays(opCtx.Env)
	if err != nil {
		return model.MigrateRequest{}, domainfailure.Failure{
			Kind: domainfailure.KindValidation,
			Code: model.MigrateFailedCode,
			Err:  err,
		}
	}
	runtimeContract, err := opCtx.Env.RuntimeContract()
	if err != nil {
		return model.MigrateRequest{}, domainfailure.Failure{
			Kind: domainfailure.KindValidation,
			Code: model.MigrateFailedCode,
			Err:  err,
		}
	}

	prepared := model.MigrateRequest{
		SourceScope:      strings.TrimSpace(req.SourceScope),
		TargetScope:      opCtx.Scope,
		ProjectDir:       opCtx.ProjectDir,
		ComposeFile:      cleanOptionalPath(req.ComposeFile),
		SourceEnvFile:    sourceEnv.FilePath,
		TargetEnvFile:    opCtx.Env.FilePath,
		SourceBackupRoot: s.env.ResolveProjectPath(opCtx.ProjectDir, sourceEnv.BackupRoot()),
		TargetBackupRoot: opCtx.BackupRoot,
		DBBackup:         strings.TrimSpace(req.DBBackup),
		FilesBackup:      strings.TrimSpace(req.FilesBackup),
		SkipDB:           req.SkipDB,
		SkipFiles:        req.SkipFiles,
		NoStart:          req.NoStart,
		StorageDir:       opsconfig.ResolveProjectPath(opCtx.ProjectDir, opCtx.Env.ESPOStorageDir()),
		Target: model.RuntimeTarget{
			ProjectDir:       opCtx.ProjectDir,
			ComposeFile:      cleanOptionalPath(req.ComposeFile),
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
			ComposeFile:   cleanOptionalPath(req.ComposeFile),
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
		SourceSettings: migrateCompatibilitySettingsFromEnv(sourceEnv),
		TargetSettings: migrateCompatibilitySettingsFromEnv(opCtx.Env),
	}

	if req.SkipDB {
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

func migrateCompatibilitySettingsFromEnv(values domainenv.OperationEnv) model.MigrateCompatibilitySettings {
	return model.MigrateCompatibilitySettings{
		EspoCRMImage:    strings.TrimSpace(values.Value("ESPOCRM_IMAGE")),
		MariaDBTag:      strings.TrimSpace(values.Value("MARIADB_TAG")),
		DefaultLanguage: strings.TrimSpace(values.Value("ESPO_DEFAULT_LANGUAGE")),
		TimeZone:        strings.TrimSpace(values.Value("ESPO_TIME_ZONE")),
	}
}
