package operation

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/lazuale/espocrm-ops/internal/testutil"
)

func TestPrepareOperationDoesNotPrecreateRuntimeDirs(t *testing.T) {
	projectDir := newOperationProject(t, "prod")

	ctx, err := testService().PrepareOperation(OperationContextRequest{
		Scope:      "prod",
		Operation:  "backup",
		ProjectDir: projectDir,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = ctx.Release()
	}()

	for _, path := range []string{
		filepath.Join(projectDir, "runtime", "prod", "db"),
		filepath.Join(projectDir, "runtime", "prod", "espo"),
		filepath.Join(projectDir, "backups", "prod", "db"),
		filepath.Join(projectDir, "backups", "prod", "files"),
		filepath.Join(projectDir, "backups", "prod", "manifests"),
	} {
		if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
			t.Fatalf("expected %s to stay absent after preflight, got err=%v", path, statErr)
		}
	}

	locksDir := filepath.Join(projectDir, "backups", "prod", "locks")
	if _, err := os.Stat(locksDir); err != nil {
		t.Fatalf("expected maintenance lock directory %s: %v", locksDir, err)
	}
}

func TestPrepareOperation_BackupAllowsReadOnlyRuntimeStorage(t *testing.T) {
	projectDir := newOperationProject(t, "prod")
	makeOperationRuntimeReadOnly(t, projectDir, "prod")

	ctx, err := testService().PrepareOperation(OperationContextRequest{
		Scope:      "prod",
		Operation:  "backup",
		ProjectDir: projectDir,
	})
	if err != nil {
		t.Fatalf("backup preflight should ignore read-only runtime storage: %v", err)
	}
	defer func() {
		_ = ctx.Release()
	}()
}

func TestPrepareOperation_RestoreStillRequiresWritableRuntimeStorage(t *testing.T) {
	projectDir := newOperationProject(t, "prod")
	makeOperationRuntimeReadOnly(t, projectDir, "prod")

	if _, err := testService().PrepareOperation(OperationContextRequest{
		Scope:      "prod",
		Operation:  "restore",
		ProjectDir: projectDir,
	}); err == nil {
		t.Fatal("expected restore preflight to require writable runtime storage")
	}
}

func TestPrepareOperationRejectsInvalidRuntimeContract(t *testing.T) {
	projectDir := newOperationProject(t, "prod")
	envPath := filepath.Join(projectDir, ".env.prod")

	raw, err := os.ReadFile(envPath)
	if err != nil {
		t.Fatal(err)
	}
	updated := strings.Replace(string(raw), "ESPO_RUNTIME_UID=33\n", "ESPO_RUNTIME_UID=oops\n", 1)
	if err := os.WriteFile(envPath, []byte(updated), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := testService().PrepareOperation(OperationContextRequest{
		Scope:      "prod",
		Operation:  "backup",
		ProjectDir: projectDir,
	}); err == nil {
		t.Fatal("expected runtime contract validation failure")
	} else if !strings.Contains(err.Error(), "ESPO_RUNTIME_UID must be an integer") {
		t.Fatalf("unexpected runtime contract error: %v", err)
	}
}

func newOperationProject(t *testing.T, scope string) string {
	t.Helper()

	projectDir := filepath.Join(t.TempDir(), "project")
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatal(err)
	}

	values := testutil.BaseEnvValues(scope)

	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	lines := make([]string, 0, len(keys))
	for _, key := range keys {
		lines = append(lines, key+"="+values[key])
	}

	envPath := filepath.Join(projectDir, ".env."+scope)
	if err := os.WriteFile(envPath, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	return projectDir
}

func makeOperationRuntimeReadOnly(t *testing.T, projectDir, scope string) {
	t.Helper()

	runtimeDir := filepath.Join(projectDir, "runtime", scope)
	if err := os.MkdirAll(runtimeDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(runtimeDir, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = os.Chmod(runtimeDir, 0o755)
	})
}
