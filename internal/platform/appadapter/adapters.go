package appadapter

import (
	"archive/tar"
	"io"

	"github.com/lazuale/espocrm-ops/internal/app/ports"
	domainbackup "github.com/lazuale/espocrm-ops/internal/domain/backup"
	domainenv "github.com/lazuale/espocrm-ops/internal/domain/env"
	domainfailure "github.com/lazuale/espocrm-ops/internal/domain/failure"
	platformbackupstore "github.com/lazuale/espocrm-ops/internal/platform/backupstore"
	platformconfig "github.com/lazuale/espocrm-ops/internal/platform/config"
	platformdocker "github.com/lazuale/espocrm-ops/internal/platform/docker"
	platformfs "github.com/lazuale/espocrm-ops/internal/platform/fs"
	platformlocks "github.com/lazuale/espocrm-ops/internal/platform/locks"
)

type EnvLoader struct{}

func (EnvLoader) LoadOperationEnv(projectDir, scope, overridePath string) (env domainenv.OperationEnv, err error) {
	env, err = platformconfig.LoadOperationEnv(projectDir, scope, overridePath)
	if err == nil {
		return env, nil
	}

	switch err.(type) {
	case platformconfig.MissingEnvFileError, platformconfig.InvalidEnvFileError, platformconfig.EnvParseError, platformconfig.MissingEnvValueError, platformconfig.UnsupportedContourError:
		return env, domainfailure.Failure{Kind: domainfailure.KindValidation, Code: "operation_execute_failed", Err: err}
	default:
		return env, domainfailure.Failure{Kind: domainfailure.KindIO, Code: "operation_execute_failed", Err: err}
	}
}

func (EnvLoader) ResolveProjectPath(projectDir, value string) string {
	return platformconfig.ResolveProjectPath(projectDir, value)
}

func (EnvLoader) ResolveDBPassword(req ports.DBPasswordRequest) (string, error) {
	return platformconfig.ResolveDBPassword(platformconfig.DBConfig{
		Container:    req.Container,
		Name:         req.Name,
		User:         req.User,
		Password:     req.Password,
		PasswordFile: req.PasswordFile,
	})
}

func (EnvLoader) ResolveDBRootPassword(req ports.DBPasswordRequest) (string, error) {
	return platformconfig.ResolveDBRootPassword(platformconfig.DBConfig{
		Container:    req.Container,
		Name:         req.Name,
		User:         req.User,
		Password:     req.Password,
		PasswordFile: req.PasswordFile,
	})
}

type Runtime struct{}

func (Runtime) Up(target ports.RuntimeTarget, services ...string) error {
	return platformdocker.ComposeUp(composeTarget(target), services...)
}

func (Runtime) Stop(target ports.RuntimeTarget, services ...string) error {
	return platformdocker.ComposeStop(composeTarget(target), services...)
}

func (Runtime) ConfigText(target ports.RuntimeTarget) (string, error) {
	return platformdocker.ComposeConfigText(composeTarget(target))
}

func (Runtime) PSText(target ports.RuntimeTarget) (string, error) {
	return platformdocker.ComposePSText(composeTarget(target))
}

func (Runtime) RunningServices(target ports.RuntimeTarget) ([]string, error) {
	return platformdocker.ComposeRunningServices(composeTarget(target))
}

func (Runtime) ServiceState(target ports.RuntimeTarget, service string) (ports.RuntimeServiceState, error) {
	state, err := platformdocker.ComposeServiceStateFor(composeTarget(target), service)
	if err != nil {
		return ports.RuntimeServiceState{}, err
	}

	return ports.RuntimeServiceState{
		Status:        state.Status,
		HealthMessage: state.HealthMessage,
	}, nil
}

func (Runtime) ServiceContainerID(target ports.RuntimeTarget, service string) (string, error) {
	return platformdocker.ComposeServiceContainerID(composeTarget(target), service)
}

func (Runtime) WaitForServicesReady(target ports.RuntimeTarget, timeoutSeconds int, services ...string) error {
	return platformdocker.WaitForServicesReady(composeTarget(target), timeoutSeconds, services...)
}

