package operation

import (
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/lazuale/espocrm-ops/internal/app/ports"
	domainenv "github.com/lazuale/espocrm-ops/internal/domain/env"
	domainfailure "github.com/lazuale/espocrm-ops/internal/domain/failure"
)

type OperationContextRequest struct {
	Scope           string
	Operation       string
	ProjectDir      string
	EnvFileOverride string
	LogWriter       io.Writer
}

type Dependencies struct {
	Env   ports.EnvLoader
	Files ports.Files
	Locks ports.Locks
}

type Service struct {
	env   ports.EnvLoader
	files ports.Files
	locks ports.Locks
}

type OperationContext struct {
	Scope          string
	Operation      string
	ProjectDir     string
	Env            domainenv.OperationEnv
	ComposeProject string
	BackupRoot     string
	opLock         ports.Releaser
	maintenance    ports.Releaser
}

func NewService(deps Dependencies) Service {
	return Service{
		env:   deps.Env,
		files: deps.Files,
		locks: deps.Locks,
	}
}

func (s Service) PrepareOperation(req OperationContextRequest) (OperationContext, error) {
	ctx := OperationContext{
		Scope:      strings.TrimSpace(req.Scope),
		Operation:  strings.TrimSpace(req.Operation),
		ProjectDir: strings.TrimSpace(req.ProjectDir),
	}

	env, err := s.env.LoadOperationEnv(ctx.ProjectDir, ctx.Scope, req.EnvFileOverride)
	if err != nil {
		return ctx, classifyOperationEnvError(err)
	}
	ctx.Env = env
	ctx.ComposeProject = env.ComposeProject()
	ctx.BackupRoot = s.env.ResolveProjectPath(ctx.ProjectDir, env.BackupRoot())

	if err := s.verifyRuntimePaths(ctx.ProjectDir, env); err != nil {
		return ctx, domainfailure.Failure{
			Kind: domainfailure.KindIO,
			Code: "operation_execute_failed",
			Err:  err,
		}
	}

	opLock, err := s.locks.AcquireSharedOperationLock(ctx.ProjectDir, ctx.Operation, req.LogWriter)
	if err != nil {
		return ctx, classifyOperationLockError(err)
	}
	ctx.opLock = opLock

	maintenanceLock, err := s.locks.AcquireMaintenanceLock(ctx.BackupRoot, ctx.Scope, ctx.Operation, req.LogWriter)
	if err != nil {
		_ = ctx.Release()
		return ctx, classifyOperationLockError(err)
	}
	ctx.maintenance = maintenanceLock

	return ctx, nil
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

func classifyOperationEnvError(err error) error {
	var failure domainfailure.Failure
	if errors.As(err, &failure) {
		return failure
	}

	return domainfailure.Failure{
		Kind: domainfailure.KindIO,
		Code: "operation_execute_failed",
		Err:  err,
	}
}

func classifyOperationLockError(err error) error {
	var failure domainfailure.Failure
	if errors.As(err, &failure) {
		return failure
	}

	return domainfailure.Failure{
		Kind: domainfailure.KindIO,
		Code: "operation_execute_failed",
		Err:  err,
	}
}
