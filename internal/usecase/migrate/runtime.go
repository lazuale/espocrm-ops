package migrate

import (
	"fmt"
	"strings"

	platformdocker "github.com/lazuale/espocrm-ops/internal/platform/docker"
)

const defaultReadinessTimeoutSeconds = 300

var migrationAppServices = []string{
	"espocrm",
	"espocrm-daemon",
	"espocrm-websocket",
}

func prepareRuntime(projectDir, composeFile, envFile string) (runtimePrepareInfo, error) {
	info := runtimePrepareInfo{}
	cfg := platformdocker.ComposeConfig{
		ProjectDir:  projectDir,
		ComposeFile: composeFile,
		EnvFile:     envFile,
	}

	dbState, err := platformdocker.ComposeServiceStateFor(cfg, "db")
	if err != nil {
		return info, wrapExternalError(err)
	}

	if dbState.Status != "running" && dbState.Status != "healthy" {
		info.StartedDBTemporarily = true
		if err := platformdocker.ComposeUp(cfg, "db"); err != nil {
			return info, wrapExternalError(err)
		}
	}

	if err := platformdocker.WaitForServicesReady(cfg, defaultReadinessTimeoutSeconds, "db"); err != nil {
		return info, wrapExternalError(err)
	}

	runningServices, err := platformdocker.ComposeRunningServices(cfg)
	if err != nil {
		return info, wrapExternalError(err)
	}
	if migrationAppServicesRunning(runningServices) {
		info.StoppedAppServices = runningAppServices(runningServices)
		if err := platformdocker.ComposeStop(cfg, migrationAppServices...); err != nil {
			return info, wrapExternalError(err)
		}
	}

	return info, nil
}

func expectedStartedTargetServices() []string {
	return append([]string{"db"}, migrationAppServices...)
}

func resolveDBContainer(projectDir, composeFile, envFile string) (string, error) {
	cfg := platformdocker.ComposeConfig{
		ProjectDir:  projectDir,
		ComposeFile: composeFile,
		EnvFile:     envFile,
	}

	container, err := platformdocker.ComposeServiceContainerID(cfg, "db")
	if err != nil {
		return "", wrapExternalError(err)
	}
	if strings.TrimSpace(container) == "" {
		return "", wrapExternalError(fmt.Errorf("could not resolve the db container after target runtime preparation"))
	}

	return container, nil
}

func migrationAppServicesRunning(services []string) bool {
	for _, service := range services {
		for _, appService := range migrationAppServices {
			if service == appService {
				return true
			}
		}
	}

	return false
}

func runningAppServices(services []string) []string {
	set := map[string]struct{}{}
	for _, service := range services {
		set[service] = struct{}{}
	}

	items := make([]string, 0, len(migrationAppServices))
	for _, service := range migrationAppServices {
		if _, ok := set[service]; ok {
			items = append(items, service)
		}
	}

	return items
}
