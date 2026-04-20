package migrate

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/lazuale/espocrm-ops/internal/contract/apperr"
	domainbackup "github.com/lazuale/espocrm-ops/internal/domain/backup"
	"github.com/lazuale/espocrm-ops/internal/platform/locks"
)

func TestExecute_SkipDBNoStart_ReconcilesFilesPermissions(t *testing.T) {
	fixture := newExecuteFixture(t)

	info, err := Execute(ExecuteRequest{
		SourceScope: "dev",
		TargetScope: "prod",
		ProjectDir:  fixture.projectDir,
		ComposeFile: fixture.composeFile,
		SkipDB:      true,
		NoStart:     true,
	})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if !info.Ready() {
		t.Fatal("expected migrate info to be ready")
	}

	completed, skipped, failed, notRun := info.Counts()
	if completed != 6 || skipped != 2 || failed != 0 || notRun != 0 {
		t.Fatalf("unexpected step counts: completed=%d skipped=%d failed=%d not_run=%d", completed, skipped, failed, notRun)
	}

	filesStep := requireExecuteStep(t, info, "files_restore")
	if filesStep.Status != MigrateStepStatusCompleted {
		t.Fatalf("expected files_restore completed, got %s", filesStep.Status)
	}

	if _, err := os.Stat(filepath.Join(fixture.storageDir, ".permissions-reconciled")); err != nil {
		t.Fatalf("expected permission reconcile marker: %v", err)
	}
	mustContainFile(t, filepath.Join(fixture.storageDir, "restored.txt"), "hello")
	if _, err := os.Stat(filepath.Join(fixture.storageDir, "stale.txt")); !os.IsNotExist(err) {
		t.Fatalf("expected stale target file removed, got %v", err)
	}

	log := readFile(t, fixture.logPath)
	if !containsAll(log,
		"image inspect espocrm/espocrm:9.3.4-apache",
		"-v "+fixture.storageDir+":/espo-storage",
	) {
		t.Fatalf("expected permission reconcile docker calls in log:\n%s", log)
	}
}

func TestExecute_FailsClosedWhenFilesPermissionReconcileFails(t *testing.T) {
	fixture := newExecuteFixture(t)
	t.Setenv("DOCKER_MOCK_RESTORE_RECONCILE_ERROR", "permission reconcile failed")

	info, err := Execute(ExecuteRequest{
		SourceScope: "dev",
		TargetScope: "prod",
		ProjectDir:  fixture.projectDir,
		ComposeFile: fixture.composeFile,
		SkipDB:      true,
		NoStart:     true,
	})
	if err == nil {
		t.Fatal("expected reconcile failure")
	}
	if kind, ok := apperr.KindOf(err); !ok || kind != apperr.KindExternal {
		t.Fatalf("expected external error kind, got %v", err)
	}
	if code, ok := apperr.CodeOf(err); !ok || code != "migrate_failed" {
		t.Fatalf("expected migrate_failed code, got %q ok=%v", code, ok)
	}
	if info.Ready() {
		t.Fatal("expected migrate info to report failure")
	}

	completed, skipped, failed, notRun := info.Counts()
	if completed != 5 || skipped != 1 || failed != 1 || notRun != 1 {
		t.Fatalf("unexpected step counts: completed=%d skipped=%d failed=%d not_run=%d", completed, skipped, failed, notRun)
	}

	filesStep := requireExecuteStep(t, info, "files_restore")
	if filesStep.Status != MigrateStepStatusFailed {
		t.Fatalf("expected files_restore failed, got %s", filesStep.Status)
	}
	if !strings.Contains(filesStep.Details, "permission reconcile failed") {
		t.Fatalf("expected reconcile failure details, got %q", filesStep.Details)
	}

	targetStartStep := requireExecuteStep(t, info, "target_start")
	if targetStartStep.Status != MigrateStepStatusNotRun {
		t.Fatalf("expected target_start not_run, got %s", targetStartStep.Status)
	}

	if _, err := os.Stat(filepath.Join(fixture.storageDir, ".permissions-reconciled")); !os.IsNotExist(err) {
		t.Fatalf("did not expect permission reconcile marker after failure, got %v", err)
	}
}

type executeFixture struct {
	projectDir  string
	composeFile string
	storageDir  string
	logPath     string
}

