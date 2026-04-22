package doctor

import (
	"maps"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	platformlocks "github.com/lazuale/espocrm-ops/internal/platform/locks"
	"github.com/lazuale/espocrm-ops/internal/testutil"
)

func TestDiagnoseReadyForSingleContour(t *testing.T) {
	projectDir := newDoctorProject(t)
	writeDoctorEnv(t, projectDir, "prod", nil)
	prependFakeDocker(t)

	report, err := testDoctorService().Diagnose(Request{
		Scope:       "prod",
		ProjectDir:  projectDir,
		ComposeFile: filepath.Join(projectDir, "compose.yaml"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if !report.Ready() {
		t.Fatalf("expected ready report, got %#v", report.Checks)
	}
	if check, ok := findCheck(report, "env_contract", "prod"); !ok || check.Status != statusOK {
		t.Fatalf("expected env_contract ok, got %#v", check)
	}
}

func TestDiagnoseReportsSharedLockConflict(t *testing.T) {
	projectDir := newDoctorProject(t)
	writeDoctorEnv(t, projectDir, "prod", nil)
	prependFakeDocker(t)

	lock, err := platformlocks.AcquireSharedOperationLock(projectDir, "backup", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = lock.Release()
	}()

	report, err := testDoctorService().Diagnose(Request{
		Scope:       "prod",
		ProjectDir:  projectDir,
		ComposeFile: filepath.Join(projectDir, "compose.yaml"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if report.Ready() {
		t.Fatalf("expected lock conflict report, got %#v", report.Checks)
	}
	check, ok := findCheck(report, "shared_operation_lock", "")
	if !ok {
		t.Fatalf("missing shared_operation_lock check in %#v", report.Checks)
	}
	if check.Status != statusFail {
		t.Fatalf("expected failing shared lock check, got %#v", check)
	}
}

func TestDiagnoseReportsCrossScopeCompatibilityDrift(t *testing.T) {
	projectDir := newDoctorProject(t)
	writeDoctorEnv(t, projectDir, "prod", nil)
	writeDoctorEnv(t, projectDir, "dev", map[string]string{
		"APP_PORT":      "18082",
		"WS_PORT":       "18083",
		"ESPOCRM_IMAGE": "espocrm/espocrm:9.9.9-apache",
	})
	prependFakeDocker(t)

	report, err := testDoctorService().Diagnose(Request{
		Scope:       "all",
		ProjectDir:  projectDir,
		ComposeFile: filepath.Join(projectDir, "compose.yaml"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if report.Ready() {
		t.Fatalf("expected cross-scope drift failure, got %#v", report.Checks)
	}
	check, ok := findCheck(report, "cross_scope_compatibility", "cross")
	if !ok {
		t.Fatalf("missing cross_scope_compatibility check in %#v", report.Checks)
	}
	if check.Status != statusFail {
		t.Fatalf("expected failing cross-scope compatibility check, got %#v", check)
	}
}

func TestDiagnoseReportsInvalidRuntimeContract(t *testing.T) {
	projectDir := newDoctorProject(t)
	writeDoctorEnv(t, projectDir, "prod", map[string]string{
		"ESPO_RUNTIME_UID": "oops",
	})
	prependFakeDocker(t)

	report, err := testDoctorService().Diagnose(Request{
		Scope:       "prod",
		ProjectDir:  projectDir,
		ComposeFile: filepath.Join(projectDir, "compose.yaml"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if report.Ready() {
		t.Fatalf("expected env contract failure, got %#v", report.Checks)
	}
	check, ok := findCheck(report, "env_contract", "prod")
	if !ok {
		t.Fatalf("missing env_contract check in %#v", report.Checks)
	}
	if check.Status != statusFail {
		t.Fatalf("expected failing env_contract check, got %#v", check)
	}
	if !strings.Contains(check.Details, "ESPO_RUNTIME_UID must be an integer") {
		t.Fatalf("expected runtime contract validation detail, got %#v", check)
	}
}

func newDoctorProject(t *testing.T) string {
	t.Helper()

	projectDir := filepath.Join(t.TempDir(), "project")
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, "compose.yaml"), []byte("services: {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	return projectDir
}

func writeDoctorEnv(t *testing.T, projectDir, scope string, overrides map[string]string) {
	t.Helper()

	values := testutil.BaseEnvValues(scope)
	maps.Copy(values, overrides)

	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	lines := make([]string, 0, len(keys))
	for _, key := range keys {
		lines = append(lines, key+"="+values[key])
	}

	path := filepath.Join(projectDir, ".env."+scope)
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
}

func prependFakeDocker(t *testing.T) {
	t.Helper()

	binDir := filepath.Join(t.TempDir(), "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}

	script := `#!/usr/bin/env bash
set -Eeuo pipefail

if [[ "${1:-}" == "version" && "${2:-}" == "--format" && "${3:-}" == "{{.Client.Version}}" ]]; then
  echo "25.0.2"
  exit 0
fi

if [[ "${1:-}" == "version" && "${2:-}" == "--format" && "${3:-}" == "{{.Server.Version}}" ]]; then
  echo "25.0.2"
  exit 0
fi

if [[ "${1:-}" == "compose" && "${2:-}" == "version" && "${3:-}" == "--short" ]]; then
  echo "2.24.1"
  exit 0
fi

if [[ "${1:-}" == "compose" ]]; then
  shift
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --project-directory|-f|--env-file)
        shift 2
        ;;
      config)
        exit 0
        ;;
      -q)
        shift
        ;;
      ps)
        shift
        while [[ $# -gt 0 ]]; do
          case "$1" in
            --status)
              shift 2
              ;;
            --services|-q)
              exit 0
              ;;
            *)
              shift
              ;;
          esac
        done
        exit 0
        ;;
      *)
        shift
        ;;
    esac
  done
fi

echo "unexpected docker args: $*" >&2
exit 97
`

	path := filepath.Join(binDir, "docker")
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func findCheck(report Report, code, scope string) (Check, bool) {
	for _, check := range report.Checks {
		if check.Code == code && check.Scope == scope {
			return check, true
		}
	}

	return Check{}, false
}
