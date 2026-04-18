package doctor

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"

	platformfs "github.com/lazuale/espocrm-ops/internal/platform/fs"
)

type PathCheckMode string

const (
	PathCheckModeMutating PathCheckMode = "mutating"
	PathCheckModeReadOnly PathCheckMode = "read_only"
)

type dirReadiness struct {
	Path        string
	ProbePath   string
	Exists      bool
	Creatable   bool
	Writable    bool
	FreeSpaceOK bool
}

func normalizePathCheckMode(mode PathCheckMode) PathCheckMode {
	switch mode {
	case PathCheckModeReadOnly:
		return PathCheckModeReadOnly
	default:
		return PathCheckModeMutating
	}
}

func checkRuntimePath(report *Report, scope, code, label, path string, minFreeMB int, hasMinFree bool, mode PathCheckMode) {
	if mode == PathCheckModeReadOnly {
		checkRuntimePathReadOnly(report, scope, code, label, path, minFreeMB, hasMinFree)
		return
	}

	if err := platformfs.EnsureWritableDir(path); err != nil {
		report.fail(scope, code, fmt.Sprintf("%s is not writable", label), err.Error(), fmt.Sprintf("Adjust permissions for %s or choose a different path in the env file.", path))
		return
	}

	if hasMinFree {
		if err := platformfs.EnsureFreeSpace(path, uint64(minFreeMB)*1024*1024); err != nil {
			report.fail(scope, code, fmt.Sprintf("%s is below the configured free-space threshold", label), err.Error(), fmt.Sprintf("Free space under %s or lower MIN_FREE_DISK_MB intentionally after reviewing the risk.", path))
			return
		}
	}

	report.ok(scope, code, fmt.Sprintf("%s is writable", label), path)
}

func checkRuntimePathReadOnly(report *Report, scope, code, label, path string, minFreeMB int, hasMinFree bool) {
	readiness, err := inspectDirReadiness(path, minFreeMB, hasMinFree)
	if err != nil {
		report.fail(scope, code, fmt.Sprintf("%s is not ready", label), err.Error(), fmt.Sprintf("Adjust permissions for %s or choose a different path in the env file.", path))
		return
	}

	if !readiness.Writable {
		target := readiness.ProbePath
		if target == "" {
			target = readiness.Path
		}
		report.fail(scope, code, fmt.Sprintf("%s is not writable", label), fmt.Sprintf("%s is not writable", target), fmt.Sprintf("Adjust permissions for %s or choose a different path in the env file.", target))
		return
	}

	if !readiness.FreeSpaceOK {
		target := readiness.ProbePath
		if target == "" {
			target = readiness.Path
		}
		report.fail(scope, code, fmt.Sprintf("%s is below the configured free-space threshold", label), fmt.Sprintf("%s has less free space than MIN_FREE_DISK_MB requires", target), fmt.Sprintf("Free space under %s or lower MIN_FREE_DISK_MB intentionally after reviewing the risk.", target))
		return
	}

	if readiness.Exists {
		report.ok(scope, code, fmt.Sprintf("%s is writable", label), readiness.Path)
		return
	}

	report.warn(scope, code, fmt.Sprintf("%s does not exist yet", label), readiness.Path, fmt.Sprintf("The update preflight would create %s if %s stays writable.", readiness.Path, readiness.ProbePath))
}

func inspectDirReadiness(path string, minFreeMB int, hasMinFree bool) (dirReadiness, error) {
	cleanPath := filepath.Clean(path)
	if cleanPath == "." || cleanPath == "" {
		return dirReadiness{}, fmt.Errorf("path must not be blank")
	}

	fi, err := os.Stat(cleanPath)
	if err == nil {
		if !fi.IsDir() {
			return dirReadiness{}, fmt.Errorf("%s exists but is not a directory", cleanPath)
		}

		writable, err := pathWriteAccess(cleanPath)
		if err != nil {
			return dirReadiness{}, err
		}

		readiness := dirReadiness{
			Path:        cleanPath,
			ProbePath:   cleanPath,
			Exists:      true,
			Writable:    writable,
			Creatable:   writable,
			FreeSpaceOK: true,
		}

		if hasMinFree {
			if err := platformfs.EnsureFreeSpace(cleanPath, uint64(minFreeMB)*1024*1024); err != nil {
				var freeErr platformfs.InsufficientFreeSpaceError
				if errors.As(err, &freeErr) {
					readiness.FreeSpaceOK = false
					return readiness, nil
				}
				return dirReadiness{}, err
			}
		}

		return readiness, nil
	}
	if !os.IsNotExist(err) {
		return dirReadiness{}, fmt.Errorf("stat %s: %w", cleanPath, err)
	}

	parent, err := nearestExistingParent(cleanPath)
	if err != nil {
		return dirReadiness{}, err
	}

	writable, err := pathWriteAccess(parent)
	if err != nil {
		return dirReadiness{}, err
	}

	readiness := dirReadiness{
		Path:        cleanPath,
		ProbePath:   parent,
		Exists:      false,
		Writable:    writable,
		Creatable:   writable,
		FreeSpaceOK: true,
	}

	if hasMinFree {
		if err := platformfs.EnsureFreeSpace(parent, uint64(minFreeMB)*1024*1024); err != nil {
			var freeErr platformfs.InsufficientFreeSpaceError
			if errors.As(err, &freeErr) {
				readiness.FreeSpaceOK = false
				return readiness, nil
			}
			return dirReadiness{}, err
		}
	}

	return readiness, nil
}

func nearestExistingParent(path string) (string, error) {
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
