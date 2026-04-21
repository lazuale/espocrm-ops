package doctor

import (
	"fmt"
)

type PathCheckMode string

const (
	PathCheckModeMutating PathCheckMode = "mutating"
	PathCheckModeReadOnly PathCheckMode = "read_only"
)

func normalizePathCheckMode(mode PathCheckMode) PathCheckMode {
	switch mode {
	case PathCheckModeReadOnly:
		return PathCheckModeReadOnly
	default:
		return PathCheckModeMutating
	}
}

func (s Service) checkRuntimePath(report *Report, scope, code, label, path string, minFreeMB int, hasMinFree bool, mode PathCheckMode) {
	if mode == PathCheckModeReadOnly {
		s.checkRuntimePathReadOnly(report, scope, code, label, path, minFreeMB, hasMinFree)
		return
	}

	if err := s.files.EnsureWritableDir(path); err != nil {
		report.fail(scope, code, fmt.Sprintf("%s is not writable", label), err.Error(), fmt.Sprintf("Adjust permissions for %s or choose a different path in the env file.", path))
		return
	}

	if hasMinFree {
		if err := s.files.EnsureFreeSpace(path, uint64(minFreeMB)*1024*1024); err != nil {
			report.fail(scope, code, fmt.Sprintf("%s is below the configured free-space threshold", label), err.Error(), fmt.Sprintf("Free space under %s or lower MIN_FREE_DISK_MB intentionally after reviewing the risk.", path))
			return
		}
	}

	report.ok(scope, code, fmt.Sprintf("%s is writable", label), path)
}

func (s Service) checkRuntimePathReadOnly(report *Report, scope, code, label, path string, minFreeMB int, hasMinFree bool) {
	readiness, err := s.files.InspectDirReadiness(path, minFreeMB, hasMinFree)
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

	report.warn(scope, code, fmt.Sprintf("%s does not exist yet", label), readiness.Path, fmt.Sprintf("Backup or recovery preparation would create %s if %s stays writable.", readiness.Path, readiness.ProbePath))
}
