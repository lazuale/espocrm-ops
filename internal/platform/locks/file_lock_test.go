package locks

import (
	"path/filepath"
	"testing"
)

func TestAcquireLockRejectsConcurrentHolder(t *testing.T) {
	lockPath := filepath.Join(t.TempDir(), "restore.lock")

	lock, err := AcquireFileLock(lockPath)
	if err != nil {
		t.Fatalf("first lock acquire failed: %v", err)
	}

	if second, err := AcquireFileLock(lockPath); err == nil {
		_ = second.Release()
		t.Fatal("expected second lock acquire to fail")
	}

	if err := lock.Release(); err != nil {
		t.Fatalf("release lock failed: %v", err)
	}

	if third, err := AcquireFileLock(lockPath); err != nil {
		t.Fatalf("expected lock acquire after release to succeed: %v", err)
	} else {
		_ = third.Release()
	}
}
