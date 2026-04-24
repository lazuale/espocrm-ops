package runtime

import (
	"compress/gzip"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDockerComposeValidateRunsComposeConfig(t *testing.T) {
	projectDir := t.TempDir()
	logPath := installFakeDocker(t)

	rt := DockerCompose{}
	err := rt.Validate(context.Background(), Target{
		ProjectDir:  projectDir,
		ComposeFile: filepath.Join(projectDir, "compose.yaml"),
		EnvFile:     filepath.Join(projectDir, ".env.prod"),
	})
	if err != nil {
		t.Fatalf("Validate failed: %v", err)
	}

	log := mustReadFile(t, logPath)
	if !strings.Contains(log, "compose --env-file "+filepath.Join(projectDir, ".env.prod")+" -f "+filepath.Join(projectDir, "compose.yaml")+" config") {
		t.Fatalf("unexpected docker log:\n%s", log)
	}
}

func TestDockerComposeComposeConfigRunsComposeConfig(t *testing.T) {
	projectDir := t.TempDir()
	logPath := installFakeDocker(t)

	rt := DockerCompose{}
	err := rt.ComposeConfig(context.Background(), Target{
		ProjectDir:  projectDir,
		ComposeFile: filepath.Join(projectDir, "compose.yaml"),
		EnvFile:     filepath.Join(projectDir, ".env.prod"),
	})
	if err != nil {
		t.Fatalf("ComposeConfig failed: %v", err)
	}

	log := mustReadFile(t, logPath)
	if !strings.Contains(log, "compose --env-file "+filepath.Join(projectDir, ".env.prod")+" -f "+filepath.Join(projectDir, "compose.yaml")+" config") {
		t.Fatalf("unexpected docker log:\n%s", log)
	}
}

func TestDockerComposeDumpDatabaseRunsMariadbDump(t *testing.T) {
	projectDir := t.TempDir()
	logPath := installFakeDocker(t)
	destPath := filepath.Join(t.TempDir(), "db.sql.gz")

	rt := DockerCompose{}
	err := rt.DumpDatabase(context.Background(), Target{
		ProjectDir:  projectDir,
		ComposeFile: filepath.Join(projectDir, "compose.yaml"),
		EnvFile:     filepath.Join(projectDir, ".env.prod"),
		DBService:   "db",
		DBUser:      "espocrm",
		DBPassword:  "db-secret",
		DBName:      "espocrm",
	}, destPath)
	if err != nil {
		t.Fatalf("DumpDatabase failed: %v", err)
	}

	log := mustReadFile(t, logPath)
	if !strings.Contains(log, "compose --env-file "+filepath.Join(projectDir, ".env.prod")+" -f "+filepath.Join(projectDir, "compose.yaml")+" exec -T -e MYSQL_PWD db mariadb-dump --single-transaction --quick --routines --triggers --events -u espocrm espocrm") {
		t.Fatalf("unexpected docker log:\n%s", log)
	}
	if strings.Contains(log, "db-secret") {
		t.Fatalf("docker log leaked db password:\n%s", log)
	}

	raw, err := os.Open(destPath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if closeErr := raw.Close(); closeErr != nil {
			t.Fatal(closeErr)
		}
	}()

	gz, err := gzip.NewReader(raw)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if closeErr := gz.Close(); closeErr != nil {
			t.Fatal(closeErr)
		}
	}()

	body, err := io.ReadAll(gz)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "create table test(id int);\n" {
		t.Fatalf("unexpected dump body: %q", string(body))
	}
}

func TestDockerComposeStopServicesRunsComposeStop(t *testing.T) {
	projectDir := t.TempDir()
	logPath := installFakeDocker(t)

	rt := DockerCompose{}
	err := rt.StopServices(context.Background(), Target{
		ProjectDir:  projectDir,
		ComposeFile: filepath.Join(projectDir, "compose.yaml"),
		EnvFile:     filepath.Join(projectDir, ".env.prod"),
	}, []string{"espocrm", "espocrm-daemon"})
	if err != nil {
		t.Fatalf("StopServices failed: %v", err)
	}

	log := mustReadFile(t, logPath)
	if !strings.Contains(log, "compose --env-file "+filepath.Join(projectDir, ".env.prod")+" -f "+filepath.Join(projectDir, "compose.yaml")+" stop espocrm espocrm-daemon") {
		t.Fatalf("unexpected docker log:\n%s", log)
	}
}

