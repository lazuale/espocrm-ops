package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

type cliHeldLock struct {
	file *os.File
	path string
}

func holdCLIScopeLock(t *testing.T, projectDir, scope string) cliHeldLock {
	t.Helper()

	lockDir := filepath.Join(projectDir, ".espops", "locks")
	if err := os.MkdirAll(lockDir, 0o755); err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(lockDir, "scope-"+scope+".lock")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = file.Close()
		t.Fatalf("acquire cli test lock %s: %v", path, err)
	}
	return cliHeldLock{
		file: file,
		path: path,
	}
}

func (l cliHeldLock) Release(t *testing.T) {
	t.Helper()

	if l.file == nil {
		return
	}
	if err := syscall.Flock(int(l.file.Fd()), syscall.LOCK_UN); err != nil {
		t.Fatalf("unlock cli test lock %s: %v", l.path, err)
	}
	if err := l.file.Close(); err != nil {
		t.Fatalf("close cli test lock %s: %v", l.path, err)
	}
}

func cliScopeLockMessage(scope string) string {
	return fmt.Sprintf(`operation lock busy for scope %q`, scope)
}
