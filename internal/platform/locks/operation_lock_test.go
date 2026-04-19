package locks

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAcquireSharedOperationLockReusesInheritedContext(t *testing.T) {
	projectDir := filepath.Join(t.TempDir(), "project")
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatal(err)
	}

	t.Setenv("ESPO_OPERATION_LOCK", "1")

	lock, err := AcquireSharedOperationLock(projectDir, "restore", nil)
	if err != nil {
		t.Fatalf("acquire inherited shared operation lock: %v", err)
	}
	if lock != nil {
		t.Fatalf("expected inherited shared operation lock to return no owned handle, got %#v", lock)
	}

	metadataPath, handlePath := predictedSharedOperationLockPaths(projectDir)
	if _, err := os.Stat(metadataPath); !os.IsNotExist(err) {
		t.Fatalf("expected no shared operation metadata to be created, got err=%v", err)
	}
	if _, err := os.Stat(handlePath); !os.IsNotExist(err) {
		t.Fatalf("expected no shared operation handle to be created, got err=%v", err)
	}
}

func TestAcquireMaintenanceLockReusesInheritedContext(t *testing.T) {
	backupRoot := filepath.Join(t.TempDir(), "backups")

	t.Setenv("ESPO_MAINTENANCE_LOCK", "1")

	lock, err := AcquireMaintenanceLock(backupRoot, "dev", "restore", nil)
	if err != nil {
		t.Fatalf("acquire inherited maintenance lock: %v", err)
	}
	if lock != nil {
		t.Fatalf("expected inherited maintenance lock to return no owned handle, got %#v", lock)
	}

	if _, err := os.Stat(filepath.Join(backupRoot, "locks")); !os.IsNotExist(err) {
		t.Fatalf("expected no maintenance lock directory to be created, got err=%v", err)
	}
}