func TestDockerComposeStartServicesRunsComposeStart(t *testing.T) {
	projectDir := t.TempDir()
	logPath := installFakeDocker(t)

	rt := DockerCompose{}
	err := rt.StartServices(context.Background(), Target{
		ProjectDir:  projectDir,
		ComposeFile: filepath.Join(projectDir, "compose.yaml"),
		EnvFile:     filepath.Join(projectDir, ".env.prod"),
	}, []string{"espocrm", "espocrm-daemon"})
	if err != nil {
		t.Fatalf("StartServices failed: %v", err)
	}

	log := mustReadFile(t, logPath)
	if !strings.Contains(log, "compose --env-file "+filepath.Join(projectDir, ".env.prod")+" -f "+filepath.Join(projectDir, "compose.yaml")+" start espocrm espocrm-daemon") {
		t.Fatalf("unexpected docker log:\n%s", log)
	}
}

func TestDockerComposeUpServiceRunsComposeUp(t *testing.T) {
	projectDir := t.TempDir()
	logPath := installFakeDocker(t)

	rt := DockerCompose{}
	err := rt.UpService(context.Background(), Target{
		ProjectDir:  projectDir,
		ComposeFile: filepath.Join(projectDir, "compose.yaml"),
		EnvFile:     filepath.Join(projectDir, ".env.prod"),
		DBService:   "db",
	}, "db")
	if err != nil {
		t.Fatalf("UpService failed: %v", err)
	}

	log := mustReadFile(t, logPath)
	if !strings.Contains(log, "compose --env-file "+filepath.Join(projectDir, ".env.prod")+" -f "+filepath.Join(projectDir, "compose.yaml")+" up -d db") {
		t.Fatalf("unexpected docker log:\n%s", log)
	}
}

func TestDockerComposeServicesRunsComposePS(t *testing.T) {
	projectDir := t.TempDir()
	logPath := installFakeDocker(t)
	t.Setenv("TEST_DOCKER_PS_OUTPUT", `[{"Service":"db"},{"Service":"espocrm"}]`)

	rt := DockerCompose{}
	services, err := rt.Services(context.Background(), Target{
		ProjectDir:  projectDir,
		ComposeFile: filepath.Join(projectDir, "compose.yaml"),
		EnvFile:     filepath.Join(projectDir, ".env.prod"),
	})
	if err != nil {
		t.Fatalf("Services failed: %v", err)
	}
	if len(services) != 2 || services[0].Name != "db" || services[1].Name != "espocrm" {
		t.Fatalf("unexpected services: %#v", services)
	}

	log := mustReadFile(t, logPath)
	if !strings.Contains(log, "compose --env-file "+filepath.Join(projectDir, ".env.prod")+" -f "+filepath.Join(projectDir, "compose.yaml")+" ps --format json") {
		t.Fatalf("unexpected docker log:\n%s", log)
	}
}

func TestDockerComposeServicesAcceptsJSONLines(t *testing.T) {
	projectDir := t.TempDir()
	installFakeDocker(t)
	t.Setenv("TEST_DOCKER_PS_OUTPUT", "{\"Service\":\"db\"}\n{\"Service\":\"espocrm\"}\n")

	rt := DockerCompose{}
	services, err := rt.Services(context.Background(), Target{
		ProjectDir:  projectDir,
		ComposeFile: filepath.Join(projectDir, "compose.yaml"),
		EnvFile:     filepath.Join(projectDir, ".env.prod"),
	})
	if err != nil {
		t.Fatalf("Services failed: %v", err)
	}
	if len(services) != 2 || services[0].Name != "db" || services[1].Name != "espocrm" {
		t.Fatalf("unexpected services: %#v", services)
	}
}

func TestDockerComposeDBPingRunsMariadbSelect1(t *testing.T) {
	projectDir := t.TempDir()
	logPath := installFakeDocker(t)

	rt := DockerCompose{}
	err := rt.DBPing(context.Background(), Target{
		ProjectDir:  projectDir,
		ComposeFile: filepath.Join(projectDir, "compose.yaml"),
		EnvFile:     filepath.Join(projectDir, ".env.prod"),
		DBService:   "db",
		DBUser:      "espocrm",
		DBPassword:  "db-secret",
		DBName:      "espocrm",
	})
	if err != nil {
		t.Fatalf("DBPing failed: %v", err)
	}

	log := mustReadFile(t, logPath)
	if !strings.Contains(log, "compose --env-file "+filepath.Join(projectDir, ".env.prod")+" -f "+filepath.Join(projectDir, "compose.yaml")+" exec -T -e MYSQL_PWD db mariadb -u espocrm espocrm -e SELECT 1;") {
		t.Fatalf("unexpected docker log:\n%s", log)
	}
	if strings.Contains(log, "db-secret") {
		t.Fatalf("docker log leaked db password:\n%s", log)
	}
}

