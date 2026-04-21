package migrate

import (
	"fmt"
	"strings"

	runtimeport "github.com/lazuale/espocrm-ops/internal/app/ports/runtimeport"
	domainfailure "github.com/lazuale/espocrm-ops/internal/domain/failure"
	domainruntime "github.com/lazuale/espocrm-ops/internal/domain/runtime"
)

func (s Service) prepareRuntime(projectDir, composeFile, envFile string) (runtimePrepareInfo, error) {
	info := runtimePrepareInfo{}
	target := runtimeport.Target{
		ProjectDir:  projectDir,
		ComposeFile: composeFile,
		EnvFile:     envFile,
	}

	dbState, err := s.runtime.ServiceState(target, "db")
	if err != nil {
		return info, executeFailure{Kind: domainfailure.KindExternal, Err: err}
	}

	if dbState.Status != "running" && dbState.Status != "healthy" {
		info.StartedDBTemporarily = true
		if err := s.runtime.Up(target, "db"); err != nil {
			return info, executeFailure{Kind: domainfailure.KindExternal, Err: err}
		}
	}

	if err := s.runtime.WaitForServicesReady(target, domainruntime.DefaultReadinessTimeoutSeconds, "db"); err != nil {
		return info, executeFailure{Kind: domainfailure.KindExternal, Err: err}
	}

	runningServices, err := s.runtime.RunningServices(target)
	if err != nil {
		return info, executeFailure{Kind: domainfailure.KindExternal, Err: err}
	}
	info.StoppedAppServices = domainruntime.RunningAppServices(runningServices)
	if len(info.StoppedAppServices) != 0 {
		if err := s.runtime.Stop(target, info.StoppedAppServices...); err != nil {
			return info, executeFailure{Kind: domainfailure.KindExternal, Err: err}
		}
	}

	return info, nil
}

func expectedStartedTargetServices() []string {
	return append([]string{"db"}, domainruntime.AppServices()...)
}

func (s Service) resolveDBContainer(projectDir, composeFile, envFile string) (string, error) {
	target := runtimeport.Target{
		ProjectDir:  projectDir,
		ComposeFile: composeFile,
		EnvFile:     envFile,
	}

	container, err := s.runtime.ServiceContainerID(target, "db")
	if err != nil {
		return "", executeFailure{Kind: domainfailure.KindExternal, Err: err}
	}
	if strings.TrimSpace(container) == "" {
		return "", executeFailure{
			Kind: domainfailure.KindExternal,
			Err:  fmt.Errorf("could not resolve the db container after target runtime preparation"),
		}
	}

	return container, nil
}
