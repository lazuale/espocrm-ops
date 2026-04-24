package ops

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
)

const operationLockDirRelative = ".espops/locks"

type operationLockSpec struct {
	ProjectDir string
	Scope      string
}

type operationFileLock interface {
	Release() error
}

type operationLockRequest struct {
	Path  string
	Scope string
}

type operationLockBusyError struct {
	Scope string
	Path  string
}

func (e *operationLockBusyError) Error() string {
	if e == nil {
		return ""
	}
	return fmt.Sprintf("operation lock busy for scope %q at %s", e.Scope, e.Path)
}

type heldOperationLocksKey struct{}

var operationLockAcquireFile = acquireOperationFileLock

func WithProjectScopeOperationLocks[T any](ctx context.Context, projectDir string, scopes []string, failureMessage string, fn func(context.Context) (T, error)) (T, error) {
	specs := make([]operationLockSpec, 0, len(scopes))
	for _, scope := range scopes {
		specs = append(specs, operationLockSpec{
			ProjectDir: projectDir,
			Scope:      scope,
		})
	}
	return withOperationLocks(ctx, specs, failureMessage, fn)
}

func withOperationLocks[T any](ctx context.Context, specs []operationLockSpec, failureMessage string, fn func(context.Context) (T, error)) (result T, err error) {
	requests, markPaths, err := prepareOperationLockRequests(ctx, specs)
	if err != nil {
		return result, runtimeError(failureMessage, err)
	}

	locks := make([]operationFileLock, 0, len(requests))
	for _, request := range requests {
		lock, lockErr := operationLockAcquireFile(request)
		if lockErr != nil {
			releaseErr := releaseOperationLocks(locks)
			if releaseErr != nil {
				lockErr = errors.Join(lockErr, releaseErr)
			}
			return result, runtimeError(failureMessage, lockErr)
		}
		locks = append(locks, lock)
	}

	lockedCtx := contextWithHeldOperationLocks(ctx, markPaths)
	defer func() {
		releaseErr := releaseOperationLocks(locks)
		if releaseErr == nil {
			return
		}
		if err == nil {
			err = runtimeError(failureMessage, fmt.Errorf("release operation lock: %w", releaseErr))
			return
		}
		err = errors.Join(err, fmt.Errorf("release operation lock: %w", releaseErr))
	}()

	return fn(lockedCtx)
}

func prepareOperationLockRequests(ctx context.Context, specs []operationLockSpec) ([]operationLockRequest, []string, error) {
	requests := make([]operationLockRequest, 0, len(specs))
	seen := make(map[string]struct{}, len(specs))
	held := heldOperationLocksFromContext(ctx)
	markPaths := make([]string, 0, len(specs))

	for _, spec := range specs {
		request, err := operationLockRequestForSpec(spec)
		if err != nil {
			return nil, nil, err
		}
		if _, ok := seen[request.Path]; ok {
			continue
		}
		seen[request.Path] = struct{}{}
		markPaths = append(markPaths, request.Path)
		if _, ok := held[request.Path]; ok {
			continue
		}
		requests = append(requests, request)
	}

	slices.SortFunc(requests, func(a, b operationLockRequest) int {
		return strings.Compare(a.Path, b.Path)
	})
	slices.Sort(markPaths)
	return requests, markPaths, nil
}

func operationLockRequestForSpec(spec operationLockSpec) (operationLockRequest, error) {
	lockPath, err := operationLockFilePath(spec.ProjectDir, spec.Scope)
	if err != nil {
		return operationLockRequest{}, err
	}
	return operationLockRequest{
		Path:  lockPath,
		Scope: strings.TrimSpace(spec.Scope),
	}, nil
}

func operationLockFilePath(projectDir, scope string) (string, error) {
	projectDir = strings.TrimSpace(projectDir)
	if projectDir == "" {
		return "", fmt.Errorf("operation lock project dir is required")
	}
	key, err := operationLockKey(scope)
	if err != nil {
		return "", err
	}
	return filepath.Join(projectDir, operationLockDirRelative, key+".lock"), nil
}

func operationLockKey(scope string) (string, error) {
	scope = strings.TrimSpace(scope)
	if scope == "" {
		return "", fmt.Errorf("operation lock scope is required")
	}
	if scope == "." || scope == ".." {
		return "", fmt.Errorf("operation lock scope %q is unsafe", scope)
	}
	for _, ch := range scope {
		if ('a' <= ch && ch <= 'z') || ('A' <= ch && ch <= 'Z') || ('0' <= ch && ch <= '9') || ch == '_' || ch == '-' || ch == '.' {
			continue
		}
		return "", fmt.Errorf("operation lock scope %q is unsafe", scope)
	}
	return "scope-" + scope, nil
}

func contextWithHeldOperationLocks(ctx context.Context, paths []string) context.Context {
	if len(paths) == 0 {
		return ctx
	}

	current := heldOperationLocksFromContext(ctx)
	next := make(map[string]struct{}, len(current)+len(paths))
	for path := range current {
		next[path] = struct{}{}
	}
	for _, path := range paths {
		next[path] = struct{}{}
	}
	return context.WithValue(ctx, heldOperationLocksKey{}, next)
}

func heldOperationLocksFromContext(ctx context.Context) map[string]struct{} {
	if ctx == nil {
		return map[string]struct{}{}
	}
	held, _ := ctx.Value(heldOperationLocksKey{}).(map[string]struct{})
	if held == nil {
		return map[string]struct{}{}
	}
	return held
}

func releaseOperationLocks(locks []operationFileLock) error {
	var releaseErr error
	for i := len(locks) - 1; i >= 0; i-- {
		if locks[i] == nil {
			continue
		}
		if err := locks[i].Release(); err != nil {
			releaseErr = errors.Join(releaseErr, err)
		}
	}
	return releaseErr
}
