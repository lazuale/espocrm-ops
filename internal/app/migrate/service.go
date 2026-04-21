package migrate

import (
	operationapp "github.com/lazuale/espocrm-ops/internal/app/operation"
	backupstoreport "github.com/lazuale/espocrm-ops/internal/app/ports/backupstoreport"
	envport "github.com/lazuale/espocrm-ops/internal/app/ports/envport"
	runtimeport "github.com/lazuale/espocrm-ops/internal/app/ports/runtimeport"
	restoreapp "github.com/lazuale/espocrm-ops/internal/app/restore"
)

type Dependencies struct {
	Operations operationapp.Service
	Restore    restoreapp.Service
	Env        envport.Loader
	Runtime    runtimeport.Runtime
	Store      backupstoreport.Store
}

type Service struct {
	operations operationapp.Service
	restore    restoreapp.Service
	env        envport.Loader
	runtime    runtimeport.Runtime
	store      backupstoreport.Store
}

func NewService(deps Dependencies) Service {
	return Service{
		operations: deps.Operations,
		restore:    deps.Restore,
		env:        deps.Env,
		runtime:    deps.Runtime,
		store:      deps.Store,
	}
}
