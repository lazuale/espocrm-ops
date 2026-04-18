package locks

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

const (
	LockReady  = "ready"
	LockActive = "active"
	LockLegacy = "legacy"
	LockStale  = "stale"
)

type LockReadiness struct {
	State        string
	MetadataPath string
	PID          string
	StalePaths   []string
}

func CheckSharedOperationReadiness(rootDir string) (LockReadiness, error) {
	metadataPath, handlePath := predictedSharedOperationLockPaths(rootDir)

	legacy, pid, err := legacyMetadataOnlyLock(metadataPath, handlePath)
	if err != nil {
		return LockReadiness{}, err
	}
	if legacy {
		return LockReadiness{
			State:        LockLegacy,
			MetadataPath: metadataPath,
			PID:          pid,
		}, nil
	}

	handle, err := os.Open(handlePath)
	if err != nil {
		if os.IsNotExist(err) {
			return LockReadiness{
				State:        LockReady,
				MetadataPath: metadataPath,
			}, nil
		}
		return LockReadiness{}, fmt.Errorf("open lock handle %s: %w", handlePath, err)
	}
	defer handle.Close()

	if err := syscall.Flock(int(handle.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err == nil {
		_ = syscall.Flock(int(handle.Fd()), syscall.LOCK_UN)
		state := LockStale
		if _, statErr := os.Stat(metadataPath); statErr != nil {
			if os.IsNotExist(statErr) {
				state = LockStale
			} else {
				return LockReadiness{}, fmt.Errorf("stat lock metadata %s: %w", metadataPath, statErr)
			}
		} else {
			state = LockStale
		}
		return LockReadiness{
			State:        state,
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
		case "legacy_unverified":
			return LockReadiness{
				State:        LockLegacy,
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
