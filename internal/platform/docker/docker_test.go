package docker

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDockerCommandEnvFiltersHostProcessEnv(t *testing.T) {
	t.Setenv("PATH", "/tmp/fake-bin")
	t.Setenv("DOCKER_TEST_TOKEN", "keep-me")
	t.Setenv("UNRELATED_SECRET", "drop-me")

	env := dockerCommandEnv("MYSQL_PWD=secret")
	dump := strings.Join(env, "\n")

	if !strings.Contains(dump, "PATH=/tmp/fake-bin") {
		t.Fatalf("expected PATH in filtered docker env, got %s", dump)
	}
	if !strings.Contains(dump, "DOCKER_TEST_TOKEN=keep-me") {
		t.Fatalf("expected DOCKER_* env in filtered docker env, got %s", dump)
	}
	if !strings.Contains(dump, "MYSQL_PWD=secret") {
		t.Fatalf("expected extra env in filtered docker env, got %s", dump)
	}
	if strings.Contains(dump, "UNRELATED_SECRET=drop-me") {
		t.Fatalf("unexpected unrelated env leak in filtered docker env: %s", dump)
	}
}

func TestCheckContainerRunningFiltersHostEnv(t *testing.T) {
	tmp := t.TempDir()
	envLogPath := filepath.Join(tmp, "docker.env")

	t.Setenv("UNRELATED_SECRET", "host-only-secret")
	prependFakeDocker(t, fakeDockerOptions{
		envLogPath:     envLogPath,
		inspectRunning: "true",
	})

	if err := CheckContainerRunning("db-container"); err != nil {
		t.Fatalf("CheckContainerRunning failed: %v", err)
	}

	rawEnv, err := os.ReadFile(envLogPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(rawEnv), "UNRELATED_SECRET=host-only-secret") {
		t.Fatalf("unexpected host env leak into inspect path: %s", string(rawEnv))
	}
}

func TestCheckContainerRunningRejectsEmptyContainer(t *testing.T) {
	err := CheckContainerRunning("   ")
	if err == nil {
		t.Fatal("expected container inspect error")
	}

	var inspectErr ContainerInspectError
	if !errors.As(err, &inspectErr) {
		t.Fatalf("expected ContainerInspectError, got %T: %v", err, err)
	}
	if !strings.Contains(err.Error(), "container is required") {
		t.Fatalf("expected explicit empty container error, got %v", err)
	}
}

func TestDetectDBClientDoesNotHideUnexpectedExecFailure(t *testing.T) {
	prependFakeDocker(t, fakeDockerOptions{
		mariaDBAvailable: true,
		mysqlAvailable:   true,
		probeStderr:      "permission denied",
		probeExitCode:    23,
	})

	_, err := DetectDBClient("db-container")
	if err == nil {
		t.Fatal("expected db client detection error")
	}

	var typedErr DBClientDetectionError
	if !errors.As(err, &typedErr) {
		t.Fatalf("expected DBClientDetectionError, got %T: %v", err, err)
	}
	if !strings.Contains(err.Error(), "permission denied") {
		t.Fatalf("expected probe failure in error, got %v", err)
	}
}
