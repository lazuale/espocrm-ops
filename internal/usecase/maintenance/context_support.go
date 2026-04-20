package maintenance

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/lazuale/espocrm-ops/internal/contract/apperr"
	platformconfig "github.com/lazuale/espocrm-ops/internal/platform/config"
	platformlocks "github.com/lazuale/espocrm-ops/internal/platform/locks"
)

func ensureRuntimeDirs(projectDir string, env platformconfig.OperationEnv) error {
	paths := []string{
		platformconfig.ResolveProjectPath(projectDir, env.DBStorageDir()),
		platformconfig.ResolveProjectPath(projectDir, env.ESPOStorageDir()),
		filepath.Join(platformconfig.ResolveProjectPath(projectDir, env.BackupRoot()), "db"),
		filepath.Join(platformconfig.ResolveProjectPath(projectDir, env.BackupRoot()), "files"),
		filepath.Join(platformconfig.ResolveProjectPath(projectDir, env.BackupRoot()), "locks"),
		filepath.Join(platformconfig.ResolveProjectPath(projectDir, env.BackupRoot()), "manifests"),
		filepath.Join(platformconfig.ResolveProjectPath(projectDir, env.BackupRoot()), "reports"),
		filepath.Join(platformconfig.ResolveProjectPath(projectDir, env.BackupRoot()), "support"),
	}

	for _, path := range paths {
		if err := os.MkdirAll(path, 0o755); err != nil {
			return fmt.Errorf("create runtime directory %s: %w", path, err)
		}
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