func (Runtime) ValidateComposeConfig(target ports.RuntimeTarget) error {
	return platformdocker.ValidateComposeConfig(composeTarget(target))
}

func (Runtime) CheckDockerAvailable() error {
	return platformdocker.CheckDockerAvailable()
}

func (Runtime) CheckContainerRunning(container string) error {
	return platformdocker.CheckContainerRunning(container)
}

func (Runtime) DumpMySQLDumpGz(target ports.RuntimeTarget, service, user, password, dbName, destPath string) error {
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

type Files struct{}

func (Files) CreateTarGz(sourceDir, archivePath string) error {
	return platformfs.CreateTarGz(sourceDir, archivePath)
}

func (Files) SHA256File(path string) (string, error) {
	return platformfs.SHA256File(path)
}

func (Files) InspectDirReadiness(path string, minFreeMB int, hasMinFree bool) (ports.DirReadiness, error) {
	readiness, err := platformfs.InspectDirReadiness(path, minFreeMB, hasMinFree)
	if err != nil {
		return ports.DirReadiness{}, err
	}
	return ports.DirReadiness{
		Path:        readiness.Path,
		ProbePath:   readiness.ProbePath,
		Exists:      readiness.Exists,
		Creatable:   readiness.Creatable,
		Writable:    readiness.Writable,
		FreeSpaceOK: readiness.FreeSpaceOK,
	}, nil
}

func (Files) EnsureNonEmptyFile(label, path string) (int64, error) {
	return platformfs.EnsureNonEmptyFile(label, path)
}

func (Files) EnsureWritableDir(path string) error {
	return platformfs.EnsureWritableDir(path)
}

func (Files) EnsureFreeSpace(path string, neededBytes uint64) error {
	return platformfs.EnsureFreeSpace(path, neededBytes)
}

func (Files) NewSiblingStage(targetDir, prefix string) (ports.FileStage, error) {
	stage, err := platformfs.NewSiblingStage(targetDir, prefix)
	if err != nil {
		return nil, err
	}
	return stageAdapter{stage: stage}, nil
}

func (Files) UnpackTarGz(archivePath, destDir string, validateHeader func(*tar.Header) error) error {
	return platformfs.UnpackTarGz(archivePath, destDir, validateHeader)
}

func (Files) PreparedTreeRoot(stageDir, targetBase string) (string, error) {
	return platformfs.PreparedTreeRoot(stageDir, targetBase)
}

func (Files) PreparedTreeRootExact(stageDir, targetBase string) (string, error) {
	return platformfs.PreparedTreeRootExact(stageDir, targetBase)
}

func (Files) ReplaceTree(targetDir, preparedDir string) error {
	return platformfs.ReplaceTree(targetDir, preparedDir)
}

type stageAdapter struct {
	stage platformfs.Stage
}

func (s stageAdapter) PreparedDir() string { return s.stage.PreparedDir }
func (s stageAdapter) Cleanup() error      { return s.stage.Cleanup() }

type BackupStore struct{}

func (BackupStore) VerifyManifest(manifestPath string) error {
	return classifyBackupStoreError(platformbackupstore.VerifyManifest(manifestPath))
}

func (BackupStore) VerifyManifestDetailed(manifestPath string) (ports.VerifiedBackup, error) {
	info, err := platformbackupstore.VerifyManifestDetailed(manifestPath)
	if err != nil {
		return ports.VerifiedBackup{}, classifyBackupStoreError(err)
	}
	return ports.VerifiedBackup{
		ManifestPath: info.ManifestPath,
		Scope:        info.Scope,
		CreatedAt:    info.CreatedAt,
		DBBackupPath: info.DBBackupPath,
		FilesPath:    info.FilesPath,
	}, nil
}

func (BackupStore) VerifyDirectDBBackup(dbPath string) error {
	return classifyBackupStoreError(platformbackupstore.VerifyDirectDBBackup(dbPath))
}

func (BackupStore) VerifyDirectFilesBackup(filesPath string) error {
	return classifyBackupStoreError(platformbackupstore.VerifyDirectFilesBackup(filesPath))
}

func (BackupStore) ManifestCandidates(backupRoot string) ([]ports.ManifestCandidate, error) {
	candidates, err := platformbackupstore.ManifestCandidates(backupRoot)
	if err != nil {
		return nil, err
	}
	out := make([]ports.ManifestCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		out = append(out, ports.ManifestCandidate{
			Prefix:       candidate.Prefix,
			Stamp:        candidate.Stamp,
			ManifestPath: candidate.ManifestPath,
		})
	}
	return out, nil
}

func (BackupStore) Groups(backupRoot string, mode ports.GroupMode) ([]domainbackup.BackupGroup, error) {
	return platformbackupstore.Groups(backupRoot, platformbackupstore.GroupMode(mode))
}

func (BackupStore) LoadManifest(path string) (domainbackup.Manifest, error) {
	manifest, err := platformbackupstore.LoadManifest(path)
	if err != nil {
		return domainbackup.Manifest{}, classifyBackupStoreError(err)
	}
	return manifest, nil
}

func (BackupStore) WriteManifest(path string, manifest domainbackup.Manifest) error {
	return platformbackupstore.WriteManifest(path, manifest)
}

func (BackupStore) WriteSHA256Sidecar(filePath, checksum, sidecarPath string) error {
	return platformbackupstore.WriteSHA256Sidecar(filePath, checksum, sidecarPath)
}

type Locks struct{}

func (Locks) AcquireSharedOperationLock(rootDir, scope string, log io.Writer) (ports.Releaser, error) {
	return platformlocks.AcquireSharedOperationLock(rootDir, scope, log)
}

func (Locks) AcquireMaintenanceLock(backupRoot, contour, scope string, log io.Writer) (ports.Releaser, error) {
	return platformlocks.AcquireMaintenanceLock(backupRoot, contour, scope, log)
}

func (Locks) AcquireRestoreDBLock() (ports.Releaser, error) {
	return platformlocks.AcquireRestoreDBLock()
}

func (Locks) AcquireRestoreFilesLock() (ports.Releaser, error) {
	return platformlocks.AcquireRestoreFilesLock()
}

func (Locks) CheckSharedOperationReadiness(rootDir string) (ports.LockReadiness, error) {
	return adaptLockReadiness(platformlocks.CheckSharedOperationReadiness(rootDir))
}

func (Locks) CheckMaintenanceReadiness(backupRoot string) (ports.LockReadiness, error) {
	return adaptLockReadiness(platformlocks.CheckMaintenanceReadiness(backupRoot))
}

func (Locks) CheckRestoreDBReadiness() (ports.LockReadiness, error) {
	return adaptLockReadiness(platformlocks.CheckRestoreDBReadiness())
}

func (Locks) CheckRestoreFilesReadiness() (ports.LockReadiness, error) {
	return adaptLockReadiness(platformlocks.CheckRestoreFilesReadiness())
}

func adaptLockReadiness(readiness platformlocks.LockReadiness, err error) (ports.LockReadiness, error) {
	if err != nil {
		return ports.LockReadiness{}, err
	}
	return ports.LockReadiness{
		State:        readiness.State,
		MetadataPath: readiness.MetadataPath,
		PID:          readiness.PID,
		StalePaths:   append([]string(nil), readiness.StalePaths...),
	}, nil
}

func composeTarget(target ports.RuntimeTarget) platformdocker.ComposeConfig {
	return platformdocker.ComposeConfig{
		ProjectDir:  target.ProjectDir,
		ComposeFile: target.ComposeFile,
		EnvFile:     target.EnvFile,
	}
}

func classifyBackupStoreError(err error) error {
	if err == nil {
		return nil
	}
	switch err.(type) {
	case platformbackupstore.ManifestError:
		return domainfailure.Failure{Kind: domainfailure.KindManifest, Code: "manifest_invalid", Err: err}
	case platformbackupstore.ValidationError:
		return domainfailure.Failure{Kind: domainfailure.KindValidation, Code: "backup_verification_failed", Err: err}
	default:
		return err
	}
}
