//go:build unix

package ops

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

type osProjectFileLock struct {
	file *os.File
	path string
}

func acquireProjectFileLock(path string) (projectFileLock, error) {
	lockDir := filepath.Dir(path)
	if err := os.MkdirAll(lockDir, 0o755); err != nil {
		return nil, fmt.Errorf("create project lock directory %s: %w", lockDir, err)
	}

	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open project lock file %s: %w", path, err)
	}

	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = file.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) || errors.Is(err, syscall.EAGAIN) {
			return nil, &projectLockBusyError{Path: path}
		}
		return nil, fmt.Errorf("acquire project lock %s: %w", path, err)
	}

	return &osProjectFileLock{file: file, path: path}, nil
}

func (l *osProjectFileLock) Release() error {
	if l == nil || l.file == nil {
		return nil
	}

	unlockErr := syscall.Flock(int(l.file.Fd()), syscall.LOCK_UN)
	closeErr := l.file.Close()
	l.file = nil

	switch {
	case unlockErr != nil && closeErr != nil:
		return errors.Join(
			fmt.Errorf("unlock project lock %s: %w", l.path, unlockErr),
			fmt.Errorf("close project lock %s: %w", l.path, closeErr),
		)
	case unlockErr != nil:
		return fmt.Errorf("unlock project lock %s: %w", l.path, unlockErr)
	case closeErr != nil:
		return fmt.Errorf("close project lock %s: %w", l.path, closeErr)
	default:
		return nil
	}
}
