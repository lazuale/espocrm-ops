package docker

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestComposeRunningServicesDeduplicatesAndTrims(t *testing.T) {
	prependFakeDocker(t, fakeDockerOptions{
		composeRunningOutput: "db\nespocrm\n\ndb\n",
	})

	services, err := ComposeRunningServices(validComposeConfig(t))
	if err != nil {
		t.Fatalf("ComposeRunningServices failed: %v", err)
	}
	if len(services) != 2 || services[0] != "db" || services[1] != "espocrm" {
		t.Fatalf("unexpected running services: %#v", services)
	}
}

func TestValidateComposeConfigFiltersHostEnv(t *testing.T) {
	tmp := t.TempDir()
	envLogPath := filepath.Join(tmp, "docker.env")

	t.Setenv("UNRELATED_SECRET", "host-only-secret")
	prependFakeDocker(t, fakeDockerOptions{
		envLogPath: envLogPath,
	})

	if err := ValidateComposeConfig(validComposeConfig(t)); err != nil {
		t.Fatalf("ValidateComposeConfig failed: %v", err)
	}

	rawEnv, err := os.ReadFile(envLogPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(rawEnv), "UNRELATED_SECRET=host-only-secret") {
		t.Fatalf("unexpected host env leak into compose path: %s", string(rawEnv))
	}
}

func TestComposeServiceStateForReturnsHealthMessage(t *testing.T) {
	prependFakeDocker(t, fakeDockerOptions{
		runningServices: []string{"db"},
		serviceStates: map[string]string{
			"db": "unhealthy",
		},
		healthMessage: "db health failed",
	})

	state, err := ComposeServiceStateFor(validComposeConfig(t), "db")
	if err != nil {
		t.Fatalf("ComposeServiceStateFor failed: %v", err)
	}
	if state.Status != "unhealthy" {
		t.Fatalf("unexpected service status: %s", state.Status)
	}
	if state.HealthMessage != "db health failed" {
		t.Fatalf("unexpected health message: %q", state.HealthMessage)
	}
}

func TestWaitForServicesReadyFailsOnUnhealthyService(t *testing.T) {
	prependFakeDocker(t, fakeDockerOptions{
		runningServices: []string{"db"},
		serviceStates: map[string]string{
			"db": "unhealthy",
		},
		healthMessage: "db health failed",
	})

	err := WaitForServicesReady(validComposeConfig(t), 1, "db")
	if err == nil {
		t.Fatal("expected readiness failure")
	}
	if !strings.Contains(err.Error(), "db health failed") {
		t.Fatalf("expected health message in readiness failure, got %v", err)
	}
}

func TestValidateComposeConfigRejectsEmptyTargetFields(t *testing.T) {
	err := ValidateComposeConfig(ComposeConfig{})
	if err == nil {
		t.Fatal("expected compose target validation error")
	}
	if !strings.Contains(err.Error(), "compose project dir is required") {
		t.Fatalf("unexpected compose target error: %v", err)
	}
}

func validComposeConfig(t *testing.T) ComposeConfig {
	t.Helper()

	tmp := t.TempDir()
	return ComposeConfig{
		ProjectDir:  tmp,
		ComposeFile: filepath.Join(tmp, "compose.yaml"),
		EnvFile:     filepath.Join(tmp, ".env"),
	}
}
