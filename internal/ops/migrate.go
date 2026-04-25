package ops

import (
	"context"
	"fmt"
	"strings"
	"time"

	config "github.com/lazuale/espocrm-ops/internal/config"
)

type MigrateResult struct {
	Manifest         string
	SnapshotManifest string
}

func Migrate(ctx context.Context, fromScope string, targetCfg config.BackupConfig, manifestPath string, rt restoreRuntime, now time.Time) (result MigrateResult, err error) {
	if rt == nil {
		return MigrateResult{}, runtimeError("migrate runtime is required", nil)
	}
	fromScope = strings.TrimSpace(fromScope)
	if err := validateMigrateInputs(fromScope, targetCfg, manifestPath); err != nil {
		return MigrateResult{}, &VerifyError{Kind: ErrorKindUsage, Message: err.Error()}
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	now = now.UTC()

	lockSpecs := []operationLockSpec{
		{ProjectDir: targetCfg.ProjectDir, Scope: fromScope},
		{ProjectDir: targetCfg.ProjectDir, Scope: targetCfg.Scope},
	}
	restoreResult, restoreErr := withOperationLocks(ctx, lockSpecs, "migrate lock failed", func(lockedCtx context.Context) (RestoreResult, error) {
		return restoreWithAllowedSourceScopeLocked(lockedCtx, targetCfg, manifestPath, rt, now, fromScope)
	})
	result.Manifest = restoreResult.Manifest
	result.SnapshotManifest = restoreResult.SnapshotManifest
	if restoreErr != nil {
		return result, restoreErr
	}

	return result, nil
}

func validateMigrateInputs(fromScope string, targetCfg config.BackupConfig, manifestPath string) error {
	fromScope = strings.TrimSpace(fromScope)
	switch fromScope {
	case "":
		return fmt.Errorf("--from-scope is required")
	case "dev", "prod":
	default:
		return fmt.Errorf("--from-scope must be dev or prod")
	}

	if err := validateBackupConfig(targetCfg); err != nil {
		return err
	}
	if fromScope == targetCfg.Scope {
		return fmt.Errorf("--from-scope and --to-scope must differ")
	}
	if strings.TrimSpace(manifestPath) == "" {
		return fmt.Errorf("--manifest is required")
	}

	return nil
}
