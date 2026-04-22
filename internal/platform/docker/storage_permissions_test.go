package docker

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReconcileEspoStoragePermissionsUsesExplicitHelperContractAndFilteredEnv(t *testing.T) {
	tmp := t.TempDir()
	targetDir := filepath.Join(tmp, "storage")
	dataDir := filepath.Join(targetDir, "data")
	dataFile := filepath.Join(dataDir, "test.txt")
	logPath := filepath.Join(tmp, "docker.log")
	envLogPath := filepath.Join(tmp, "docker.env")
	helperImage := "registry.example.com/espops-helper:1.0"

	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dataFile, []byte("hello\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	t.Setenv("UNRELATED_SECRET", "host-only-secret")
	prependFakeDocker(t, fakeDockerOptions{
		logPath:         logPath,
		envLogPath:      envLogPath,
		availableImages: []string{helperImage},
	})

	if err := ReconcileEspoStoragePermissions(targetDir, helperImage, 33, 33); err != nil {
		t.Fatalf("ReconcileEspoStoragePermissions failed: %v", err)
	}

	rawLog, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	log := string(rawLog)
	if !strings.Contains(log, "image inspect "+helperImage) {
		t.Fatalf("expected explicit helper image probe, got %s", log)
	}
	if !strings.Contains(log, "-e ESPO_RUNTIME_UID -e ESPO_RUNTIME_GID "+helperImage+" -euc") {
		t.Fatalf("expected explicit env handoff into helper shell, got %s", log)
	}
	if strings.Contains(log, "/var/www/html/") || strings.Contains(log, "ls -nd") {
		t.Fatalf("permission reconcile must not probe runtime image layout, got %s", log)
	}

	rawEnv, err := os.ReadFile(envLogPath)
	if err != nil {
		t.Fatal(err)
	}
	envDump := string(rawEnv)
	if !strings.Contains(envDump, "ESPO_RUNTIME_UID=33") || !strings.Contains(envDump, "ESPO_RUNTIME_GID=33") {
		t.Fatalf("expected runtime uid/gid env in helper run, got %s", envDump)
	}
	if strings.Contains(envDump, "UNRELATED_SECRET=host-only-secret") {
		t.Fatalf("unexpected host env leak into helper run: %s", envDump)
	}

	if mode := mustPermString(t, dataDir); mode != "775" {
		t.Fatalf("unexpected data dir mode: %s", mode)
	}
	if mode := mustPermString(t, dataFile); mode != "664" {
		t.Fatalf("unexpected data file mode: %s", mode)
	}
}

func TestReconcileEspoStoragePermissionsFailsClosedWhenHelperImageIsMissing(t *testing.T) {
	targetDir := t.TempDir()

	err := ReconcileEspoStoragePermissions(targetDir, "registry.example.com/espops-helper:1.0", 33, 33)
	if err == nil {
		t.Fatal("expected missing helper image error")
	}
	if !strings.Contains(err.Error(), "is not available locally") {
		t.Fatalf("unexpected missing helper image error: %v", err)
	}
}

func TestReconcileEspoStoragePermissionsRejectsNegativeRuntimeOwner(t *testing.T) {
	err := ReconcileEspoStoragePermissions(t.TempDir(), "registry.example.com/espops-helper:1.0", -1, 33)
	if err == nil {
		t.Fatal("expected negative runtime uid error")
	}
	if !strings.Contains(err.Error(), "ESPO_RUNTIME_UID must be non-negative") {
		t.Fatalf("unexpected runtime uid error: %v", err)
	}
}

func mustPermString(t *testing.T, path string) string {
	t.Helper()

	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	return fmt.Sprintf("%o", fi.Mode().Perm())
}
