package backup

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	operationapp "github.com/lazuale/espocrm-ops/internal/app/operation"
	"github.com/lazuale/espocrm-ops/internal/app/ports"
	"github.com/lazuale/espocrm-ops/internal/contract/apperr"
	domainenv "github.com/lazuale/espocrm-ops/internal/domain/env"
	domainfailure "github.com/lazuale/espocrm-ops/internal/domain/failure"
)

type Dependencies struct {
	Operations operationapp.Service
	Env        ports.EnvLoader
	Runtime    ports.Runtime
	Files      ports.Files
	Store      ports.BackupStore
}

type Service struct {
	operations operationapp.Service
	env        ports.EnvLoader
	runtime    ports.Runtime
	files      ports.Files
	store      ports.BackupStore
}

type Request struct {
	Scope           string
	ProjectDir      string
	ComposeFile     string
	EnvFileOverride string
	SkipDB          bool
	SkipFiles       bool
	NoStop          bool
	Now             func() time.Time
}

func NewService(deps Dependencies) Service {
	return Service{
		operations: deps.Operations,
		env:        deps.Env,
		runtime:    deps.Runtime,
		files:      deps.Files,
		store:      deps.Store,
	}
}

func (s Service) Execute(req Request) (info ExecuteInfo, err error) {
	ctx, err := s.operations.PrepareOperation(operationapp.OperationContextRequest{
		Scope:           strings.TrimSpace(req.Scope),
		Operation:       "backup",
		ProjectDir:      filepath.Clean(req.ProjectDir),
		EnvFileOverride: strings.TrimSpace(req.EnvFileOverride),
	})
	if err != nil {
		return info, wrapBackupBoundaryError(err)
	}
	defer func() {
		if releaseErr := ctx.Release(); releaseErr != nil {
			if err == nil {
				err = wrapBackupBoundaryError(releaseErr)
				return
			}
			err = errors.Join(err, wrapBackupBoundaryError(releaseErr))
		}
	}()

	prepared, err := s.buildPreparedRequest(ctx, req)
	if err != nil {
		return info, wrapBackupBoundaryError(err)
	}

	return s.ExecutePrepared(prepared)
}

func (s Service) buildPreparedRequest(ctx operationapp.OperationContext, req Request) (PreparedRequest, error) {
	retentionDays, err := domainenv.BackupRetentionDays(ctx.Env)
	if err != nil {
		return PreparedRequest{}, domainfailure.Failure{
			Kind: domainfailure.KindValidation,
			Code: "backup_failed",
			Err:  err,
		}
	}

	prepared := PreparedRequest{
		Scope:          ctx.Scope,
		ProjectDir:     ctx.ProjectDir,
		ComposeFile:    filepath.Clean(req.ComposeFile),
		EnvFile:        ctx.Env.FilePath,
		BackupRoot:     ctx.BackupRoot,
		StorageDir:     s.env.ResolveProjectPath(ctx.ProjectDir, ctx.Env.ESPOStorageDir()),
		NamePrefix:     domainenv.BackupNamePrefix(ctx.Env),
		RetentionDays:  retentionDays,
		ComposeProject: ctx.ComposeProject,
		DBUser:         strings.TrimSpace(ctx.Env.Value("DB_USER")),
		DBPassword:     ctx.Env.Value("DB_PASSWORD"),
		DBName:         strings.TrimSpace(ctx.Env.Value("DB_NAME")),
		EspoCRMImage:   strings.TrimSpace(ctx.Env.Value("ESPOCRM_IMAGE")),
		MariaDBTag:     strings.TrimSpace(ctx.Env.Value("MARIADB_TAG")),
		SkipDB:         req.SkipDB,
		SkipFiles:      req.SkipFiles,
		NoStop:         req.NoStop,
		Now:            req.Now,
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
			return PreparedRequest{}, domainfailure.Failure{
				Kind: domainfailure.KindValidation,
				Code: "backup_failed",
				Err:  fmt.Errorf("%s is not set in %s", field.name, ctx.Env.FilePath),
			}
		}
	}

	return prepared, nil
}

func wrapBackupBoundaryError(err error) error {
	var failure domainfailure.Failure
	if errors.As(err, &failure) && failure.Kind != "" {
		code := failure.Code
		if code == "" {
			code = "backup_failed"
		}
		return apperr.Wrap(apperr.Kind(failure.Kind), code, err)
	}
	if kind, ok := apperr.KindOf(err); ok {
		return apperr.Wrap(kind, "backup_failed", err)
	}

	return apperr.Wrap(apperr.KindInternal, "backup_failed", err)
}
