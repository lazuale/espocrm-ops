package runtimeport

type Target struct {
	ProjectDir  string
	ComposeFile string
	EnvFile     string
}

type ServiceState struct {
	Status        string
	HealthMessage string
}

type Runtime interface {
	Up(target Target, services ...string) error
	Stop(target Target, services ...string) error
	DockerClientVersion() (string, error)
	DockerServerVersion() (string, error)
	ComposeVersion() (string, error)
	RunningServices(target Target) ([]string, error)
	ServiceState(target Target, service string) (ServiceState, error)
	ServiceContainerID(target Target, service string) (string, error)
	WaitForServicesReady(target Target, timeoutSeconds int, services ...string) error
	ValidateComposeConfig(target Target) error
	CheckDockerAvailable() error
	CheckContainerRunning(container string) error
	DumpMySQLDumpGz(target Target, service, user, password, dbName, destPath string) error
	ResetAndRestoreMySQLDumpGz(dbPath, container, rootPassword, dbName, appUser string) error
	CreateTarArchiveViaHelper(sourceDir, archivePath, mariaDBTag, espocrmImage string) error
	ReconcileEspoStoragePermissions(targetDir, mariaDBTag, espocrmImage string) error
}
