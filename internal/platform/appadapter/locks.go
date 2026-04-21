package appadapter

import (
	"io"

	lockport "github.com/lazuale/espocrm-ops/internal/app/ports/lockport"
	platformlocks "github.com/lazuale/espocrm-ops/internal/platform/locks"
)

type Locks struct{}

func (Locks) AcquireSharedOperationLock(rootDir, scope string, log io.Writer) (lockport.Releaser, error) {
	return platformlocks.AcquireSharedOperationLock(rootDir, scope, log)
}

func (Locks) AcquireMaintenanceLock(backupRoot, contour, scope string, log io.Writer) (lockport.Releaser, error) {
	return platformlocks.AcquireMaintenanceLock(backupRoot, contour, scope, log)
}

func (Locks) AcquireRestoreDBLock() (lockport.Releaser, error) {
	return platformlocks.AcquireRestoreDBLock()
}

func (Locks) AcquireRestoreFilesLock() (lockport.Releaser, error) {
	return platformlocks.AcquireRestoreFilesLock()
}

func (Locks) CheckSharedOperationReadiness(rootDir string) (lockport.Readiness, error) {
	return adaptLockReadiness(platformlocks.CheckSharedOperationReadiness(rootDir))
}

func (Locks) CheckMaintenanceReadiness(backupRoot string) (lockport.Readiness, error) {
	return adaptLockReadiness(platformlocks.CheckMaintenanceReadiness(backupRoot))
}

func (Locks) CheckRestoreDBReadiness() (lockport.Readiness, error) {
	return adaptLockReadiness(platformlocks.CheckRestoreDBReadiness())
}

func (Locks) CheckRestoreFilesReadiness() (lockport.Readiness, error) {
	return adaptLockReadiness(platformlocks.CheckRestoreFilesReadiness())
}

func adaptLockReadiness(readiness platformlocks.LockReadiness, err error) (lockport.Readiness, error) {
	if err != nil {
		return lockport.Readiness{}, err
	}
	return lockport.Readiness{
		State:        readiness.State,
		MetadataPath: readiness.MetadataPath,
		PID:          readiness.PID,
		StalePaths:   append([]string(nil), readiness.StalePaths...),
	}, nil
}
