package ops

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestOperationLockAcquireRelease(t *testing.T) {
	root := t.TempDir()
	lock := mustAcquireScopeOperationLock(t, root, "prod")
	if err := lock.Release(); err != nil {
		t.Fatalf("release lock: %v", err)
	}
	assertScopeOperationLockAvailable(t, root, "prod")
}

func TestOperationLockSecondAcquireFailsWhileFirstHeld(t *testing.T) {
	root := t.TempDir()
	lock := mustAcquireScopeOperationLock(t, root, "prod")
	defer func() {
		if err := lock.Release(); err != nil {
			t.Fatalf("release lock: %v", err)
		}
	}()

	err := runOperationLockHelper(root, "prod")
	if err == nil {
		t.Fatal("expected helper acquire failure")
	}
	if !strings.Contains(err.Error(), `operation lock busy for scope "prod"`) {
		t.Fatalf("unexpected helper error: %v", err)
	}
}

func TestOperationLockReleaseAllowsAcquireAgain(t *testing.T) {
	root := t.TempDir()
	lock := mustAcquireScopeOperationLock(t, root, "prod")
	if err := lock.Release(); err != nil {
		t.Fatalf("release lock: %v", err)
	}

	assertScopeOperationLockAvailable(t, root, "prod")
}

func TestOperationLockKeyRejectsUnsafeValues(t *testing.T) {
	for _, value := range []string{"", ".", "..", "../prod", "prod/../dev", "prod/dev", "prod\\dev"} {
		_, err := operationLockKey(value)
		if err == nil {
			t.Fatalf("expected unsafe scope %q to fail", value)
		}
	}
}

func TestWithOperationLocksSortsKeys(t *testing.T) {
	oldAcquire := operationLockAcquireFile
	defer func() {
		operationLockAcquireFile = oldAcquire
	}()

	var acquired []string
	var released []string
	operationLockAcquireFile = func(request operationLockRequest) (operationFileLock, error) {
		acquired = append(acquired, filepath.Base(request.Path))
		return fakeOperationFileLock{
			path: filepath.Base(request.Path),
			released: func(path string) {
				released = append(released, path)
			},
		}, nil
	}

	_, err := withOperationLocks(context.Background(), []operationLockSpec{
		{ProjectDir: "/tmp/project", Scope: "prod"},
		{ProjectDir: "/tmp/project", Scope: "dev"},
	}, "test lock failed", func(ctx context.Context) (struct{}, error) {
		return struct{}{}, nil
	})
	if err != nil {
		t.Fatalf("withOperationLocks failed: %v", err)
	}
	if got := strings.Join(acquired, ","); got != "scope-dev.lock,scope-prod.lock" {
		t.Fatalf("unexpected acquire order: %s", got)
	}
	if got := strings.Join(released, ","); got != "scope-prod.lock,scope-dev.lock" {
		t.Fatalf("unexpected release order: %s", got)
	}
}

func TestWithOperationLocksFailureAcquiringSecondReleasesFirst(t *testing.T) {
	oldAcquire := operationLockAcquireFile
	defer func() {
		operationLockAcquireFile = oldAcquire
	}()

	var acquired []string
	var released []string
	operationLockAcquireFile = func(request operationLockRequest) (operationFileLock, error) {
		path := filepath.Base(request.Path)
		acquired = append(acquired, path)
		if path == "scope-prod.lock" {
			return nil, &operationLockBusyError{
				Scope: request.Scope,
				Path:  request.Path,
			}
		}
		return fakeOperationFileLock{
			path: path,
			released: func(path string) {
				released = append(released, path)
			},
		}, nil
	}

	called := false
	_, err := withOperationLocks(context.Background(), []operationLockSpec{
		{ProjectDir: "/tmp/project", Scope: "prod"},
		{ProjectDir: "/tmp/project", Scope: "dev"},
	}, "test lock failed", func(ctx context.Context) (struct{}, error) {
		called = true
		return struct{}{}, nil
	})
	assertVerifyErrorKind(t, err, ErrorKindRuntime)
	if called {
		t.Fatal("expected callback not to run")
	}
	if got := strings.Join(acquired, ","); got != "scope-dev.lock,scope-prod.lock" {
		t.Fatalf("unexpected acquire order: %s", got)
	}
	if got := strings.Join(released, ","); got != "scope-dev.lock" {
		t.Fatalf("unexpected released locks: %s", got)
	}
}

