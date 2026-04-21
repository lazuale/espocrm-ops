package appadapter

import (
	"errors"

	runtimeport "github.com/lazuale/espocrm-ops/internal/app/ports/runtimeport"
	domainfailure "github.com/lazuale/espocrm-ops/internal/domain/failure"
	platformdocker "github.com/lazuale/espocrm-ops/internal/platform/docker"
)

type Runtime struct{}

func (Runtime) Up(target runtimeport.Target, services ...string) error {
	return classifyRuntimeError(platformdocker.ComposeUp(composeTarget(target), services...))
}

func (Runtime) Stop(target runtimeport.Target, services ...string) error {
	return classifyRuntimeError(platformdocker.ComposeStop(composeTarget(target), services...))
}

func (Runtime) DockerClientVersion() (string, error) {
	version, err := platformdocker.DockerClientVersion()
	return version, classifyRuntimeError(err)
}

func (Runtime) DockerServerVersion() (string, error) {
	version, err := platformdocker.DockerServerVersion()
	return version, classifyRuntimeError(err)
}

func (Runtime) ComposeVersion() (string, error) {
	version, err := platformdocker.ComposeVersion()
	return version, classifyRuntimeError(err)
}

func (Runtime) RunningServices(target runtimeport.Target) ([]string, error) {
	services, err := platformdocker.ComposeRunningServices(composeTarget(target))
	return services, classifyRuntimeError(err)
}

func (Runtime) ServiceState(target runtimeport.Target, service string) (runtimeport.ServiceState, error) {
	state, err := platformdocker.ComposeServiceStateFor(composeTarget(target), service)
	if err != nil {
		return runtimeport.ServiceState{}, classifyRuntimeError(err)
	}

	return runtimeport.ServiceState{
		Status:        state.Status,
		HealthMessage: state.HealthMessage,
	}, nil
}

func (Runtime) ServiceContainerID(target runtimeport.Target, service string) (string, error) {
	container, err := platformdocker.ComposeServiceContainerID(composeTarget(target), service)
	return container, classifyRuntimeError(err)
}

func (Runtime) WaitForServicesReady(target runtimeport.Target, timeoutSeconds int, services ...string) error {
	return classifyRuntimeError(platformdocker.WaitForServicesReady(composeTarget(target), timeoutSeconds, services...))
}

func (Runtime) ValidateComposeConfig(target runtimeport.Target) error {
	return classifyRuntimeError(platformdocker.ValidateComposeConfig(composeTarget(target)))
}

func (Runtime) CheckDockerAvailable() error {
	return classifyRuntimeError(platformdocker.CheckDockerAvailable())
}

func (Runtime) CheckContainerRunning(container string) error {
	return classifyRuntimeError(platformdocker.CheckContainerRunning(container))
}

func (Runtime) DumpMySQLDumpGz(target runtimeport.Target, service, user, password, dbName, destPath string) error {
	return classifyRuntimeError(platformdocker.DumpMySQLDumpGz(composeTarget(target), service, user, password, dbName, destPath))
}

func (Runtime) ResetAndRestoreMySQLDumpGz(dbPath, container, rootPassword, dbName, appUser string) error {
	return classifyRuntimeError(platformdocker.ResetAndRestoreMySQLDumpGz(dbPath, container, rootPassword, dbName, appUser))
}

func (Runtime) CreateTarArchiveViaHelper(sourceDir, archivePath, mariaDBTag, espocrmImage string) error {
	return classifyRuntimeError(platformdocker.CreateTarArchiveViaHelper(sourceDir, archivePath, mariaDBTag, espocrmImage))
}

func (Runtime) ReconcileEspoStoragePermissions(targetDir, mariaDBTag, espocrmImage string) error {
	return classifyRuntimeError(platformdocker.ReconcileEspoStoragePermissions(targetDir, mariaDBTag, espocrmImage))
}

func composeTarget(target runtimeport.Target) platformdocker.ComposeConfig {
	return platformdocker.ComposeConfig{
		ProjectDir:  target.ProjectDir,
		ComposeFile: target.ComposeFile,
		EnvFile:     target.EnvFile,
	}
}

func classifyRuntimeError(err error) error {
	if err == nil {
		return nil
	}

	var unavailableErr platformdocker.UnavailableError
	if errors.As(err, &unavailableErr) {
		return domainfailure.Failure{Kind: domainfailure.KindExternal, Code: "docker_unavailable", Err: err}
	}

	var inspectErr platformdocker.ContainerInspectError
	if errors.As(err, &inspectErr) {
		return domainfailure.Failure{Kind: domainfailure.KindExternal, Code: "container_inspect_failed", Err: err}
	}

	var notRunningErr platformdocker.ContainerNotRunningError
	if errors.As(err, &notRunningErr) {
		return domainfailure.Failure{Kind: domainfailure.KindExternal, Code: "container_not_running", Err: err}
	}

	var detectErr platformdocker.DBClientDetectionError
	if errors.As(err, &detectErr) {
		return domainfailure.Failure{Kind: domainfailure.KindExternal, Code: "restore_db_failed", Err: err}
	}

	var execErr platformdocker.DBExecutionError
	if errors.As(err, &execErr) {
		return domainfailure.Failure{Kind: domainfailure.KindExternal, Code: "restore_db_failed", Err: err}
	}

	return err
}
