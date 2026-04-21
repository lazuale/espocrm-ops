package backupflow

import (
	backupstoreport "github.com/lazuale/espocrm-ops/internal/app/ports/backupstoreport"
	envport "github.com/lazuale/espocrm-ops/internal/app/ports/envport"
	filesport "github.com/lazuale/espocrm-ops/internal/app/ports/filesport"
	runtimeport "github.com/lazuale/espocrm-ops/internal/app/ports/runtimeport"
)

type Dependencies struct {
	Env     envport.Loader
	Runtime runtimeport.Runtime
	Files   filesport.Files
	Store   backupstoreport.Store
}

type Service struct {
	env     envport.Loader
	runtime runtimeport.Runtime
	files   filesport.Files
	store   backupstoreport.Store
}

func NewService(deps Dependencies) Service {
	return Service{
		env:     deps.Env,
		runtime: deps.Runtime,
		files:   deps.Files,
		store:   deps.Store,
	}
}