func TestDockerComposeResetDatabaseRunsMariadbRootCommand(t *testing.T) {
	projectDir := t.TempDir()
	logPath := installFakeDocker(t)

	rt := DockerCompose{}
	err := rt.ResetDatabase(context.Background(), Target{
		ProjectDir:     projectDir,
		ComposeFile:    filepath.Join(projectDir, "compose.yaml"),
		EnvFile:        filepath.Join(projectDir, ".env.prod"),
		DBService:      "db",
		DBRootPassword: "root-secret",
		DBName:         "espocrm",
	})
	if err != nil {
		t.Fatalf("ResetDatabase failed: %v", err)
	}

	log := mustReadFile(t, logPath)
	if !strings.Contains(log, "compose --env-file "+filepath.Join(projectDir, ".env.prod")+" -f "+filepath.Join(projectDir, "compose.yaml")+" exec -T -e MYSQL_PWD db mariadb -u root -e DROP DATABASE IF EXISTS `espocrm`; CREATE DATABASE `espocrm` CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;") {
		t.Fatalf("unexpected docker log:\n%s", log)
	}
	if strings.Contains(log, "root-secret") {
		t.Fatalf("docker log leaked db root password:\n%s", log)
	}
}

func TestDockerComposeRestoreDatabaseRunsMariadb(t *testing.T) {
	projectDir := t.TempDir()
	logPath := installFakeDocker(t)
	stdinLogPath := filepath.Join(t.TempDir(), "restore-db.sql")
	t.Setenv("TEST_DOCKER_STDIN_LOG", stdinLogPath)

	rt := DockerCompose{}
	err := rt.RestoreDatabase(context.Background(), Target{
		ProjectDir:  projectDir,
		ComposeFile: filepath.Join(projectDir, "compose.yaml"),
		EnvFile:     filepath.Join(projectDir, ".env.prod"),
		DBService:   "db",
		DBUser:      "espocrm",
		DBPassword:  "db-secret",
		DBName:      "espocrm",
	}, strings.NewReader("create table restored(id int);\n"))
	if err != nil {
		t.Fatalf("RestoreDatabase failed: %v", err)
	}

	log := mustReadFile(t, logPath)
	if !strings.Contains(log, "compose --env-file "+filepath.Join(projectDir, ".env.prod")+" -f "+filepath.Join(projectDir, "compose.yaml")+" exec -T -e MYSQL_PWD db mariadb -u espocrm espocrm") {
		t.Fatalf("unexpected docker log:\n%s", log)
	}
	if strings.Contains(log, "db-secret") {
		t.Fatalf("docker log leaked db password:\n%s", log)
	}
	if body := mustReadFile(t, stdinLogPath); body != "create table restored(id int);\n" {
		t.Fatalf("unexpected restore db body: %q", body)
	}
}

