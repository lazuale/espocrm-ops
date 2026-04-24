//go:build unix

package ops

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

type osOperationFileLock struct {
	file *os.File
	path string
}

func acquireOperationFileLock(request operationLockRequest) (operationFileLock, error) {
	lockDir := filepath.Dir(request.Path)
	if err := os.MkdirAll(lockDir, 0o755); err != nil {
		return nil, fmt.Errorf("create operation lock directory %s: %w", lockDir, err)
	}

	file, err := os.OpenFile(request.Path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open operation lock file %s: %w", request.Path, err)
	}

	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = file.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) || errors.Is(err, syscall.EAGAIN) {
			return nil, &operationLockBusyError{
				Scope: request.Scope,
				Path:  request.Path,
			}
		}
		return nil, fmt.Errorf("acquire operation lock %s: %w", request.Path, err)
	}

	return &osOperationFileLock{
		file: file,
		path: request.Path,
	}, nil
}

func (l *osOperationFileLock) Release() error {
	if l == nil || l.file == nil {
		return nil
	}

	unlockErr := syscall.Flock(int(l.file.Fd()), syscall.LOCK_UN)
	closeErr := l.file.Close()
	l.file = nil

	switch {
	case unlockErr != nil && closeErr != nil:
		return errors.Join(
			fmt.Errorf("unlock operation lock %s: %w", l.path, unlockErr),
			fmt.Errorf("close operation lock %s: %w", l.path, closeErr),
		)
	case unlockErr != nil:
		return fmt.Errorf("unlock operation lock %s: %w", l.path, unlockErr)
	case closeErr != nil:
		return fmt.Errorf("close operation lock %s: %w", l.path, closeErr)
	default:
		return nil
	}
}
