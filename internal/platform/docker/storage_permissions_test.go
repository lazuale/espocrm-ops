package docker

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveEspoRuntimeOwnerUsesFilteredEnvAndExplicitShellSeam(t *testing.T) {
	tmp := t.TempDir()
	logPath := filepath.Join(tmp, "docker.log")
	envLogPath := filepath.Join(tmp, "docker.env")

	t.Setenv("UNRELATED_SECRET", "host-only-secret")
	prependFakeDocker(t, fakeDockerOptions{
		logPath:         logPath,
		envLogPath:      envLogPath,
		availableImages: []string{"espocrm/espocrm:9.3.4-apache"},
		runtimeOwner:    "1000:1001",
	})

	uid, gid, err := ResolveEspoRuntimeOwner("espocrm/espocrm:9.3.4-apache")
	if err != nil {
		t.Fatalf("ResolveEspoRuntimeOwner failed: %v", err)
	}
	if uid != 1000 || gid != 1001 {
		t.Fatalf("unexpected runtime owner: %d:%d", uid, gid)
	}

	rawLog, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(rawLog), "run --pull=never --rm --user 0:0 --entrypoint sh espocrm/espocrm:9.3.4-apache -euc") {
		t.Fatalf("expected explicit shell runtime owner probe, got %s", string(rawLog))
	}

	rawEnv, err := os.ReadFile(envLogPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(rawEnv), "UNRELATED_SECRET=host-only-secret") {
		t.Fatalf("unexpected host env leak into runtime owner probe: %s", string(rawEnv))
	}
}

func TestResolveEspoRuntimeOwnerRejectsMalformedOwner(t *testing.T) {
	prependFakeDocker(t, fakeDockerOptions{
		availableImages: []string{"espocrm/espocrm:9.3.4-apache"},
		runtimeOwner:    "oops",
	})

	_, _, err := ResolveEspoRuntimeOwner("espocrm/espocrm:9.3.4-apache")
	if err == nil {
		t.Fatal("expected malformed runtime owner error")
	}
	if !strings.Contains(err.Error(), "uid:gid format") {
		t.Fatalf("unexpected runtime owner error: %v", err)
	}
}

func TestReconcileEspoStoragePermissionsUsesLocalHelperImageAndFilteredEnv(t *testing.T) {
	tmp := t.TempDir()
	targetDir := filepath.Join(tmp, "storage")
	dataDir := filepath.Join(targetDir, "data")
	dataFile := filepath.Join(dataDir, "test.txt")
	logPath := filepath.Join(tmp, "docker.log")
	envLogPath := filepath.Join(tmp, "docker.env")

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
		availableImages: []string{"espocrm/espocrm:9.3.4-apache"},
		runtimeOwner:    "33:33",
	})

	if err := ReconcileEspoStoragePermissions(targetDir, "10.11", "espocrm/espocrm:9.3.4-apache"); err != nil {
		t.Fatalf("ReconcileEspoStoragePermissions failed: %v", err)
	}

	rawLog, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	log := string(rawLog)
	if !strings.Contains(log, "image inspect espocrm/espocrm:9.3.4-apache") {
		t.Fatalf("expected runtime/helper image probe, got %s", log)
	}
	if !strings.Contains(log, "-e ESPO_RUNTIME_UID -e ESPO_RUNTIME_GID espocrm/espocrm:9.3.4-apache -euc") {
		t.Fatalf("expected explicit env handoff into helper shell, got %s", log)
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

func mustPermString(t *testing.T, path string) string {
	t.Helper()

	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	return fmt.Sprintf("%o", fi.Mode().Perm())
}
