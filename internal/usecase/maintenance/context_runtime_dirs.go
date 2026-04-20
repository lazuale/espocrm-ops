package maintenance

import (
	"fmt"

	"github.com/lazuale/espocrm-ops/internal/contract/apperr"
	platformconfig "github.com/lazuale/espocrm-ops/internal/platform/config"
	platformfs "github.com/lazuale/espocrm-ops/internal/platform/fs"
	platformlocks "github.com/lazuale/espocrm-ops/internal/platform/locks"
)

func verifyRuntimePaths(projectDir string, env platformconfig.OperationEnv) error {
	paths := []string{
		platformconfig.ResolveProjectPath(projectDir, env.DBStorageDir()),
		platformconfig.ResolveProjectPath(projectDir, env.ESPOStorageDir()),
		platformconfig.ResolveProjectPath(projectDir, env.BackupRoot()),
	}

	for _, path := range paths {
		readiness, err := platformfs.InspectDirReadiness(path, 0, false)
		if err != nil {
			return fmt.Errorf("inspect runtime path %s: %w", path, err)
		}
		if readiness.Writable {
			continue
		}

		target := readiness.Path
		if readiness.ProbePath != "" {
			target = readiness.ProbePath
		}
		return fmt.Errorf("runtime path %s is not writable via %s", readiness.Path, target)
	}

	return nil
}

func wrapOperationEnvError(err error) error {
	switch err.(type) {
	case platformconfig.MissingEnvFileError, platformconfig.InvalidEnvFileError, platformconfig.EnvParseError, platformconfig.MissingEnvValueError, platformconfig.UnsupportedContourError:
		return apperr.Wrap(apperr.KindValidation, "operation_execute_failed", err)
	default:
		return apperr.Wrap(apperr.KindIO, "operation_execute_failed", err)
	}
}

func wrapOperationLockError(err error) error {
	switch err.(type) {
	case platformlocks.MaintenanceConflictError, platformlocks.LegacyMetadataOnlyLockError:
		return apperr.Wrap(apperr.KindConflict, "operation_execute_failed", err)
	default:
		return apperr.Wrap(apperr.KindIO, "operation_execute_failed", err)
	}
}
