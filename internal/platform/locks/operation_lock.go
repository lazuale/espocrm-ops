package locks

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

type OperationLock struct {
	metadataPath string
	file         *os.File
}

type MaintenanceLock struct {
	metadataPath string
	file         *os.File
}

type LegacyMetadataOnlyLockError struct {
	Path string
	PID  string
	Kind string
}

func (e LegacyMetadataOnlyLockError) Error() string {
	if e.PID != "" {
		return fmt.Sprintf("found a legacy lock file without a flock handle for %s (PID %s): %s. Its state can no longer be verified safely; remove it manually after verification", e.Kind, e.PID, e.Path)
	}

	return fmt.Sprintf("found a legacy lock file without a flock handle for %s: %s. Its state can no longer be verified safely; remove it manually after verification", e.Kind, e.Path)
}

type MaintenanceConflictError struct {
	Contour string
	Path    string
	PID     string
}

func (e MaintenanceConflictError) Error() string {
	if e.PID != "" {
		return fmt.Sprintf("for contour %q another maintenance operation is already running (PID %s): %s", e.Contour, e.PID, e.Path)
	}

	return fmt.Sprintf("for contour %q another maintenance operation is already running: %s", e.Contour, e.Path)
}

func AcquireSharedOperationLock(rootDir, scope string, log io.Writer) (*OperationLock, error) {
	metadataPath, handlePath, err := sharedOperationLockPaths(rootDir)
	if err != nil {
		return nil, err
	}

	if legacy, pid, err := legacyMetadataOnlyLock(metadataPath, handlePath); err != nil {
		return nil, err
	} else if legacy {
		return nil, LegacyMetadataOnlyLockError{
			Path: metadataPath,
			PID:  pid,
			Kind: "the toolkit shared operations lock",
		}
	}

	handle, err := os.OpenFile(handlePath, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open operation lock handle %s: %w", handlePath, err)
	}

	waitLogged := false
	for {
		if err := syscall.Flock(int(handle.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err == nil {
			break
		} else if err != syscall.EWOULDBLOCK {
			_ = handle.Close()
			return nil, fmt.Errorf("acquire shared operations lock %s: %w", metadataPath, err)
		}

		if !waitLogged {
			pid := lockFileOwnerPID(metadataPath)
			if pid != "" {
				lockInfo(log, "Detected another active toolkit operation (PID %s), waiting for the shared lock to be released", pid)
			} else {
				lockInfo(log, "Detected another active toolkit operation, waiting for the shared lock to be released")
			}
			waitLogged = true
		}
		time.Sleep(time.Second)
	}

	if _, err := os.Stat(metadataPath); err == nil {
		lockWarn(log, "Detected leftovers from a previous unfinished toolkit operation, rewriting metadata: %s", metadataPath)
	}

	if err := writeLockMetadataFile(metadataPath, []string{
		fmt.Sprintf("%d", os.Getpid()),
		scope,
		time.Now().Format("2006-01-02_15-04-05"),
		rootDir,
	}); err != nil {
		_ = syscall.Flock(int(handle.Fd()), syscall.LOCK_UN)
		_ = handle.Close()
		return nil, err
	}

	lockInfo(log, "Acquired shared operations lock '%s': %s", scope, metadataPath)
	return &OperationLock{
		metadataPath: metadataPath,
		file:         handle,
	}, nil
}

func (l *OperationLock) Release() error {
	return releaseMetadataLock(l.metadataPath, l.file)
}

func AcquireMaintenanceLock(backupRoot, contour, scope string, log io.Writer) (*MaintenanceLock, error) {
	locksDir := filepath.Join(backupRoot, "locks")
	if err := os.MkdirAll(locksDir, 0o755); err != nil {
		return nil, fmt.Errorf("create locks directory %s: %w", locksDir, err)
	}

	lockFiles, err := filepath.Glob(filepath.Join(locksDir, "*.lock"))
	if err != nil {
		return nil, fmt.Errorf("list maintenance locks in %s: %w", locksDir, err)
	}

	for _, lockFile := range lockFiles {
		state, pid, err := metadataLockState(lockFile)
		if err != nil {
			return nil, err
		}

		switch state {
		case "active":
			return nil, MaintenanceConflictError{
				Contour: contour,
				Path:    lockFile,
				PID:     pid,
			}
		case "legacy_unverified":
			return nil, LegacyMetadataOnlyLockError{
				Path: lockFile,
				PID:  pid,
				Kind: fmt.Sprintf("contour maintenance operation %q", contour),
			}
		case "stale":
			lockWarn(log, "Found a stale lock file, removing: %s", lockFile)
			_ = os.Remove(lockFile)
		}
	}

	metadataPath := filepath.Join(locksDir, "maintenance.lock")
	handlePath := metadataLockHandlePath(metadataPath)
	handle, err := os.OpenFile(handlePath, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open maintenance lock handle %s: %w", handlePath, err)
	}

	if err := syscall.Flock(int(handle.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = handle.Close()
		if err == syscall.EWOULDBLOCK {
			return nil, MaintenanceConflictError{
				Contour: contour,
				Path:    metadataPath,
				PID:     lockFileOwnerPID(metadataPath),
			}
		}
		return nil, fmt.Errorf("acquire maintenance lock %s: %w", metadataPath, err)
	}

	if err := writeLockMetadataFile(metadataPath, []string{
		fmt.Sprintf("%d", os.Getpid()),
		scope,
		time.Now().Format("2006-01-02_15-04-05"),
	}); err != nil {
		_ = syscall.Flock(int(handle.Fd()), syscall.LOCK_UN)
		_ = handle.Close()
		return nil, err
	}

	lockInfo(log, "Acquired maintenance lock '%s': %s", scope, metadataPath)
	return &MaintenanceLock{
		metadataPath: metadataPath,
		file:         handle,
	}, nil
}

func (l *MaintenanceLock) Release() error {
	return releaseMetadataLock(l.metadataPath, l.file)
}

func sharedOperationLockPaths(rootDir string) (string, string, error) {
	lockRoot := filepath.Join(os.TempDir(), "espocrm-toolkit-locks")
	if err := os.MkdirAll(lockRoot, 0o755); err != nil {
		return "", "", fmt.Errorf("create operation lock root %s: %w", lockRoot, err)
	}

	sum := sha256.Sum256([]byte(rootDir))
	name := "repo-operation-" + hex.EncodeToString(sum[:8])[:16]
	metadataPath := filepath.Join(lockRoot, name+".lock")
	return metadataPath, filepath.Join(lockRoot, name+".flock"), nil
}

func metadataLockHandlePath(metadataPath string) string {
	dir := filepath.Dir(metadataPath)
	base := filepath.Base(metadataPath)
	return filepath.Join(dir, "."+base+".flock")
}

func legacyMetadataOnlyLock(metadataPath, handlePath string) (bool, string, error) {
	if _, err := os.Stat(metadataPath); err != nil {
		if os.IsNotExist(err) {
			return false, "", nil
		}
		return false, "", fmt.Errorf("stat lock metadata %s: %w", metadataPath, err)
	}

	if _, err := os.Stat(handlePath); err == nil {
		return false, "", nil
	} else if !os.IsNotExist(err) {
		return false, "", fmt.Errorf("stat lock handle %s: %w", handlePath, err)
	}

	return true, lockFileOwnerPID(metadataPath), nil
}

func metadataLockState(metadataPath string) (string, string, error) {
	handlePath := metadataLockHandlePath(metadataPath)
	legacy, pid, err := legacyMetadataOnlyLock(metadataPath, handlePath)
	if err != nil {
		return "", "", err
	}
	if legacy {
		return "legacy_unverified", pid, nil
	}

	handle, err := os.OpenFile(handlePath, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return "", "", fmt.Errorf("open lock handle %s: %w", handlePath, err)
	}
	defer handle.Close()

	if err := syscall.Flock(int(handle.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err == nil {
		_ = syscall.Flock(int(handle.Fd()), syscall.LOCK_UN)
		return "stale", lockFileOwnerPID(metadataPath), nil
	} else if err == syscall.EWOULDBLOCK {
		return "active", lockFileOwnerPID(metadataPath), nil
	} else {
		return "", "", fmt.Errorf("probe lock state %s: %w", metadataPath, err)
	}
}

func writeLockMetadataFile(path string, lines []string) error {
	dir := filepath.Dir(path)
	base := filepath.Base(path)
	tmp, err := os.CreateTemp(dir, "."+base+".tmp.")
	if err != nil {
		return fmt.Errorf("create temporary metadata lock file %s: %w", path, err)
	}

	tmpPath := tmp.Name()
	defer func() {
		_ = os.Remove(tmpPath)
	}()

	for _, line := range lines {
		if _, err := fmt.Fprintln(tmp, line); err != nil {
			_ = tmp.Close()
			return fmt.Errorf("write metadata for lock file %s: %w", path, err)
		}
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temporary metadata lock file %s: %w", path, err)
	}

	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("update lock-file metadata atomically %s: %w", path, err)
	}

	return nil
}

func releaseMetadataLock(metadataPath string, handle *os.File) error {
	if handle == nil {
		return nil
	}
	defer func() {
		_ = os.Remove(metadataPath)
	}()

	if err := syscall.Flock(int(handle.Fd()), syscall.LOCK_UN); err != nil {
		_ = handle.Close()
		return err
	}
	return handle.Close()
}

func lockFileOwnerPID(path string) string {
	raw, err := os.ReadFile(path)
	if err != nil {
		return ""
	}

	for _, line := range splitLines(string(raw)) {
		if line != "" {
			return line
		}
	}

	return ""
}

func splitLines(text string) []string {
	text = strings.ReplaceAll(text, "\r", "")
	return strings.Split(text, "\n")
}

func lockInfo(w io.Writer, format string, args ...any) {
	if w == nil {
		return
	}
	_, _ = fmt.Fprintf(w, "[info] "+format+"\n", args...)
}

func lockWarn(w io.Writer, format string, args ...any) {
	if w == nil {
		return
	}
	_, _ = fmt.Fprintf(w, "[warn] "+format+"\n", args...)
}
