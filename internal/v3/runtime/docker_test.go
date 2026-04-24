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
	if !strings.Contains(log, "compose --env-file "+filepath.Join(projectDir, ".env.prod")+" -f "+filepath.Join(projectDir, "compose.yaml")+" exec -T db mariadb-dump --single-transaction --quick --routines --triggers --events -u espocrm espocrm") {
		t.Fatalf("unexpected docker log:\n%s", log)
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
  exec)
    shift
    [[ "${1:-}" == "-T" ]] || exit 1
    shift
    [[ "${1:-}" == "db" ]] || exit 1
    shift
    [[ "${1:-}" == "mariadb-dump" ]] || exit 1
    [[ "${MYSQL_PWD:-}" == "db-secret" ]] || exit 1
    printf 'create table test(id int);\n'
    exit 0
    ;;
  ps)
    printf '[]'
    exit 0
    ;;
  stop|start)
    exit 0
    ;;
esac

printf 'unexpected docker invocation: %s\n' "$*" >&2
exit 1
`