func newExecuteFixture(t *testing.T) executeFixture {
	t.Helper()

	restoreLocks := locks.SetLockDirForTest(t.TempDir())
	t.Cleanup(restoreLocks)

	tmp := t.TempDir()
	projectDir := filepath.Join(tmp, "project")
	stateDir := filepath.Join(tmp, "docker-state")
	logPath := filepath.Join(tmp, "docker.log")
	composeFile := filepath.Join(projectDir, "compose.yaml")
	storageDir := filepath.Join(projectDir, "runtime", "prod", "espo")

	for _, dir := range []string{projectDir, stateDir, storageDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(composeFile, []byte("services: {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stateDir, "running-services"), []byte("db\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stateDir, "db.statuses"), []byte("healthy\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(storageDir, "stale.txt"), []byte("stale\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	writeExecuteEnvFile(t, projectDir, "dev", nil)
	writeExecuteEnvFile(t, projectDir, "prod", nil)
	writeExecuteBackupSet(t, filepath.Join(projectDir, "backups", "dev"), "espocrm-dev", "2026-04-19_08-00-00", "dev")
	prependFakeDockerForExecuteTest(t)

	t.Setenv("DOCKER_MOCK_RECOVERY_STATE_DIR", stateDir)
	t.Setenv("DOCKER_MOCK_RECOVERY_LOG", logPath)
	t.Setenv("DOCKER_MOCK_RESTORE_RUNTIME_UID", strconv.Itoa(os.Getuid()))
	t.Setenv("DOCKER_MOCK_RESTORE_RUNTIME_GID", strconv.Itoa(os.Getgid()))

	return executeFixture{
		projectDir:  projectDir,
		composeFile: composeFile,
		storageDir:  storageDir,
		logPath:     logPath,
	}
}

func requireExecuteStep(t *testing.T, info ExecuteInfo, code string) ExecuteStep {
	t.Helper()

	for _, step := range info.Steps {
		if step.Code == code {
			return step
		}
	}

	t.Fatalf("missing step %q", code)
	return ExecuteStep{}
}

func writeExecuteEnvFile(t *testing.T, projectDir, scope string, overrides map[string]string) {
	t.Helper()

	values := map[string]string{
		"ADMIN_PASSWORD":             "admin-secret",
		"ADMIN_USERNAME":             "admin",
		"APP_PORT":                   "18080",
		"BACKUP_MAX_DB_AGE_HOURS":    "48",
		"BACKUP_MAX_FILES_AGE_HOURS": "48",
		"BACKUP_NAME_PREFIX":         "espocrm-" + scope,
		"BACKUP_RETENTION_DAYS":      "7",
		"BACKUP_ROOT":                "./backups/" + scope,
		"COMPOSE_PROJECT_NAME":       "espocrm-" + scope,
		"DAEMON_CPUS":                "0.50",
		"DAEMON_MEM_LIMIT":           "256m",
		"DAEMON_PIDS_LIMIT":          "128",
		"DB_CPUS":                    "1.00",
		"DB_MEM_LIMIT":               "512m",
		"DB_NAME":                    "espocrm",
		"DB_PASSWORD":                "db-secret",
		"DB_PIDS_LIMIT":              "256",
		"DB_ROOT_PASSWORD":           "root-secret",
		"DB_STORAGE_DIR":             "./runtime/" + scope + "/db",
		"DB_USER":                    "espocrm",
		"DOCKER_LOG_MAX_FILE":        "5",
		"DOCKER_LOG_MAX_SIZE":        "10m",
		"ESPO_CONTOUR":               scope,
		"ESPO_CPUS":                  "1.00",
		"ESPO_DEFAULT_LANGUAGE":      "ru_RU",
		"ESPO_LOGGER_LEVEL":          "INFO",
		"ESPO_MEM_LIMIT":             "512m",
		"ESPO_PIDS_LIMIT":            "256",
		"ESPO_STORAGE_DIR":           "./runtime/" + scope + "/espo",
		"ESPO_TIME_ZONE":             "Europe/Moscow",
		"ESPOCRM_IMAGE":              "espocrm/espocrm:9.3.4-apache",
		"MARIADB_TAG":                "10.11",
		"MIN_FREE_DISK_MB":           "1",
		"REPORT_RETENTION_DAYS":      "30",
		"SITE_URL":                   "http://127.0.0.1:18080",
		"SUPPORT_RETENTION_DAYS":     "14",
		"WS_CPUS":                    "0.50",
		"WS_MEM_LIMIT":               "256m",
		"WS_PIDS_LIMIT":              "128",
		"WS_PORT":                    "18081",
		"WS_PUBLIC_URL":              "ws://127.0.0.1:18081",
	}
	for key, value := range overrides {
		values[key] = value
	}

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

func writeExecuteBackupSet(t *testing.T, backupRoot, prefix, stamp, scope string) {
	t.Helper()

	dbName := prefix + "_" + stamp + ".sql.gz"
	filesName := prefix + "_files_" + stamp + ".tar.gz"
	dbPath := filepath.Join(backupRoot, "db", dbName)
	filesPath := filepath.Join(backupRoot, "files", filesName)
	manifestPath := filepath.Join(backupRoot, "manifests", prefix+"_"+stamp+".manifest.json")

	for _, dir := range []string{filepath.Dir(dbPath), filepath.Dir(filesPath), filepath.Dir(manifestPath)} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}

	writeExecuteGzipFile(t, dbPath, []byte("select 1;"))
	writeExecuteTarGz(t, filesPath, map[string]string{
		"espo/restored.txt": "hello",
	})
	writeExecuteSHA256Sidecar(t, dbPath)
	writeExecuteSHA256Sidecar(t, filesPath)
	writeExecuteManifest(t, manifestPath, domainbackup.Manifest{
		Version:   1,
		Scope:     scope,
		CreatedAt: time.Date(2026, 4, 19, 8, 0, 0, 0, time.UTC).Format(time.RFC3339),
		Artifacts: domainbackup.ManifestArtifacts{
			DBBackup:    dbName,
			FilesBackup: filesName,
		},
		Checksums: domainbackup.ManifestChecksums{
			DBBackup:    sha256OfPath(t, dbPath),
			FilesBackup: sha256OfPath(t, filesPath),
		},
	})
}

func prependFakeDockerForExecuteTest(t *testing.T) {
	t.Helper()

	binDir := filepath.Join(t.TempDir(), "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}

	script := `#!/usr/bin/env bash
set -Eeuo pipefail

state_dir="${DOCKER_MOCK_RECOVERY_STATE_DIR:-}"

log_call() {
  if [[ -n "${DOCKER_MOCK_RECOVERY_LOG:-}" ]]; then
    printf '%s\n' "$*" >> "${DOCKER_MOCK_RECOVERY_LOG}"
  fi
}

read_running_services() {
  if [[ -f "$state_dir/running-services" ]]; then
    cat "$state_dir/running-services"
  fi
}

write_running_services() {
  mkdir -p "$state_dir"
  : > "$state_dir/running-services"
  for service in "$@"; do
    [[ -n "$service" ]] || continue
    printf '%s\n' "$service" >> "$state_dir/running-services"
  done
}

service_is_running() {
  local service="$1"
  grep -qx "$service" "$state_dir/running-services" 2>/dev/null
}

set_running_services() {
  local mode="$1"
  shift

  local canonical=("db" "espocrm" "espocrm-daemon" "espocrm-websocket")
  local requested=("$@")
  local current=()
  local result=()
  local service

  if [[ -f "$state_dir/running-services" ]]; then
    mapfile -t current < "$state_dir/running-services"
  fi

  case "$mode" in
    add)
      for service in "${canonical[@]}"; do
        if printf '%s\n' "${current[@]}" "${requested[@]}" | grep -qx "$service"; then
          result+=("$service")
        fi
      done
      ;;
    remove)
      for service in "${canonical[@]}"; do
        if printf '%s\n' "${requested[@]}" | grep -qx "$service"; then
          continue
        fi
        if printf '%s\n' "${current[@]}" | grep -qx "$service"; then
          result+=("$service")
        fi
      done
      ;;
  esac

  write_running_services "${result[@]}"
}

log_call "$*"

if [[ "${1:-}" == "compose" ]]; then
  shift
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --project-directory|-f|--env-file)
        shift 2
        continue
        ;;
      ps)
        shift
        if [[ "${1:-}" == "--status" && "${2:-}" == "running" && "${3:-}" == "--services" ]]; then
          read_running_services
          exit 0
        fi
        if [[ "${1:-}" == "-q" ]]; then
          service="${2:-}"
          if service_is_running "$service"; then
            echo "mock-${service}"
          fi
          exit 0
        fi
        ;;
      up)
        shift
        if [[ "${1:-}" == "-d" ]]; then
          shift
        fi
        set_running_services add "$@"
        exit 0
        ;;
      stop)
        shift
        set_running_services remove "$@"
        exit 0
        ;;
    esac
    shift
  done
fi

if [[ "${1:-}" == "image" && "${2:-}" == "inspect" ]]; then
  exit 0
fi

if [[ "${1:-}" == "inspect" ]]; then
  if [[ "$*" == *".State.Health.Log"* ]]; then
    printf '%s\n' "${DOCKER_MOCK_RECOVERY_HEALTH_MESSAGE:-mock health failure}"
    exit 0
  fi

  container="${*: -1}"
  service="${container#mock-}"
  status_file="$state_dir/${service}.statuses"
  status=""

  if [[ -f "$status_file" ]]; then
    status="$(head -n 1 "$status_file")"
  elif service_is_running "$service"; then
    status="healthy"
  fi

  printf '%s\n' "$status"
  exit 0
fi

if [[ "${1:-}" == "run" ]]; then
  storage_host=""
  previous=""
  for arg in "$@"; do
    if [[ "$previous" == "-v" && "$arg" == *":/espo-storage" ]]; then
      storage_host="${arg%%:/espo-storage}"
      break
    fi
    if [[ "$arg" == *":/espo-storage" ]]; then
      storage_host="${arg%%:/espo-storage}"
      break
    fi
    previous="$arg"
  done

  if [[ -n "$storage_host" ]]; then
    if [[ -n "${DOCKER_MOCK_RESTORE_RECONCILE_ERROR:-}" ]]; then
      echo "${DOCKER_MOCK_RESTORE_RECONCILE_ERROR}" >&2
      exit 73
    fi
    printf 'ok\n' > "$storage_host/.permissions-reconciled"
    exit 0
  fi

  printf '%s:%s\n' "${DOCKER_MOCK_RESTORE_RUNTIME_UID:-1000}" "${DOCKER_MOCK_RESTORE_RUNTIME_GID:-1000}"
  exit 0
fi

echo "unexpected docker invocation: $*" >&2
exit 98
`

	dockerPath := filepath.Join(binDir, "docker")
	if err := os.WriteFile(dockerPath, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func mustContainFile(t *testing.T, path, want string) {
	t.Helper()

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != want {
		t.Fatalf("unexpected content in %s: got %q want %q", path, string(raw), want)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func containsAll(text string, parts ...string) bool {
	for _, part := range parts {
		if !strings.Contains(text, part) {
			return false
		}
	}
	return true
}

func writeExecuteManifest(t *testing.T, path string, manifest domainbackup.Manifest) {
	t.Helper()

	raw, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatal(err)
	}
}

func writeExecuteGzipFile(t *testing.T, path string, body []byte) {
	t.Helper()

	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer closeExecuteWriter(t, "gzip file", file)

	gz := gzip.NewWriter(file)
	if _, err := gz.Write(body); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
}

func writeExecuteTarGz(t *testing.T, path string, files map[string]string) {
	t.Helper()

	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer closeExecuteWriter(t, "tar archive file", file)

	gz := gzip.NewWriter(file)
	defer closeExecuteWriter(t, "tar archive gzip writer", gz)

	tw := tar.NewWriter(gz)
	defer closeExecuteWriter(t, "tar archive writer", tw)

	for name, body := range files {
		header := &tar.Header{
			Name: name,
			Mode: 0o644,
			Size: int64(len(body)),
		}
		if err := tw.WriteHeader(header); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
}

func writeExecuteSHA256Sidecar(t *testing.T, path string) {
	t.Helper()

	body := sha256OfPath(t, path) + "  " + filepath.Base(path) + "\n"
	if err := os.WriteFile(path+".sha256", []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func sha256OfPath(t *testing.T, path string) string {
	t.Helper()

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func closeExecuteWriter(t *testing.T, label string, closer interface{ Close() error }) {
	t.Helper()

	if err := closer.Close(); err != nil {
		t.Fatalf("close %s: %v", label, err)
	}
}