func TestDockerComposeDBPingRuntimeErrorRedactsPassword(t *testing.T) {
	projectDir := t.TempDir()
	installFakeDocker(t)
	t.Setenv("TEST_DOCKER_FAIL_EXEC", "1")
	t.Setenv("TEST_DOCKER_FAIL_STDERR", "db ping failed with MYSQL_PWD=db-secret")

	rt := DockerCompose{}
	err := rt.DBPing(context.Background(), Target{
		ProjectDir:  projectDir,
		ComposeFile: filepath.Join(projectDir, "compose.yaml"),
		EnvFile:     filepath.Join(projectDir, ".env.prod"),
		DBService:   "db",
		DBUser:      "espocrm",
		DBPassword:  "db-secret",
		DBName:      "espocrm",
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if strings.Contains(err.Error(), "db-secret") {
		t.Fatalf("runtime error leaked db password: %v", err)
	}
	if !strings.Contains(err.Error(), "MYSQL_PWD=<redacted>") {
		t.Fatalf("expected redacted runtime error, got %v", err)
	}
}

func TestDockerComposeResetDatabaseRuntimeErrorRedactsRootPassword(t *testing.T) {
	projectDir := t.TempDir()
	installFakeDocker(t)
	t.Setenv("TEST_DOCKER_FAIL_EXEC", "1")
	t.Setenv("TEST_DOCKER_FAIL_STDERR", "reset failed with MYSQL_PWD=root-secret and root-secret")

	rt := DockerCompose{}
	err := rt.ResetDatabase(context.Background(), Target{
		ProjectDir:     projectDir,
		ComposeFile:    filepath.Join(projectDir, "compose.yaml"),
		EnvFile:        filepath.Join(projectDir, ".env.prod"),
		DBService:      "db",
		DBRootPassword: "root-secret",
		DBName:         "espocrm",
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if strings.Contains(err.Error(), "root-secret") {
		t.Fatalf("runtime error leaked db root password: %v", err)
	}
	if !strings.Contains(err.Error(), "MYSQL_PWD=<redacted>") {
		t.Fatalf("expected redacted runtime error, got %v", err)
	}
}

func installFakeDocker(t *testing.T) string {
	t.Helper()

	rootDir := t.TempDir()
	binDir := filepath.Join(rootDir, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}

	logPath := filepath.Join(rootDir, "docker.log")
	scriptPath := filepath.Join(binDir, "docker")
	if err := os.WriteFile(scriptPath, []byte(fakeDockerScript), 0o755); err != nil {
		t.Fatal(err)
	}

	t.Setenv("TEST_DOCKER_LOG", logPath)
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return logPath
}

func mustReadFile(t *testing.T, path string) string {
	t.Helper()

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

const fakeDockerScript = `#!/usr/bin/env bash
set -Eeuo pipefail

printf '%s\n' "$*" >>"$TEST_DOCKER_LOG"

if [[ "${1:-}" != "compose" ]]; then
  printf 'unexpected docker invocation: %s\n' "$*" >&2
  exit 1
fi
shift

while [[ $# -gt 0 ]]; do
  case "$1" in
    --env-file|-f)
      shift 2
      ;;
    *)
      break
      ;;
  esac
done

case "${1:-}" in
  config)
    exit 0
    ;;
  ps)
    printf '%s' "${TEST_DOCKER_PS_OUTPUT:-[]}"
    exit 0
    ;;
  up)
    [[ "${2:-}" == "-d" ]] || exit 1
    exit 0
    ;;
  exec)
    shift
    [[ "${1:-}" == "-T" ]] || exit 1
    shift
    [[ "${1:-}" == "-e" ]] || exit 1
    shift
    [[ "${1:-}" == "MYSQL_PWD" ]] || exit 1
    shift
    [[ "${1:-}" == "db" ]] || exit 1
    shift
    if [[ "${TEST_DOCKER_FAIL_EXEC:-}" == "1" ]]; then
      printf '%s\n' "${TEST_DOCKER_FAIL_STDERR:-exec failed}" >&2
      exit 1
    fi
    case "${1:-}" in
      mariadb-dump)
        [[ "${MYSQL_PWD:-}" == "db-secret" ]] || exit 1
        printf 'create table test(id int);\n'
        exit 0
        ;;
      mariadb)
        shift
        [[ "${1:-}" == "-u" ]] || exit 1
        shift
        case "${1:-}" in
          root)
            [[ "${MYSQL_PWD:-}" == "root-secret" ]] || exit 1
            shift
            [[ "${1:-}" == "-e" ]] || exit 1
            shift
            [[ "${1:-}" == DROP\ DATABASE\ IF\ EXISTS*CREATE\ DATABASE*CHARACTER\ SET\ utf8mb4\ COLLATE\ utf8mb4_unicode_ci\; ]] || exit 1
            exit 0
            ;;
          espocrm)
            [[ "${MYSQL_PWD:-}" == "db-secret" ]] || exit 1
            shift
            for arg in "$@"; do
              if [[ "$arg" == "-e" ]]; then
                exit 0
              fi
            done
            cat >"${TEST_DOCKER_STDIN_LOG:-/dev/null}"
            exit 0
            ;;
        esac
        for arg in "$@"; do
          if [[ "$arg" == "-e" ]]; then
            exit 0
          fi
        done
        cat >"${TEST_DOCKER_STDIN_LOG:-/dev/null}"
        exit 0
        ;;
    esac
    exit 1
    ;;
  stop|start)
    exit 0
    ;;
esac

printf 'unexpected docker invocation: %s\n' "$*" >&2
exit 1
`
