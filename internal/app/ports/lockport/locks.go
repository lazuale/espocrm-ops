package lockport

import "io"

type Readiness struct {
	State        string
	MetadataPath string
	PID          string
	StalePaths   []string
}

const (
	Ready  = "ready"
	Active = "active"
	Stale  = "stale"
)

type Releaser interface {
	Release() error
}

type Locks interface {
	AcquireSharedOperationLock(rootDir, scope string, log io.Writer) (Releaser, error)
	AcquireMaintenanceLock(backupRoot, contour, scope string, log io.Writer) (Releaser, error)
	AcquireRestoreDBLock() (Releaser, error)
	AcquireRestoreFilesLock() (Releaser, error)
	CheckSharedOperationReadiness(rootDir string) (Readiness, error)
	CheckMaintenanceReadiness(backupRoot string) (Readiness, error)
	CheckRestoreDBReadiness() (Readiness, error)
	CheckRestoreFilesReadiness() (Readiness, error)
}
