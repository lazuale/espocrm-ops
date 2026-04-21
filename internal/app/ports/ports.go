package ports

import (
	"archive/tar"
	"io"

	domainbackup "github.com/lazuale/espocrm-ops/internal/domain/backup"
	domainenv "github.com/lazuale/espocrm-ops/internal/domain/env"
)

type RuntimeTarget struct {
	ProjectDir  string
	ComposeFile string
	EnvFile     string
}

type RuntimeServiceState struct {
	Status        string
	HealthMessage string
}

type DirReadiness struct {
	Path        string
	ProbePath   string
	Exists      bool
	Creatable   bool
	Writable    bool
	FreeSpaceOK bool
}

type LockReadiness struct {
	State        string
	MetadataPath string
	PID          string
	StalePaths   []string
}

const (
	LockReady  = "ready"
	LockActive = "active"
	LockLegacy = "legacy"
	LockStale  = "stale"
)

type VerifiedBackup struct {
	ManifestPath string
	Scope        string
	CreatedAt    string
	DBBackupPath string
	FilesPath    string
}

type ManifestCandidate struct {
	Prefix       string
	Stamp        string
	ManifestPath string
}

type GroupMode int

const (
	GroupModeAny GroupMode = iota
	GroupModeDB
	GroupModeFiles
	GroupModeManifests
)

type DBPasswordRequest struct {
	Container    string
	Name         string
	User         string
	Password     string
	PasswordFile string
}

type FileStage interface {
	PreparedDir() string
	Cleanup() error
}

type Releaser interface {
	Release() error
}

type EnvLoader interface {
	LoadOperationEnv(projectDir, scope, overridePath string) (domainenv.OperationEnv, error)
	ResolveProjectPath(projectDir, value string) string
	ResolveDBPassword(req DBPasswordRequest) (string, error)
	ResolveDBRootPassword(req DBPasswordRequest) (string, error)
}

type Runtime interface {
	Up(target RuntimeTarget, services ...string) error
	Stop(target RuntimeTarget, services ...string) error
	ConfigText(target RuntimeTarget) (string, error)
	PSText(target RuntimeTarget) (string, error)
	RunningServices(target RuntimeTarget) ([]string, error)
	ServiceState(target RuntimeTarget, service string) (RuntimeServiceState, error)
	ServiceContainerID(target RuntimeTarget, service string) (string, error)
	WaitForServicesReady(target RuntimeTarget, timeoutSeconds int, services ...string) error
	ValidateComposeConfig(target RuntimeTarget) error
	CheckDockerAvailable() error
	CheckContainerRunning(container string) error
	DumpMySQLDumpGz(target RuntimeTarget, service, user, password, dbName, destPath string) error
	ResetAndRestoreMySQLDumpGz(dbPath, container, rootPassword, dbName, appUser string) error
	CreateTarArchiveViaHelper(sourceDir, archivePath, mariaDBTag, espocrmImage string) error
	ReconcileEspoStoragePermissions(targetDir, mariaDBTag, espocrmImage string) error
}

type Files interface {
	CreateTarGz(sourceDir, archivePath string) error
	SHA256File(path string) (string, error)
	InspectDirReadiness(path string, minFreeMB int, hasMinFree bool) (DirReadiness, error)
	EnsureNonEmptyFile(label, path string) (int64, error)
	EnsureWritableDir(path string) error
	EnsureFreeSpace(path string, neededBytes uint64) error
	NewSiblingStage(targetDir, prefix string) (FileStage, error)
	UnpackTarGz(archivePath, destDir string, validateHeader func(*tar.Header) error) error
	PreparedTreeRoot(stageDir, targetBase string) (string, error)
	PreparedTreeRootExact(stageDir, targetBase string) (string, error)
	ReplaceTree(targetDir, preparedDir string) error
}

type BackupStore interface {
	VerifyManifest(manifestPath string) error
	VerifyManifestDetailed(manifestPath string) (VerifiedBackup, error)
	VerifyDirectDBBackup(dbPath string) error
	VerifyDirectFilesBackup(filesPath string) error
	ManifestCandidates(backupRoot string) ([]ManifestCandidate, error)
	Groups(backupRoot string, mode GroupMode) ([]domainbackup.BackupGroup, error)
	LoadManifest(path string) (domainbackup.Manifest, error)
	WriteManifest(path string, manifest domainbackup.Manifest) error
	WriteSHA256Sidecar(filePath, checksum, sidecarPath string) error
}

type Locks interface {
	AcquireSharedOperationLock(rootDir, scope string, log io.Writer) (Releaser, error)
	AcquireMaintenanceLock(backupRoot, contour, scope string, log io.Writer) (Releaser, error)
	AcquireRestoreDBLock() (Releaser, error)
	AcquireRestoreFilesLock() (Releaser, error)
	CheckSharedOperationReadiness(rootDir string) (LockReadiness, error)
	CheckMaintenanceReadiness(backupRoot string) (LockReadiness, error)
	CheckRestoreDBReadiness() (LockReadiness, error)
	CheckRestoreFilesReadiness() (LockReadiness, error)
}