func TestWithOperationLocksSkipsAlreadyHeldScopeLock(t *testing.T) {
	oldAcquire := operationLockAcquireFile
	defer func() {
		operationLockAcquireFile = oldAcquire
	}()

	var acquired []string
	operationLockAcquireFile = func(request operationLockRequest) (operationFileLock, error) {
		acquired = append(acquired, filepath.Base(request.Path))
		return fakeOperationFileLock{}, nil
	}

	_, err := withOperationLocks(context.Background(), []operationLockSpec{
		{ProjectDir: "/tmp/project", Scope: "dev"},
		{ProjectDir: "/tmp/project", Scope: "prod"},
	}, "outer lock failed", func(ctx context.Context) (struct{}, error) {
		_, innerErr := withOperationLocks(ctx, []operationLockSpec{
			{ProjectDir: "/tmp/project", Scope: "dev"},
		}, "inner lock failed", func(context.Context) (struct{}, error) {
			return struct{}{}, nil
		})
		return struct{}{}, innerErr
	})
	if err != nil {
		t.Fatalf("withOperationLocks failed: %v", err)
	}
	if got := strings.Join(acquired, ","); got != "scope-dev.lock,scope-prod.lock" {
		t.Fatalf("unexpected nested acquire order: %s", got)
	}
}

func mustAcquireScopeOperationLock(t *testing.T, projectDir, scope string) operationFileLock {
	t.Helper()

	lockPath, err := operationLockFilePath(projectDir, scope)
	if err != nil {
		t.Fatal(err)
	}
	lock, err := acquireOperationFileLock(operationLockRequest{
		Path:  lockPath,
		Scope: scope,
	})
	if err != nil {
		t.Fatalf("acquire lock %s: %v", lockPath, err)
	}
	return lock
}

func assertScopeOperationLockAvailable(t *testing.T, projectDir, scope string) {
	t.Helper()

	if err := runOperationLockHelper(projectDir, scope); err != nil {
		t.Fatalf("expected lock availability for scope %q: %v", scope, err)
	}
}

func runOperationLockHelper(projectDir, scope string) error {
	lockPath, err := operationLockFilePath(projectDir, scope)
	if err != nil {
		return err
	}

	cmd := exec.Command(os.Args[0], "-test.run=^TestOperationLockHelperProcess$", "--", lockPath, scope)
	cmd.Env = append(os.Environ(), "GO_WANT_OPERATION_LOCK_HELPER=1")
	output, runErr := cmd.CombinedOutput()
	if runErr == nil {
		return nil
	}
	return fmt.Errorf("%s: %w", strings.TrimSpace(string(output)), runErr)
}

func TestOperationLockHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_OPERATION_LOCK_HELPER") != "1" {
		return
	}

	if len(os.Args) < 3 {
		fmt.Fprintln(os.Stderr, "missing lock arguments")
		os.Exit(2)
	}
	lockPath := os.Args[len(os.Args)-2]
	scope := os.Args[len(os.Args)-1]

	lock, err := acquireOperationFileLock(operationLockRequest{
		Path:  lockPath,
		Scope: scope,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
	if err := lock.Release(); err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
	os.Exit(0)
}

type fakeOperationFileLock struct {
	path     string
	released func(string)
}

func (f fakeOperationFileLock) Release() error {
	if f.released != nil {
		f.released(f.path)
	}
	return nil
}
