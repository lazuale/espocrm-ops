package docker

import (
	"bytes"
	"compress/gzip"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestRestoreMySQLDumpGzUsesArgvWithoutShell(t *testing.T) {
	tmp := t.TempDir()
	dbPath := filepath.Join(tmp, "db.sql.gz")
	logPath := filepath.Join(tmp, "docker.log")
	stdinPath := filepath.Join(tmp, "stdin.sql")

	writeTestGzipFile(t, dbPath, []byte("select 1;"))
	prependFakeDocker(t, fakeDockerOptions{
		logPath:          logPath,
		stdinPath:        stdinPath,
		mariaDBAvailable: true,
		mysqlAvailable:   true,
		inspectRunning:   "true",
	})

	if err := RestoreMySQLDumpGz(dbPath, "db-container", "espocrm", "secret", "espocrm"); err != nil {
		t.Fatalf("RestoreMySQLDumpGz failed: %v", err)
	}

	rawLog, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	log := string(rawLog)
	if strings.Contains(log, "sh") || strings.Contains(log, "-lc") {
		t.Fatalf("restore should not go through shell: %s", log)
	}
	if strings.Contains(log, "secret") {
		t.Fatalf("restore should not put password into docker argv: %s", log)
	}
	if !strings.Contains(log, "exec -i -e MYSQL_PWD db-container mariadb -u espocrm espocrm") {
		t.Fatalf("unexpected docker argv: %s", log)
	}

	rawStdin, err := os.ReadFile(stdinPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(rawStdin) != "select 1;" {
		t.Fatalf("unexpected streamed SQL: %q", string(rawStdin))
	}
}

func TestDumpMySQLDumpGzUsesArgvWithoutShell(t *testing.T) {
	tmp := t.TempDir()
	logPath := filepath.Join(tmp, "docker.log")
	dbPath := filepath.Join(tmp, "db.sql.gz")

	prependFakeDocker(t, fakeDockerOptions{
		logPath:          logPath,
		mariaDBAvailable: true,
		mysqlAvailable:   true,
		dumpStdout:       "create table test(id int);\n",
		inspectRunning:   "true",
	})

	cfg := ComposeConfig{
		ProjectDir:  tmp,
		ComposeFile: filepath.Join(tmp, "compose.yaml"),
		EnvFile:     filepath.Join(tmp, ".env"),
	}

	if err := DumpMySQLDumpGz(cfg, "db", "espocrm", "secret", "espocrm", dbPath); err != nil {
		t.Fatalf("DumpMySQLDumpGz failed: %v", err)
	}

	rawLog, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	log := string(rawLog)
	if strings.Contains(log, "sh") || strings.Contains(log, "-lc") {
		t.Fatalf("dump should not go through shell: %s", log)
	}
	if strings.Contains(log, "secret") {
		t.Fatalf("dump should not put password into docker argv: %s", log)
	}
	if !strings.Contains(log, "exec -i -e MYSQL_PWD mock-db mariadb-dump -u espocrm espocrm --single-transaction --quick --routines --triggers --events") {
		t.Fatalf("unexpected docker argv: %s", log)
	}

	rawDump, err := os.ReadFile(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	reader, err := gzip.NewReader(bytes.NewReader(rawDump))
	if err != nil {
		t.Fatal(err)
	}
	body, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "create table test(id int);\n" {
		t.Fatalf("unexpected dump content: %q", string(body))
	}
}

func TestResetAndRestoreMySQLDumpGzUsesRootWithoutPasswordArgv(t *testing.T) {
	tmp := t.TempDir()
	dbPath := filepath.Join(tmp, "db.sql.gz")
	logPath := filepath.Join(tmp, "docker.log")
	stdinPath := filepath.Join(tmp, "stdin.sql")

	writeTestGzipFile(t, dbPath, []byte("select 1;"))
	prependFakeDocker(t, fakeDockerOptions{
		logPath:          logPath,
		stdinPath:        stdinPath,
		mariaDBAvailable: true,
		mysqlAvailable:   true,
		inspectRunning:   "true",
	})

	if err := ResetAndRestoreMySQLDumpGz(dbPath, "db-container", "root-secret", "espocrm", "espocrm"); err != nil {
		t.Fatalf("ResetAndRestoreMySQLDumpGz failed: %v", err)
	}

	rawLog, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	log := string(rawLog)
	if strings.Contains(log, "sh") || strings.Contains(log, "-lc") {
		t.Fatalf("restore should not go through shell: %s", log)
	}
	if strings.Contains(log, "root-secret") {
		t.Fatalf("restore should not put root password into docker argv: %s", log)
	}
	if !strings.Contains(log, "exec -i -e MYSQL_PWD db-container mariadb -u root\n") {
		t.Fatalf("missing reset argv: %s", log)
	}
	if !strings.Contains(log, "exec -i -e MYSQL_PWD db-container mariadb -u root espocrm\n") {
		t.Fatalf("missing restore argv: %s", log)
	}

	rawStdin, err := os.ReadFile(stdinPath)
	if err != nil {
		t.Fatal(err)
	}
	stdin := string(rawStdin)
	if !strings.Contains(stdin, "DROP DATABASE IF EXISTS `espocrm`;") {
		t.Fatalf("missing reset SQL: %q", stdin)
	}
	if !strings.Contains(stdin, "select 1;") {
		t.Fatalf("missing streamed dump SQL: %q", stdin)
	}
}

func TestDetectDBClientFallsBackToMySQL(t *testing.T) {
	prependFakeDocker(t, fakeDockerOptions{
		mariaDBAvailable: false,
		mysqlAvailable:   true,
		inspectRunning:   "true",
	})

	client, err := DetectDBClient("db-container")
	if err != nil {
		t.Fatal(err)
	}
	if client != "mysql" {
		t.Fatalf("expected mysql fallback, got %s", client)
	}
}

func TestCheckContainerRunningRejectsStoppedContainer(t *testing.T) {
	prependFakeDocker(t, fakeDockerOptions{
		mariaDBAvailable: true,
		mysqlAvailable:   true,
		inspectRunning:   "false",
	})

	err := CheckContainerRunning("db-container")
	if err == nil {
		t.Fatal("expected stopped container error")
	}
	var notRunning ContainerNotRunningError
	if !errors.As(err, &notRunning) {
		t.Fatalf("expected typed not-running error, got %T: %v", err, err)
	}
	if notRunning.ErrorCode() != "container_not_running" {
		t.Fatalf("unexpected error code: %s", notRunning.ErrorCode())
	}
}

func TestCheckDockerAvailableMissingBinaryReturnsTypedError(t *testing.T) {
	t.Setenv("PATH", t.TempDir())

	err := CheckDockerAvailable()
	if err == nil {
		t.Fatal("expected docker unavailable error")
	}

	var unavailable UnavailableError
	if !errors.As(err, &unavailable) {
		t.Fatalf("expected typed unavailable error, got %T: %v", err, err)
	}
	if unavailable.ErrorCode() != "docker_unavailable" {
		t.Fatalf("unexpected error code: %s", unavailable.ErrorCode())
	}
}

func TestRestoreMySQLDumpGzFiltersHostEnv(t *testing.T) {
	tmp := t.TempDir()
	dbPath := filepath.Join(tmp, "db.sql.gz")
	envLogPath := filepath.Join(tmp, "docker.env")

	writeTestGzipFile(t, dbPath, []byte("select 1;"))
	t.Setenv("UNRELATED_SECRET", "host-only-secret")
	prependFakeDocker(t, fakeDockerOptions{
		envLogPath:       envLogPath,
		mariaDBAvailable: true,
		mysqlAvailable:   true,
		inspectRunning:   "true",
	})

	if err := RestoreMySQLDumpGz(dbPath, "db-container", "espocrm", "secret", "espocrm"); err != nil {
		t.Fatalf("RestoreMySQLDumpGz failed: %v", err)
	}

	rawEnv, err := os.ReadFile(envLogPath)
	if err != nil {
		t.Fatal(err)
	}
	envDump := string(rawEnv)
	if !strings.Contains(envDump, "MYSQL_PWD=secret") {
		t.Fatalf("expected MYSQL_PWD in docker env, got: %s", envDump)
	}
	if strings.Contains(envDump, "UNRELATED_SECRET=host-only-secret") {
		t.Fatalf("unexpected host env leak into docker command: %s", envDump)
	}
}

func TestRestoreMySQLDumpGzCapturesDockerExecStderr(t *testing.T) {
	tmp := t.TempDir()
	dbPath := filepath.Join(tmp, "db.sql.gz")

	writeTestGzipFile(t, dbPath, []byte("select 1;"))
	prependFakeDocker(t, fakeDockerOptions{
		mariaDBAvailable: true,
		mysqlAvailable:   true,
		inspectRunning:   "true",
		execStderr:       "permission denied",
		execExitCode:     23,
	})

	err := RestoreMySQLDumpGz(dbPath, "db-container", "espocrm", "secret", "espocrm")
	if err == nil {
		t.Fatal("expected restore error")
	}
	var execErr DBExecutionError
	if !errors.As(err, &execErr) {
		t.Fatalf("expected DBExecutionError, got %T: %v", err, err)
	}
	if execErr.ErrorCode() != "restore_db_failed" {
		t.Fatalf("unexpected error code: %s", execErr.ErrorCode())
	}
	if !strings.Contains(err.Error(), "permission denied") {
		t.Fatalf("expected stderr in restore error, got: %v", err)
	}
}

func TestDetectDBClientReturnsTypedErrorWhenNoClientDetected(t *testing.T) {
	prependFakeDocker(t, fakeDockerOptions{
		mariaDBAvailable: false,
		mysqlAvailable:   false,
		inspectRunning:   "true",
	})

	_, err := DetectDBClient("db-container")
	if err == nil {
		t.Fatal("expected db client detection error")
	}

	var typedErr DBClientDetectionError
	if !errors.As(err, &typedErr) {
		t.Fatalf("expected DBClientDetectionError, got %T: %v", err, err)
	}
	if typedErr.ErrorCode() != "restore_db_failed" {
		t.Fatalf("unexpected error code: %s", typedErr.ErrorCode())
	}
}

type fakeDockerOptions struct {
	logPath          string
	stdinPath        string
	envLogPath       string
	mariaDBAvailable bool
	mysqlAvailable   bool
	dumpStdout       string
	inspectRunning   string
	execStderr       string
	execExitCode     int
}

func prependFakeDocker(t *testing.T, opts fakeDockerOptions) {
	t.Helper()

	binDir := t.TempDir()
	dockerPath := filepath.Join(binDir, "docker")
	script := `#!/usr/bin/env bash
set -euo pipefail

args=" $* "

if [[ "${1:-}" == "version" ]]; then
  echo "11.0.0"
  exit 0
fi

if [[ "${1:-}" == "compose" && "$args" == *" ps "* && "$args" == *" -q "* ]]; then
  service="${*: -1}"
  echo "mock-${service}"
  exit 0
fi

if [[ "${1:-}" == "inspect" ]]; then
  echo "${DOCKER_INSPECT_RUNNING:-true}"
  exit 0
fi

if [[ "${1:-}" == "exec" && "${3:-}" == "mariadb" && "${4:-}" == "--version" ]]; then
  if [[ "${DOCKER_MARIADB_AVAILABLE:-true}" == "true" ]]; then
    echo "mariadb from 11.0.0"
    exit 0
  fi
  echo "mariadb missing" >&2
  exit 2
fi

if [[ "${1:-}" == "exec" && "${3:-}" == "mysql" && "${4:-}" == "--version" ]]; then
  if [[ "${DOCKER_MYSQL_AVAILABLE:-true}" != "true" ]]; then
    echo "mysql missing" >&2
    exit 2
  fi
  echo "mysql from 8.0.0"
  exit 0
fi

if [[ "${1:-}" == "exec" && "${3:-}" == "mariadb-dump" && "${4:-}" == "--version" ]]; then
  if [[ "${DOCKER_MARIADB_AVAILABLE:-true}" == "true" ]]; then
    echo "mariadb-dump from 11.0.0"
    exit 0
  fi
  echo "mariadb-dump missing" >&2
  exit 2
fi

if [[ "${1:-}" == "exec" && "${3:-}" == "mysqldump" && "${4:-}" == "--version" ]]; then
  if [[ "${DOCKER_MYSQL_AVAILABLE:-true}" != "true" ]]; then
    echo "mysqldump missing" >&2
    exit 2
  fi
  echo "mysqldump from 8.0.0"
  exit 0
fi

if [[ "${1:-}" == "exec" && "${2:-}" == "-i" ]]; then
  if [[ -n "${DOCKER_LOG:-}" ]]; then
    printf '%s\n' "$*" >> "$DOCKER_LOG"
  fi
  if [[ " $* " == *" mariadb-dump "* || " $* " == *" mysqldump "* ]]; then
    if [[ -n "${DOCKER_DUMP_STDOUT:-}" ]]; then
      printf '%s' "${DOCKER_DUMP_STDOUT}"
    fi
    exit 0
  fi
  if [[ -n "${DOCKER_ENV_LOG:-}" ]]; then
    env | sort > "$DOCKER_ENV_LOG"
  fi
  if [[ -n "${DOCKER_STDIN_LOG:-}" ]]; then
    cat >> "$DOCKER_STDIN_LOG"
  else
    cat >/dev/null
  fi
  if [[ -n "${DOCKER_EXEC_STDERR:-}" ]]; then
    printf '%s\n' "$DOCKER_EXEC_STDERR" >&2
  fi
  if [[ -n "${DOCKER_EXEC_EXIT_CODE:-}" ]]; then
    exit "$DOCKER_EXEC_EXIT_CODE"
  fi
  exit 0
fi

echo "unexpected docker invocation: $*" >&2
exit 98
`
	if err := os.WriteFile(dockerPath, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	if opts.logPath != "" {
		t.Setenv("DOCKER_LOG", opts.logPath)
	}
	if opts.stdinPath != "" {
		t.Setenv("DOCKER_STDIN_LOG", opts.stdinPath)
	}
	if opts.envLogPath != "" {
		t.Setenv("DOCKER_ENV_LOG", opts.envLogPath)
	}
	if opts.inspectRunning == "" {
		opts.inspectRunning = "true"
	}
	t.Setenv("DOCKER_INSPECT_RUNNING", opts.inspectRunning)
	if opts.mariaDBAvailable {
		t.Setenv("DOCKER_MARIADB_AVAILABLE", "true")
	} else {
		t.Setenv("DOCKER_MARIADB_AVAILABLE", "false")
	}
	if opts.mysqlAvailable {
		t.Setenv("DOCKER_MYSQL_AVAILABLE", "true")
	} else {
		t.Setenv("DOCKER_MYSQL_AVAILABLE", "false")
	}
	if opts.execStderr != "" {
		t.Setenv("DOCKER_EXEC_STDERR", opts.execStderr)
	}
	if opts.dumpStdout != "" {
		t.Setenv("DOCKER_DUMP_STDOUT", opts.dumpStdout)
	}
	if opts.execExitCode != 0 {
		t.Setenv("DOCKER_EXEC_EXIT_CODE", strconv.Itoa(opts.execExitCode))
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func writeTestGzipFile(t *testing.T, path string, body []byte) {
	t.Helper()

	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	gz := gzip.NewWriter(f)
	if _, err := gz.Write(body); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
}
