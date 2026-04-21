package locks

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

const (
	restoreDBLockName    = "espops-restore-db.lock"
	restoreFilesLockName = "espops-restore-files.lock"
)

type LockError struct {
	Err error
}

func (e LockError) Error() string {
	return e.Err.Error()
}

func (e LockError) Unwrap() error {
	return e.Err
}

type FileLock struct {
	path string
	file *os.File
}

func AcquireJournalPruneLock(journalDir string) (*FileLock, error) {
	lock, err := AcquireFileLock(pruneLockPath(journalDir))
	if err != nil {
		return nil, LockError{Err: fmt.Errorf("journal prune lock failed: %w", err)}
	}

	return lock, nil
}

func AcquireRestoreDBLock() (*FileLock, error) {
	return AcquireRestoreDBLockInDir("")
}

func AcquireRestoreFilesLock() (*FileLock, error) {
	return AcquireRestoreFilesLockInDir("")
}

func AcquireRestoreDBLockInDir(dir string) (*FileLock, error) {
	return AcquireFileLock(restoreLockPathInDir(dir, restoreDBLockName))
}

func AcquireRestoreFilesLockInDir(dir string) (*FileLock, error) {
	return AcquireFileLock(restoreLockPathInDir(dir, restoreFilesLockName))
}

func AcquireFileLock(path string) (*FileLock, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open lock file: %w", err)
	}

	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("acquire lock %s: %w", path, err)
	}

	if err := f.Truncate(0); err != nil {
		_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		_ = f.Close()
		return nil, fmt.Errorf("truncate lock file: %w", err)
	}
	if _, err := fmt.Fprintf(f, "%d\n", os.Getpid()); err != nil {
		_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		_ = f.Close()
		return nil, fmt.Errorf("write lock owner: %w", err)
	}

	return &FileLock{
		path: path,
		file: f,
	}, nil
}

func (l *FileLock) Release() error {
	if l == nil || l.file == nil {
		return nil
	}
	defer func() {
		_ = os.Remove(l.path)
	}()
	if err := syscall.Flock(int(l.file.Fd()), syscall.LOCK_UN); err != nil {
		_ = l.file.Close()
		return err
	}
	return l.file.Close()
}

func pruneLockPath(journalDir string) string {
	cleanDir := filepath.Clean(journalDir)
	absDir, err := filepath.Abs(cleanDir)
	if err == nil {
		cleanDir = absDir
	}

	sum := sha256.Sum256([]byte(cleanDir))
	return filepath.Join(os.TempDir(), "espops-journal-prune-"+hex.EncodeToString(sum[:8])+".lock")
}

func restoreLockPath(name string) string {
	return restoreLockPathInDir("", name)
}

func restoreLockPathInDir(dir, name string) string {
	if dir == "" {
		dir = os.TempDir()
	}

	return filepath.Join(filepath.Clean(dir), name)
}
