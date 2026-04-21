package restore

import (
	"fmt"
	"slices"
	"strings"

	runtimeport "github.com/lazuale/espocrm-ops/internal/app/ports/runtimeport"
	domainruntime "github.com/lazuale/espocrm-ops/internal/domain/runtime"
)

func (s Service) inspectRuntime(projectDir, composeFile, envFile string, needDB bool) (runtimePrepareInfo, error) {
	info := runtimePrepareInfo{}
	target := runtimeport.Target{
		ProjectDir:  projectDir,
		ComposeFile: composeFile,
		EnvFile:     envFile,
	}

	if needDB {
		dbState, err := s.runtime.ServiceState(target, "db")
		if err != nil {
			return info, err
		}
		if dbState.Status != "running" && dbState.Status != "healthy" {
			info.StartedDBTemporarily = true
		} else {
			container, err := s.runtime.ServiceContainerID(target, "db")
			if err != nil {
				return info, err
			}
			info.DBContainer = strings.TrimSpace(container)
		}
	}

	runningServices, err := s.runtime.RunningServices(target)
	if err != nil {
		return info, err
	}
	info.AppServicesWereRunning = domainruntime.AppServicesRunning(runningServices)
	info.StoppedAppServices = domainruntime.RunningAppServices(runningServices)
	return info, nil
}

func (s Service) prepareRuntime(projectDir, composeFile, envFile string, needDB bool, noStop bool) (runtimePrepareInfo, error) {
	info := runtimePrepareInfo{}
	target := runtimeport.Target{
		ProjectDir:  projectDir,
		ComposeFile: composeFile,
		EnvFile:     envFile,
	}

	if needDB {
		dbState, err := s.runtime.ServiceState(target, "db")
		if err != nil {
			return info, err
		}
		if dbState.Status != "running" && dbState.Status != "healthy" {
			info.StartedDBTemporarily = true
			if err := s.runtime.Up(target, "db"); err != nil {
				return info, err
			}
		}
		if err := s.runtime.WaitForServicesReady(target, domainruntime.DefaultReadinessTimeoutSeconds, "db"); err != nil {
			return info, err
		}
		container, err := s.runtime.ServiceContainerID(target, "db")
		if err != nil {
			return info, err
		}
		info.DBContainer = strings.TrimSpace(container)
		if info.DBContainer == "" {
			return info, fmt.Errorf("could not resolve the db container after runtime preparation")
		}
	}

	runningServices, err := s.runtime.RunningServices(target)
	if err != nil {
		return info, err
	}
	info.AppServicesWereRunning = domainruntime.AppServicesRunning(runningServices)
	info.StoppedAppServices = domainruntime.RunningAppServices(runningServices)
	if noStop || len(info.StoppedAppServices) == 0 {
		return info, nil
	}

	if err := s.runtime.Stop(target, info.StoppedAppServices...); err != nil {
		return info, err
	}
	return info, nil
}

func (s Service) returnRuntime(projectDir, composeFile, envFile string, prep runtimePrepareInfo, noStart bool) (runtimeReturnInfo, error) {
	info := runtimeReturnInfo{}
	target := runtimeport.Target{
		ProjectDir:  projectDir,
		ComposeFile: composeFile,
		EnvFile:     envFile,
	}

	if len(prep.StoppedAppServices) != 0 && !noStart {
		if err := s.runtime.Up(target, prep.StoppedAppServices...); err != nil {
			return info, err
		}
		info.RestartedAppServices = append(info.RestartedAppServices, prep.StoppedAppServices...)
	}

	if prep.StartedDBTemporarily && len(prep.StoppedAppServices) == 0 {
		if err := s.runtime.Stop(target, "db"); err != nil {
			return info, err
		}
		info.StoppedDB = true
	}

	return info, nil
}

func (s Service) validatePostRestoreRuntimeHealth(projectDir, composeFile, envFile string, services []string) ([]string, error) {
	if len(services) == 0 {
		return nil, nil
	}

	target := runtimeport.Target{
		ProjectDir:  projectDir,
		ComposeFile: composeFile,
		EnvFile:     envFile,
	}
	if err := s.runtime.WaitForServicesReady(target, domainruntime.DefaultReadinessTimeoutSeconds, services...); err != nil {
		return nil, err
	}

	return append([]string(nil), services...), nil
}

func expectedPostRestoreServices(req ExecuteRequest, prep runtimePrepareInfo, ret runtimeReturnInfo) []string {
	services := []string{}

	switch {
	case req.NoStop:
		services = append(services, prep.StoppedAppServices...)
	default:
		services = append(services, ret.RestartedAppServices...)
	}

	if len(services) != 0 || (!req.SkipDB && !prep.StartedDBTemporarily) {
		services = append(services, "db")
	}

	out := make([]string, 0, len(services))
	for _, service := range services {
		service = strings.TrimSpace(service)
		if service == "" || slices.Contains(out, service) {
			continue
		}
		out = append(out, service)
	}

	return out
}
