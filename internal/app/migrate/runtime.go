package migrate

import (
	"fmt"
	"strings"

	domainfailure "github.com/lazuale/espocrm-ops/internal/domain/failure"
	domainruntime "github.com/lazuale/espocrm-ops/internal/domain/runtime"
	platformdocker "github.com/lazuale/espocrm-ops/internal/platform/docker"
)

func prepareRuntime(projectDir, composeFile, envFile string) (runtimePrepareInfo, error) {
	info := runtimePrepareInfo{}
	cfg := platformdocker.ComposeConfig{
		ProjectDir:  projectDir,
		ComposeFile: composeFile,
		EnvFile:     envFile,
	}

	dbState, err := platformdocker.ComposeServiceStateFor(cfg, "db")
	if err != nil {
		return info, executeFailure{Kind: domainfailure.KindExternal, Err: err}
	}

	if dbState.Status != "running" && dbState.Status != "healthy" {
		info.StartedDBTemporarily = true
		if err := platformdocker.ComposeUp(cfg, "db"); err != nil {
			return info, executeFailure{Kind: domainfailure.KindExternal, Err: err}
		}
	}

	if err := platformdocker.WaitForServicesReady(cfg, domainruntime.DefaultReadinessTimeoutSeconds, "db"); err != nil {
		return info, executeFailure{Kind: domainfailure.KindExternal, Err: err}
	}

	runningServices, err := platformdocker.ComposeRunningServices(cfg)
	if err != nil {
		return info, executeFailure{Kind: domainfailure.KindExternal, Err: err}
	}
	info.StoppedAppServices = domainruntime.RunningAppServices(runningServices)
	if len(info.StoppedAppServices) != 0 {
		if err := platformdocker.ComposeStop(cfg, info.StoppedAppServices...); err != nil {
			return info, executeFailure{Kind: domainfailure.KindExternal, Err: err}
		}
	}

	return info, nil
}

func expectedStartedTargetServices() []string {
	return append([]string{"db"}, domainruntime.AppServices()...)
}

func resolveDBContainer(projectDir, composeFile, envFile string) (string, error) {
	cfg := platformdocker.ComposeConfig{
		ProjectDir:  projectDir,
		ComposeFile: composeFile,
		EnvFile:     envFile,
	}

	container, err := platformdocker.ComposeServiceContainerID(cfg, "db")
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
