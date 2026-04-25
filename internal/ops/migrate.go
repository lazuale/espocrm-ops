package ops

import (
	"context"
	"fmt"
	"path/filepath"
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

	lockSpecs := migrateLockSpecs(fromScope, targetCfg, manifestPath)
	restoreResult, restoreErr := withOperationLocks(ctx, lockSpecs, "migrate lock failed", func(lockedCtx context.Context) (RestoreResult, error) {
		return restoreWithOptionsLocked(lockedCtx, targetCfg, manifestPath, rt, now, restoreOptions{
			allowedSourceScope: fromScope,
			scopeErrorMessage:  "migrate source scope is invalid",
		})
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

func migrateLockSpecs(fromScope string, targetCfg config.BackupConfig, manifestPath string) []operationLockSpec {
	specs := []operationLockSpec{{
		ProjectDir: targetCfg.ProjectDir,
		Scope:      targetCfg.Scope,
	}}

	sourceCfg, err := config.LoadBackup(config.BackupRequest{
		Scope:      fromScope,
		ProjectDir: targetCfg.ProjectDir,
	})
	if err != nil {
		return specs
	}
	if !pathInsideRoot(manifestPath, sourceCfg.BackupRoot) {
		return specs
	}

	return append(specs, operationLockSpec{
		ProjectDir: sourceCfg.ProjectDir,
		Scope:      sourceCfg.Scope,
	})
}

func pathInsideRoot(path, root string) bool {
	pathAbs, err := filepath.Abs(filepath.Clean(strings.TrimSpace(path)))
	if err != nil {
		return false
	}
	rootAbs, err := filepath.Abs(filepath.Clean(strings.TrimSpace(root)))
	if err != nil {
		return false
	}
	return pathWithinRoot(rootAbs, pathAbs)
}
