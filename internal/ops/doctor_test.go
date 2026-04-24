package ops

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	config "github.com/lazuale/espocrm-ops/internal/config"
	runtime "github.com/lazuale/espocrm-ops/internal/runtime"
)

func TestDoctorMissingEnvFails(t *testing.T) {
	projectDir := doctorProjectDir(t, []string{
		"ESPO_CONTOUR=prod",
		"BACKUP_ROOT=./backups/prod",
		"ESPO_STORAGE_DIR=./runtime/prod/espo",
		"APP_SERVICES=espocrm,espocrm-daemon,espocrm-websocket",
		"DB_SERVICE=db",
		"DB_USER=espocrm",
		"DB_PASSWORD=db-secret",
	}, true)

	rt := &fakeDoctorRuntime{}
	result, err := Doctor(context.Background(), config.BackupRequest{
		Scope:      "prod",
		ProjectDir: projectDir,
	}, rt)
	assertVerifyErrorKind(t, err, ErrorKindUsage)
	if !strings.Contains(err.Error(), "DB_NAME is required") {
		t.Fatalf("unexpected error: %v", err)
	}
	assertDoctorChecks(t, result.Checks,
		DoctorCheck{Name: "config", OK: false},
	)
	if err := rt.requireCalls(); err != nil {
		t.Fatal(err)
	}
}

func TestDoctorBadComposeFileFails(t *testing.T) {
	projectDir := doctorProjectDir(t, validDoctorEnv(), true)

	rt := &fakeDoctorRuntime{
		composeErr: errf("compose failed"),
	}
	result, err := Doctor(context.Background(), config.BackupRequest{
		Scope:      "prod",
		ProjectDir: projectDir,
	}, rt)
	assertVerifyErrorKind(t, err, ErrorKindRuntime)
	if !strings.Contains(err.Error(), "doctor compose config check failed") {
		t.Fatalf("unexpected error: %v", err)
	}
	assertDoctorChecks(t, result.Checks,
		DoctorCheck{Name: "config", OK: true},
		DoctorCheck{Name: "backup_root", OK: true},
		DoctorCheck{Name: "storage_dir", OK: true},
		DoctorCheck{Name: "compose_config", OK: false},
	)
	if err := rt.requireCalls("compose_config"); err != nil {
		t.Fatal(err)
	}
}

func TestDoctorBackupRootNotWritableFails(t *testing.T) {
	projectDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(projectDir, "compose.yaml"), []byte("services: {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, ".env.prod"), []byte(strings.Join([]string{
		"ESPO_CONTOUR=prod",
		"BACKUP_ROOT=./blocked/root",
		"ESPO_STORAGE_DIR=./runtime/prod/espo",
		"APP_SERVICES=espocrm,espocrm-daemon,espocrm-websocket",
		"DB_SERVICE=db",
		"DB_USER=espocrm",
		"DB_PASSWORD=db-secret",
		"DB_NAME=espocrm",
		"",
	}, "\n")), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, "blocked"), []byte("not a directory"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(projectDir, "runtime", "prod", "espo"), 0o755); err != nil {
		t.Fatal(err)
	}

	rt := &fakeDoctorRuntime{}
	result, err := Doctor(context.Background(), config.BackupRequest{
		Scope:      "prod",
		ProjectDir: projectDir,
	}, rt)
	assertVerifyErrorKind(t, err, ErrorKindIO)
	assertDoctorChecks(t, result.Checks,
		DoctorCheck{Name: "config", OK: true},
		DoctorCheck{Name: "backup_root", OK: false},
	)
	if err := rt.requireCalls(); err != nil {
		t.Fatal(err)
	}
}

func TestDoctorMissingBackupRootFailsAndDoesNotCreateDirectory(t *testing.T) {
	projectDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(projectDir, "compose.yaml"), []byte("services: {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, ".env.prod"), []byte(strings.Join([]string{
		"ESPO_CONTOUR=prod",
		"BACKUP_ROOT=./backups/prod",
		"ESPO_STORAGE_DIR=./runtime/prod/espo",
		"APP_SERVICES=espocrm,espocrm-daemon,espocrm-websocket",
		"DB_SERVICE=db",
		"DB_USER=espocrm",
		"DB_PASSWORD=db-secret",
		"DB_NAME=espocrm",
		"",
	}, "\n")), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(projectDir, "runtime", "prod", "espo"), 0o755); err != nil {
		t.Fatal(err)
	}

	rt := &fakeDoctorRuntime{}
	result, err := Doctor(context.Background(), config.BackupRequest{
		Scope:      "prod",
		ProjectDir: projectDir,
	}, rt)
	assertVerifyErrorKind(t, err, ErrorKindIO)
	if !strings.Contains(err.Error(), "doctor backup root check failed") {
		t.Fatalf("unexpected error: %v", err)
	}
	assertDoctorChecks(t, result.Checks,
		DoctorCheck{Name: "config", OK: true},
		DoctorCheck{Name: "backup_root", OK: false},
	)
	if err := rt.requireCalls(); err != nil {
		t.Fatal(err)
	}
	if _, statErr := os.Stat(filepath.Join(projectDir, "backups", "prod")); !os.IsNotExist(statErr) {
		t.Fatalf("expected backup root to remain absent, got %v", statErr)
	}
}

