package ops

import (
	"context"
	"fmt"
	"strings"
	"time"

	v3config "github.com/lazuale/espocrm-ops/internal/v3/config"
)

type MigrateResult struct {
	Manifest         string
	SnapshotManifest string
}

func Migrate(ctx context.Context, fromScope string, targetCfg v3config.BackupConfig, manifestPath string, rt restoreRuntime, now time.Time) (result MigrateResult, err error) {
	if rt == nil {
		return MigrateResult{}, runtimeError("migrate runtime is required", nil)
	}
	fromScope = strings.TrimSpace(fromScope)
	if err := validateMigrateInputs(fromScope, targetCfg, manifestPath); err != nil {
		return MigrateResult{}, &VerifyError{Kind: ErrorKindUsage, Message: err.Error()}
	}

	result.Manifest = manifestPath

	verifyResult, verifyErr := VerifyBackup(ctx, manifestPath)
	if verifyErr != nil {
		return result, verifyErr
	}
	result.Manifest = verifyResult.Manifest
	if verifyResult.Scope != fromScope {
		return result, &VerifyError{
			Kind:    ErrorKindUsage,
			Message: "migrate source scope is invalid",
			Err: fmt.Errorf(
				"manifest scope %q does not match --from-scope %q",
				verifyResult.Scope,
				fromScope,
			),
		}
	}

	restoreResult, restoreErr := Restore(ctx, targetCfg, verifyResult.Manifest, rt, now)
	result.Manifest = restoreResult.Manifest
	result.SnapshotManifest = restoreResult.SnapshotManifest
	if restoreErr != nil {
		return result, restoreErr
	}

	return result, nil
}

func validateMigrateInputs(fromScope string, targetCfg v3config.BackupConfig, manifestPath string) error {
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
