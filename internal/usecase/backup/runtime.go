package backup

import (
	platformdocker "github.com/lazuale/espocrm-ops/internal/platform/docker"
)

type runtimePrepareInfo struct {
	AppServicesWereRunning bool
	StoppedAppServices     []string
}

type runtimeReturnInfo struct {
	RestartedAppServices []string
}

func prepareRuntime(projectDir, composeFile, envFile string) (runtimePrepareInfo, error) {
	info := runtimePrepareInfo{}
	cfg := platformdocker.ComposeConfig{
		ProjectDir:  projectDir,
		ComposeFile: composeFile,
		EnvFile:     envFile,
	}

	runningServices, err := platformdocker.ComposeRunningServices(cfg)
	if err != nil {
		return info, err
	}

	info.AppServicesWereRunning = platformdocker.OperationalAppServicesRunning(runningServices)
	info.StoppedAppServices = platformdocker.RunningOperationalAppServices(runningServices)
	if len(info.StoppedAppServices) == 0 {
		return info, nil
	}

	if err := platformdocker.ComposeStop(cfg, info.StoppedAppServices...); err != nil {
		return info, err
	}

	return info, nil
}

func returnRuntime(projectDir, composeFile, envFile string, prep runtimePrepareInfo) (runtimeReturnInfo, error) {
	info := runtimeReturnInfo{}
	if len(prep.StoppedAppServices) == 0 {
		return info, nil
	}

	cfg := platformdocker.ComposeConfig{
		ProjectDir:  projectDir,
		ComposeFile: composeFile,
		EnvFile:     envFile,
	}
	if err := platformdocker.ComposeUp(cfg, prep.StoppedAppServices...); err != nil {
		return info, err
	}

	info.RestartedAppServices = append(info.RestartedAppServices, prep.StoppedAppServices...)
	return info, nil
}
