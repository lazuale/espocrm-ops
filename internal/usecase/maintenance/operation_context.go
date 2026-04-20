package maintenance

import (
	"fmt"
	"io"
	"strings"

	"github.com/lazuale/espocrm-ops/internal/contract/apperr"
	platformconfig "github.com/lazuale/espocrm-ops/internal/platform/config"
	platformlocks "github.com/lazuale/espocrm-ops/internal/platform/locks"
)

type OperationContextRequest struct {
	Scope           string
	Operation       string
	ProjectDir      string
	EnvFileOverride string
	EnvContourHint  string
	LogWriter       io.Writer
}

type OperationContext struct {
	Scope          string
	Operation      string
	ProjectDir     string
	Env            platformconfig.OperationEnv
	ComposeProject string
	BackupRoot     string
	opLock         *platformlocks.OperationLock
	maintenance    *platformlocks.MaintenanceLock
}

func PrepareOperation(req OperationContextRequest) (OperationContext, error) {
	ctx := OperationContext{
		Scope:      strings.TrimSpace(req.Scope),
		Operation:  strings.TrimSpace(req.Operation),
		ProjectDir: strings.TrimSpace(req.ProjectDir),
	}

	env, err := platformconfig.LoadOperationEnv(ctx.ProjectDir, ctx.Scope, req.EnvFileOverride, req.EnvContourHint)
	if err != nil {
		return ctx, wrapOperationEnvError(err)
	}
	ctx.Env = env
	ctx.ComposeProject = env.ComposeProject()
	ctx.BackupRoot = platformconfig.ResolveProjectPath(ctx.ProjectDir, env.BackupRoot())

	if err := verifyRuntimePaths(ctx.ProjectDir, env); err != nil {
		return ctx, apperr.Wrap(apperr.KindIO, "operation_execute_failed", err)
	}

	opLock, err := platformlocks.AcquireSharedOperationLock(ctx.ProjectDir, ctx.Operation, req.LogWriter)
	if err != nil {
		return ctx, wrapOperationLockError(err)
	}
	ctx.opLock = opLock

	maintenanceLock, err := platformlocks.AcquireMaintenanceLock(ctx.BackupRoot, ctx.Scope, ctx.Operation, req.LogWriter)
	if err != nil {
		_ = ctx.Release()
		return ctx, wrapOperationLockError(err)
	}
	ctx.maintenance = maintenanceLock

	return ctx, nil
}

func (c OperationContext) ApplyEnv(base []string, extra map[string]string) []string {
	return platformconfig.ApplyOperationEnv(base, c.Env, extra)
}

func (c *OperationContext) Release() error {
	var releaseErr error

	if c == nil {
		return nil
	}

	if c.maintenance != nil {
		if err := c.maintenance.Release(); err != nil {
			releaseErr = fmt.Errorf("release maintenance lock: %w", err)
		}
		c.maintenance = nil
	}

	if c.opLock != nil {
		if err := c.opLock.Release(); err != nil {
			if releaseErr != nil {
				releaseErr = fmt.Errorf("%w; release shared operations lock: %v", releaseErr, err)
			} else {
				releaseErr = fmt.Errorf("release shared operations lock: %w", err)
			}
		}
		c.opLock = nil
	}

	return releaseErr
}
