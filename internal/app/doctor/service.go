package doctor

import (
	envport "github.com/lazuale/espocrm-ops/internal/app/ports/envport"
	filesport "github.com/lazuale/espocrm-ops/internal/app/ports/filesport"
	lockport "github.com/lazuale/espocrm-ops/internal/app/ports/lockport"
	runtimeport "github.com/lazuale/espocrm-ops/internal/app/ports/runtimeport"
)

type Dependencies struct {
	Env     envport.Loader
	Files   filesport.Files
	Locks   lockport.Locks
	Runtime runtimeport.Runtime
}

type Service struct {
	env     envport.Loader
	files   filesport.Files
	locks   lockport.Locks
	runtime runtimeport.Runtime
}

func NewService(deps Dependencies) Service {
	return Service{
		env:     deps.Env,
		files:   deps.Files,
		locks:   deps.Locks,
		runtime: deps.Runtime,
	}
}

func runtimeTarget(projectDir, composeFile, envFile string) runtimeport.Target {
	return runtimeport.Target{
		ProjectDir:  projectDir,
		ComposeFile: composeFile,
		EnvFile:     envFile,
	}
}
