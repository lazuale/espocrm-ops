package restoreflow

import (
	backupstoreport "github.com/lazuale/espocrm-ops/internal/app/ports/backupstoreport"
	envport "github.com/lazuale/espocrm-ops/internal/app/ports/envport"
	filesport "github.com/lazuale/espocrm-ops/internal/app/ports/filesport"
	lockport "github.com/lazuale/espocrm-ops/internal/app/ports/lockport"
	runtimeport "github.com/lazuale/espocrm-ops/internal/app/ports/runtimeport"
)

type Dependencies struct {
	Env     envport.Loader
	Runtime runtimeport.Runtime
	Files   filesport.Files
	Locks   lockport.Locks
	Store   backupstoreport.Store
}

type Service struct {
	env     envport.Loader
	runtime runtimeport.Runtime
	files   filesport.Files
	locks   lockport.Locks
	store   backupstoreport.Store
}

type restoreLock interface {
	Release() error
}

func NewService(deps Dependencies) Service {
	return Service{
		env:     deps.Env,
		runtime: deps.Runtime,
		files:   deps.Files,
		locks:   deps.Locks,
		store:   deps.Store,
	}
}
