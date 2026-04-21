package restore

import (
	backupflow "github.com/lazuale/espocrm-ops/internal/app/internal/backupflow"
	restoreflow "github.com/lazuale/espocrm-ops/internal/app/internal/restoreflow"
	operationapp "github.com/lazuale/espocrm-ops/internal/app/operation"
	backupstoreport "github.com/lazuale/espocrm-ops/internal/app/ports/backupstoreport"
	envport "github.com/lazuale/espocrm-ops/internal/app/ports/envport"
	filesport "github.com/lazuale/espocrm-ops/internal/app/ports/filesport"
	lockport "github.com/lazuale/espocrm-ops/internal/app/ports/lockport"
	runtimeport "github.com/lazuale/espocrm-ops/internal/app/ports/runtimeport"
)

type Dependencies struct {
	Operations operationapp.Service
	Env        envport.Loader
	Runtime    runtimeport.Runtime
	Files      filesport.Files
	Locks      lockport.Locks
	Store      backupstoreport.Store
}

type Service struct {
	operations operationapp.Service
	env        envport.Loader
	runtime    runtimeport.Runtime
	files      filesport.Files
	locks      lockport.Locks
	store      backupstoreport.Store
	backupFlow backupflow.Service
	flow       restoreflow.Service
}

func NewService(deps Dependencies) Service {
	return Service{
		operations: deps.Operations,
		env:        deps.Env,
		runtime:    deps.Runtime,
		files:      deps.Files,
		locks:      deps.Locks,
		store:      deps.Store,
		backupFlow: backupflow.NewService(backupflow.Dependencies{
			Env:     deps.Env,
			Runtime: deps.Runtime,
			Files:   deps.Files,
			Store:   deps.Store,
		}),
		flow: restoreflow.NewService(restoreflow.Dependencies{
			Env:     deps.Env,
			Runtime: deps.Runtime,
			Files:   deps.Files,
			Locks:   deps.Locks,
			Store:   deps.Store,
		}),
	}
}
