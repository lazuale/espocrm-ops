package fs

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

type DirReadiness struct {
	Path        string
	ProbePath   string
	Exists      bool
	Creatable   bool
	Writable    bool
	FreeSpaceOK bool
}

func InspectDirReadiness(path string, minFreeMB int, hasMinFree bool) (DirReadiness, error) {
	cleanPath := filepath.Clean(path)
	if cleanPath == "." || cleanPath == "" {
		return DirReadiness{}, fmt.Errorf("path must not be blank")
	}

	fi, err := os.Stat(cleanPath)
	if err == nil {
		if !fi.IsDir() {
			return DirReadiness{}, fmt.Errorf("%s exists but is not a directory", cleanPath)
		}

		writable, err := pathWriteAccess(cleanPath)
		if err != nil {
			return DirReadiness{}, err
		}

		readiness := DirReadiness{
			Path:        cleanPath,
			ProbePath:   cleanPath,
			Exists:      true,
			Writable:    writable,
			Creatable:   writable,
			FreeSpaceOK: true,
		}

		if hasMinFree {
			if err := EnsureFreeSpace(cleanPath, uint64(minFreeMB)*1024*1024); err != nil {
				var freeErr InsufficientFreeSpaceError
				if errors.As(err, &freeErr) {
					readiness.FreeSpaceOK = false
					return readiness, nil
				}
				return DirReadiness{}, err
			}
		}

		return readiness, nil
	}
	if !os.IsNotExist(err) {
		return DirReadiness{}, fmt.Errorf("stat %s: %w", cleanPath, err)
	}

	parent, err := NearestExistingParent(cleanPath)
	if err != nil {
		return DirReadiness{}, err
	}

	writable, err := pathWriteAccess(parent)
	if err != nil {
		return DirReadiness{}, err
	}

	readiness := DirReadiness{
		Path:        cleanPath,
		ProbePath:   parent,
		Exists:      false,
		Writable:    writable,
		Creatable:   writable,
		FreeSpaceOK: true,
	}

	if hasMinFree {
		if err := EnsureFreeSpace(parent, uint64(minFreeMB)*1024*1024); err != nil {
			var freeErr InsufficientFreeSpaceError
			if errors.As(err, &freeErr) {
				readiness.FreeSpaceOK = false
				return readiness, nil
			}
			return DirReadiness{}, err
		}
	}

	return readiness, nil
}

func NearestExistingParent(path string) (string, error) {
	current := filepath.Clean(path)
	for {
		current = filepath.Dir(current)
		fi, err := os.Stat(current)
		if err == nil {
			if !fi.IsDir() {
				return "", fmt.Errorf("%s exists but is not a directory", current)
			}
			return current, nil
		}
		if !os.IsNotExist(err) {
			return "", fmt.Errorf("stat %s: %w", current, err)
		}
		if next := filepath.Dir(current); next == current {
			return "", fmt.Errorf("could not find an existing parent directory for %s", path)
		}
	}
}

func pathWriteAccess(path string) (bool, error) {
	const writeAndTraverse = 0x2 | 0x1

	err := syscall.Access(path, writeAndTraverse)
	switch {
	case err == nil:
		return true, nil
	case errors.Is(err, syscall.EACCES), errors.Is(err, syscall.EPERM):
		return false, nil
	default:
		return false, fmt.Errorf("check write access for %s: %w", path, err)
	}
}
