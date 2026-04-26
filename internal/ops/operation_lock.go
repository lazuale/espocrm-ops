package ops

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
)

const projectLockPathRelative = ".espops/project.lock"

type projectFileLock interface {
	Release() error
}

type projectLockBusyError struct {
	Path string
}

func (e *projectLockBusyError) Error() string {
	if e == nil {
		return ""
	}
	return fmt.Sprintf("project lock busy at %s", e.Path)
}

func withProjectLock[T any](ctx context.Context, projectDir string, fn func(context.Context) (T, error)) (result T, err error) {
	if err := ctx.Err(); err != nil {
		return result, ioError("operation interrupted", err)
	}
	lockPath, err := projectLockFilePath(projectDir)
	if err != nil {
		return result, runtimeError("project lock failed", err)
	}
	lock, err := acquireProjectFileLock(lockPath)
	if err != nil {
		return result, runtimeError("project lock failed", err)
	}
	defer func() {
		releaseErr := lock.Release()
		if releaseErr == nil {
			return
		}
		if err == nil {
			err = runtimeError("project lock failed", fmt.Errorf("release project lock: %w", releaseErr))
			return
		}
		err = errors.Join(err, fmt.Errorf("release project lock: %w", releaseErr))
	}()

	return fn(ctx)
}

func projectLockFilePath(projectDir string) (string, error) {
	projectDir = strings.TrimSpace(projectDir)
	if projectDir == "" {
		return "", fmt.Errorf("project dir is required")
	}
	return filepath.Join(projectDir, projectLockPathRelative), nil
}
