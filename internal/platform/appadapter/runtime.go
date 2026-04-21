package appadapter

import (
	runtimeport "github.com/lazuale/espocrm-ops/internal/app/ports/runtimeport"
	platformdocker "github.com/lazuale/espocrm-ops/internal/platform/docker"
)

type Runtime struct{}

func (Runtime) Up(target runtimeport.Target, services ...string) error {
	return platformdocker.ComposeUp(composeTarget(target), services...)
}

func (Runtime) Stop(target runtimeport.Target, services ...string) error {
	return platformdocker.ComposeStop(composeTarget(target), services...)
}

func (Runtime) ConfigText(target runtimeport.Target) (string, error) {
	return platformdocker.ComposeConfigText(composeTarget(target))
}

func (Runtime) PSText(target runtimeport.Target) (string, error) {
	return platformdocker.ComposePSText(composeTarget(target))
}

func (Runtime) DockerClientVersion() (string, error) {
	return platformdocker.DockerClientVersion()
}

func (Runtime) DockerServerVersion() (string, error) {
	return platformdocker.DockerServerVersion()
}

func (Runtime) ComposeVersion() (string, error) {
	return platformdocker.ComposeVersion()
}

func (Runtime) RunningServices(target runtimeport.Target) ([]string, error) {
	return platformdocker.ComposeRunningServices(composeTarget(target))
}

func (Runtime) ServiceState(target runtimeport.Target, service string) (runtimeport.ServiceState, error) {
	state, err := platformdocker.ComposeServiceStateFor(composeTarget(target), service)
	if err != nil {
		return runtimeport.ServiceState{}, err
	}

	return runtimeport.ServiceState{
		Status:        state.Status,
		HealthMessage: state.HealthMessage,
	}, nil
}

func (Runtime) ServiceContainerID(target runtimeport.Target, service string) (string, error) {
	return platformdocker.ComposeServiceContainerID(composeTarget(target), service)
}

func (Runtime) WaitForServicesReady(target runtimeport.Target, timeoutSeconds int, services ...string) error {
	return platformdocker.WaitForServicesReady(composeTarget(target), timeoutSeconds, services...)
}

func (Runtime) ValidateComposeConfig(target runtimeport.Target) error {
	return platformdocker.ValidateComposeConfig(composeTarget(target))
}

func (Runtime) CheckDockerAvailable() error {
	return platformdocker.CheckDockerAvailable()
}

func (Runtime) CheckContainerRunning(container string) error {
	return platformdocker.CheckContainerRunning(container)
}

func (Runtime) DumpMySQLDumpGz(target runtimeport.Target, service, user, password, dbName, destPath string) error {
	return platformdocker.DumpMySQLDumpGz(composeTarget(target), service, user, password, dbName, destPath)
}

func (Runtime) ResetAndRestoreMySQLDumpGz(dbPath, container, rootPassword, dbName, appUser string) error {
	return platformdocker.ResetAndRestoreMySQLDumpGz(dbPath, container, rootPassword, dbName, appUser)
}

func (Runtime) CreateTarArchiveViaHelper(sourceDir, archivePath, mariaDBTag, espocrmImage string) error {
	return platformdocker.CreateTarArchiveViaHelper(sourceDir, archivePath, mariaDBTag, espocrmImage)
}

func (Runtime) ReconcileEspoStoragePermissions(targetDir, mariaDBTag, espocrmImage string) error {
	return platformdocker.ReconcileEspoStoragePermissions(targetDir, mariaDBTag, espocrmImage)
}

func composeTarget(target runtimeport.Target) platformdocker.ComposeConfig {
	return platformdocker.ComposeConfig{
		ProjectDir:  target.ProjectDir,
		ComposeFile: target.ComposeFile,
		EnvFile:     target.EnvFile,
	}
}