func TestDoctorStorageDirMissingFails(t *testing.T) {
	projectDir := doctorProjectDir(t, validDoctorEnv(), false)

	rt := &fakeDoctorRuntime{}
	result, err := Doctor(context.Background(), config.BackupRequest{
		Scope:      "prod",
		ProjectDir: projectDir,
	}, rt)
	assertVerifyErrorKind(t, err, ErrorKindIO)
	if !strings.Contains(err.Error(), "doctor storage dir check failed") {
		t.Fatalf("unexpected error: %v", err)
	}
	assertDoctorChecks(t, result.Checks,
		DoctorCheck{Name: "config", OK: true},
		DoctorCheck{Name: "backup_root", OK: true},
		DoctorCheck{Name: "storage_dir", OK: false},
	)
	if err := rt.requireCalls(); err != nil {
		t.Fatal(err)
	}
}

func TestDoctorMissingDBServiceInPSFails(t *testing.T) {
	projectDir := doctorProjectDir(t, validDoctorEnv(), true)

	rt := &fakeDoctorRuntime{
		services: []runtime.Service{
			{Name: "espocrm"},
			{Name: "espocrm-daemon"},
			{Name: "espocrm-websocket"},
		},
	}
	result, err := Doctor(context.Background(), config.BackupRequest{
		Scope:      "prod",
		ProjectDir: projectDir,
	}, rt)
	assertVerifyErrorKind(t, err, ErrorKindRuntime)
	if !strings.Contains(err.Error(), `db service "db" not found`) {
		t.Fatalf("unexpected error: %v", err)
	}
	assertDoctorChecks(t, result.Checks,
		DoctorCheck{Name: "config", OK: true},
		DoctorCheck{Name: "backup_root", OK: true},
		DoctorCheck{Name: "storage_dir", OK: true},
		DoctorCheck{Name: "compose_config", OK: true},
		DoctorCheck{Name: "services", OK: false},
	)
	if err := rt.requireCalls("compose_config", "services"); err != nil {
		t.Fatal(err)
	}
}

func TestDoctorMissingAppServiceInPSFails(t *testing.T) {
	projectDir := doctorProjectDir(t, validDoctorEnv(), true)

	rt := &fakeDoctorRuntime{
		services: []runtime.Service{
			{Name: "db"},
			{Name: "espocrm"},
			{Name: "espocrm-websocket"},
		},
	}
	result, err := Doctor(context.Background(), config.BackupRequest{
		Scope:      "prod",
		ProjectDir: projectDir,
	}, rt)
	assertVerifyErrorKind(t, err, ErrorKindRuntime)
	if !strings.Contains(err.Error(), `app service "espocrm-daemon" not found`) {
		t.Fatalf("unexpected error: %v", err)
	}
	assertDoctorChecks(t, result.Checks,
		DoctorCheck{Name: "config", OK: true},
		DoctorCheck{Name: "backup_root", OK: true},
		DoctorCheck{Name: "storage_dir", OK: true},
		DoctorCheck{Name: "compose_config", OK: true},
		DoctorCheck{Name: "services", OK: false},
	)
	if err := rt.requireCalls("compose_config", "services"); err != nil {
		t.Fatal(err)
	}
}

func doctorProjectDir(t *testing.T, envLines []string, createStorage bool) string {
	t.Helper()

	projectDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(projectDir, "compose.yaml"), []byte("services: {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, ".env.prod"), []byte(strings.Join(append(envLines, ""), "\n")), 0o644); err != nil {
		t.Fatal(err)
	}
	if backupRoot := doctorEnvValue(envLines, "BACKUP_ROOT"); backupRoot != "" {
		if err := os.MkdirAll(filepath.Join(projectDir, filepath.Clean(backupRoot)), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if createStorage {
		if err := os.MkdirAll(filepath.Join(projectDir, "runtime", "prod", "espo"), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	return projectDir
}

func doctorEnvValue(envLines []string, key string) string {
	for _, line := range envLines {
		name, value, ok := strings.Cut(line, "=")
		if ok && name == key {
			return value
		}
	}
	return ""
}

func validDoctorEnv() []string {
	return []string{
		"ESPO_CONTOUR=prod",
		"BACKUP_ROOT=./backups/prod",
		"ESPO_STORAGE_DIR=./runtime/prod/espo",
		"APP_SERVICES=espocrm,espocrm-daemon,espocrm-websocket",
		"DB_SERVICE=db",
		"DB_USER=espocrm",
		"DB_PASSWORD=db-secret",
		"DB_NAME=espocrm",
	}
}

type fakeDoctorRuntime struct {
	composeErr  error
	services    []runtime.Service
	servicesErr error
	dbPingErr   error
	calls       []string
}

func (f *fakeDoctorRuntime) ComposeConfig(_ context.Context, _ runtime.Target) error {
	f.calls = append(f.calls, "compose_config")
	return f.composeErr
}

func (f *fakeDoctorRuntime) Services(_ context.Context, _ runtime.Target) ([]runtime.Service, error) {
	f.calls = append(f.calls, "services")
	if f.servicesErr != nil {
		return nil, f.servicesErr
	}
	return append([]runtime.Service(nil), f.services...), nil
}

func (f *fakeDoctorRuntime) DBPing(_ context.Context, _ runtime.Target) error {
	f.calls = append(f.calls, "db_ping")
	return f.dbPingErr
}

func (f *fakeDoctorRuntime) requireCalls(want ...string) error {
	if strings.Join(f.calls, ",") != strings.Join(want, ",") {
		return errf("unexpected call order: got %v want %v", f.calls, want)
	}
	return nil
}

func assertDoctorChecks(t *testing.T, got []DoctorCheck, want ...DoctorCheck) {
	t.Helper()

	if len(got) != len(want) {
		t.Fatalf("unexpected checks length: got %#v want %#v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("unexpected check[%d]: got %#v want %#v", i, got[i], want[i])
		}
	}
}
