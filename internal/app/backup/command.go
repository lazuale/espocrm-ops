package backup

import (
	"errors"
	"path/filepath"
	"strings"
	"time"

	backupflow "github.com/lazuale/espocrm-ops/internal/app/internal/backupflow"
	operationapp "github.com/lazuale/espocrm-ops/internal/app/operation"
	backupstoreport "github.com/lazuale/espocrm-ops/internal/app/ports/backupstoreport"
	envport "github.com/lazuale/espocrm-ops/internal/app/ports/envport"
	filesport "github.com/lazuale/espocrm-ops/internal/app/ports/filesport"
	runtimeport "github.com/lazuale/espocrm-ops/internal/app/ports/runtimeport"
)

type Dependencies struct {
	Operations operationapp.Service
	Env        envport.Loader
	Runtime    runtimeport.Runtime
	Files      filesport.Files
	Store      backupstoreport.Store
}

type Service struct {
	operations operationapp.Service
	env        envport.Loader
	runtime    runtimeport.Runtime
	files      filesport.Files
	store      backupstoreport.Store
	flow       backupflow.Service
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
		flow: backupflow.NewService(backupflow.Dependencies{
			Env:     deps.Env,
			Runtime: deps.Runtime,
			Files:   deps.Files,
			Store:   deps.Store,
		}),
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

	prepared, err := s.flow.BuildRequest(ctx, backupflow.Options{
		ComposeFile: req.ComposeFile,
		SkipDB:      req.SkipDB,
		SkipFiles:   req.SkipFiles,
		NoStop:      req.NoStop,
		Now:         req.Now,
	})
	if err != nil {
		return info, wrapBackupBoundaryError(err)
	}

	info, err = s.flow.Execute(prepared)
	if err != nil {
		return info, wrapBackupBoundaryError(err)
	}

	return info, nil
}
