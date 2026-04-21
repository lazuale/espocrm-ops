package restore

import (
	backupapp "github.com/lazuale/espocrm-ops/internal/app/backup"
	operationapp "github.com/lazuale/espocrm-ops/internal/app/operation"
	backupstoreport "github.com/lazuale/espocrm-ops/internal/app/ports/backupstoreport"
	envport "github.com/lazuale/espocrm-ops/internal/app/ports/envport"
	filesport "github.com/lazuale/espocrm-ops/internal/app/ports/filesport"
	lockport "github.com/lazuale/espocrm-ops/internal/app/ports/lockport"
	runtimeport "github.com/lazuale/espocrm-ops/internal/app/ports/runtimeport"
)

type Dependencies struct {
	Operations operationapp.Service
	Backup     backupapp.Service
	Env        envport.Loader
	Runtime    runtimeport.Runtime
	Files      filesport.Files
	Locks      lockport.Locks
	Store      backupstoreport.Store
}

type Service struct {
	operations operationapp.Service
	backup     backupapp.Service
	env        envport.Loader
	runtime    runtimeport.Runtime
	files      filesport.Files
	locks      lockport.Locks
	store      backupstoreport.Store
}

type restoreLock interface {
	Release() error
}

func NewService(deps Dependencies) Service {
	return Service{
		operations: deps.Operations,
		backup:     deps.Backup,
		env:        deps.Env,
		runtime:    deps.Runtime,
		files:      deps.Files,
		locks:      deps.Locks,
		store:      deps.Store,
	}
}
