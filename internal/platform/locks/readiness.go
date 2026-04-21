package locks

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

const (
	LockReady  = "ready"
	LockActive = "active"
	LockStale  = "stale"
)

type LockReadiness struct {
	State        string
	MetadataPath string
	PID          string
	StalePaths   []string
}

func CheckSharedOperationReadiness(rootDir string) (readiness LockReadiness, err error) {
	metadataPath, handlePath := predictedSharedOperationLockPaths(rootDir)

	handle, err := os.Open(handlePath)
	if err != nil {
		if os.IsNotExist(err) {
			if _, statErr := os.Stat(metadataPath); statErr == nil {
				return LockReadiness{
					State:        LockStale,
					MetadataPath: metadataPath,
					PID:          lockFileOwnerPID(metadataPath),
				}, nil
			} else if !os.IsNotExist(statErr) {
				return LockReadiness{}, fmt.Errorf("stat lock metadata %s: %w", metadataPath, statErr)
			}
			return LockReadiness{
				State:        LockReady,
				MetadataPath: metadataPath,
			}, nil
		}
		return LockReadiness{}, fmt.Errorf("open lock handle %s: %w", handlePath, err)
	}
	defer closeLockHandle(handle, handlePath, &err)

	if err := syscall.Flock(int(handle.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err == nil {
		_ = syscall.Flock(int(handle.Fd()), syscall.LOCK_UN)
		if _, statErr := os.Stat(metadataPath); statErr != nil {
			if !os.IsNotExist(statErr) {
				return LockReadiness{}, fmt.Errorf("stat lock metadata %s: %w", metadataPath, statErr)
			}
		}
		return LockReadiness{
			State:        LockStale,
			MetadataPath: metadataPath,
		}, nil
	} else if err == syscall.EWOULDBLOCK {
		return LockReadiness{
			State:        LockActive,
			MetadataPath: metadataPath,
			PID:          lockFileOwnerPID(metadataPath),
		}, nil
	} else {
		return LockReadiness{}, fmt.Errorf("probe lock %s: %w", metadataPath, err)
	}
}

func CheckMaintenanceReadiness(backupRoot string) (LockReadiness, error) {
	locksDir := filepath.Join(backupRoot, "locks")
	lockFiles, err := filepath.Glob(filepath.Join(locksDir, "*.lock"))
	if err != nil {
		return LockReadiness{}, fmt.Errorf("list maintenance locks in %s: %w", locksDir, err)
	}

	readiness := LockReadiness{
		State:        LockReady,
		MetadataPath: filepath.Join(locksDir, "maintenance.lock"),
	}

	for _, lockFile := range lockFiles {
		state, pid, err := metadataLockState(lockFile)
		if err != nil {
			return LockReadiness{}, err
		}

		switch state {
		case "active":
			return LockReadiness{
				State:        LockActive,
				MetadataPath: lockFile,
				PID:          pid,
			}, nil
		case "stale":
			readiness.State = LockStale
			readiness.StalePaths = append(readiness.StalePaths, lockFile)
		}
	}

	return readiness, nil
}

func CheckRestoreDBReadiness() (LockReadiness, error) {
	return CheckRestoreDBReadinessInDir("")
}

func CheckRestoreFilesReadiness() (LockReadiness, error) {
	return CheckRestoreFilesReadinessInDir("")
}

func CheckRestoreDBReadinessInDir(dir string) (LockReadiness, error) {
	return checkFileLockReadiness(restoreLockPathInDir(dir, restoreDBLockName))
}

func CheckRestoreFilesReadinessInDir(dir string) (LockReadiness, error) {
	return checkFileLockReadiness(restoreLockPathInDir(dir, restoreFilesLockName))
}

func checkFileLockReadiness(path string) (readiness LockReadiness, err error) {
	handle, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return LockReadiness{
				State:        LockReady,
				MetadataPath: path,
			}, nil
		}
		return LockReadiness{}, fmt.Errorf("open lock file %s: %w", path, err)
	}
	defer closeLockHandle(handle, path, &err)

	if err := syscall.Flock(int(handle.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err == nil {
		_ = syscall.Flock(int(handle.Fd()), syscall.LOCK_UN)
		return LockReadiness{
			State:        LockStale,
			MetadataPath: path,
		}, nil
	} else if err == syscall.EWOULDBLOCK {
		return LockReadiness{
			State:        LockActive,
			MetadataPath: path,
		}, nil
	} else {
		return LockReadiness{}, fmt.Errorf("probe lock %s: %w", path, err)
	}
}

func closeLockHandle(handle *os.File, handlePath string, errp *error) {
	if handle == nil {
		return
	}

	if closeErr := handle.Close(); closeErr != nil {
		wrapped := fmt.Errorf("close lock handle %s: %w", handlePath, closeErr)
		if *errp == nil {
			*errp = wrapped
		} else {
			*errp = errors.Join(*errp, wrapped)
		}
	}
}
