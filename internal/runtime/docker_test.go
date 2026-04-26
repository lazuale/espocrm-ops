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

func TestDockerComposeDoesNotExposeHostRuntimeContractEnv(t *testing.T) {
	projectDir := t.TempDir()
	logPath := installFakeDocker(t)

	for _, key := range runtimeContractEnvKeys() {
		t.Setenv(key, "host-"+key)
	}
	t.Setenv("MYSQL_PWD", "host-mysql-secret")

	rt := DockerCompose{}
	err := rt.ComposeConfig(context.Background(), Target{
		ProjectDir:  projectDir,
		ComposeFile: filepath.Join(projectDir, "compose.yaml"),
		EnvFile:     filepath.Join(projectDir, ".env.prod"),
	})
	if err != nil {
		t.Fatalf("ComposeConfig failed: %v", err)
	}

	envLog := mustReadFile(t, fakeDockerEnvLogPath(logPath))
	for _, key := range append(runtimeContractEnvKeys(), "MYSQL_PWD") {
		if envLogContainsKey(envLog, key) {
			t.Fatalf("docker command inherited host env %s:\n%s", key, envLog)
		}
	}
}

func TestDockerComposeKeepsDockerSystemEnv(t *testing.T) {
	projectDir := t.TempDir()
	logPath := installFakeDocker(t)

	t.Setenv("DOCKER_HOST", "unix:///tmp/espops-docker.sock")
	t.Setenv("DOCKER_CONTEXT", "espops-context")
	t.Setenv("DOCKER_CONFIG", filepath.Join(projectDir, ".docker"))
	t.Setenv("DOCKER_CERT_PATH", filepath.Join(projectDir, "certs"))
	t.Setenv("DOCKER_TLS_VERIFY", "1")
	t.Setenv("SSH_AUTH_SOCK", filepath.Join(projectDir, "ssh-agent.sock"))
	t.Setenv("XDG_RUNTIME_DIR", filepath.Join(projectDir, "xdg-runtime"))
	t.Setenv("HTTP_PROXY", "http://proxy.example:8080")
	t.Setenv("http_proxy", "http://proxy.example:8081")

	rt := DockerCompose{}
	err := rt.ComposeConfig(context.Background(), Target{
		ProjectDir:  projectDir,
		ComposeFile: filepath.Join(projectDir, "compose.yaml"),
		EnvFile:     filepath.Join(projectDir, ".env.prod"),
	})
	if err != nil {
		t.Fatalf("ComposeConfig failed: %v", err)
	}

	envLog := mustReadFile(t, fakeDockerEnvLogPath(logPath))
	for _, want := range []string{
		"DOCKER_HOST=unix:///tmp/espops-docker.sock",
		"DOCKER_CONTEXT=espops-context",
		"DOCKER_CONFIG=" + filepath.Join(projectDir, ".docker"),
		"DOCKER_CERT_PATH=" + filepath.Join(projectDir, "certs"),
		"DOCKER_TLS_VERIFY=1",
		"SSH_AUTH_SOCK=" + filepath.Join(projectDir, "ssh-agent.sock"),
		"XDG_RUNTIME_DIR=" + filepath.Join(projectDir, "xdg-runtime"),
		"HTTP_PROXY=http://proxy.example:8080",
		"http_proxy=http://proxy.example:8081",
	} {
		if !envLogContainsEntry(envLog, want) {
			t.Fatalf("docker command did not inherit %s:\n%s", want, envLog)
		}
	}
}

func TestCommandEnvKeepsOnlyDockerSystemEnvAndExplicitMySQLPwd(t *testing.T) {
	env := commandEnv([]string{
		"PATH=/bin",
		"DB_NAME=host_db",
		"COMPOSE_PROJECT_NAME=host_project",
		"DOCKER_HOST=unix:///tmp/docker.sock",
		"HTTP_PROXY=http://proxy.example:8080",
	}, []string{
		"DB_PASSWORD=override-secret",
		"MYSQL_PWD=db-secret",
	})
	log := strings.Join(env, "\n")

	for _, want := range []string{
		"PATH=/bin",
		"DOCKER_HOST=unix:///tmp/docker.sock",
		"HTTP_PROXY=http://proxy.example:8080",
		"MYSQL_PWD=db-secret",
	} {
		if !envLogContainsEntry(log, want) {
			t.Fatalf("command env missing %s from %#v", want, env)
		}
	}
	for _, key := range []string{"DB_NAME", "COMPOSE_PROJECT_NAME", "DB_PASSWORD"} {
		if envLogContainsKey(log, key) {
			t.Fatalf("command env kept %s in %#v", key, env)
		}
	}
}

func TestCreateTarGzRunsNativeTarWithExactArgvAndNulList(t *testing.T) {
	rootDir := installFakeTar(t)
	sourceDir := filepath.Join(t.TempDir(), "storage")
	if err := os.MkdirAll(sourceDir, 0o755); err != nil {
		t.Fatal(err)
	}
	destPath := filepath.Join(t.TempDir(), "files.tar.gz")

	err := CreateTarGz(context.Background(), sourceDir, destPath, []string{
		"nested",
		"nested/file.txt",
	})
	if err != nil {
		t.Fatalf("CreateTarGz failed: %v", err)
	}

	rawArgv, err := os.ReadFile(filepath.Join(rootDir, "tar.argv"))
	if err != nil {
		t.Fatal(err)
	}
	gotArgv := strings.Split(strings.TrimSuffix(string(rawArgv), "\n"), "\n")
	wantArgv := []string{
		"-C",
		sourceDir,
		"--no-recursion",
		"--null",
		"-T",
		"-",
		"-czf",
		destPath,
	}
	if strings.Join(gotArgv, "\n") != strings.Join(wantArgv, "\n") {
		t.Fatalf("unexpected tar argv:\ngot  %#v\nwant %#v", gotArgv, wantArgv)
	}

	stdin, err := os.ReadFile(filepath.Join(rootDir, "tar.stdin"))
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(stdin), "nested\x00nested/file.txt\x00"; got != want {
		t.Fatalf("unexpected tar stdin: got %q want %q", got, want)
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

	wantArgv := []string{
		"compose",
		"--env-file", filepath.Join(projectDir, ".env.prod"),
		"-f", filepath.Join(projectDir, "compose.yaml"),
		"exec", "-T", "-e", "MYSQL_PWD", "db",
		"mariadb-dump",
		"--single-transaction",
		"--quick",
		"--routines",
		"--triggers",
		"--events",
		"--hex-blob",
		"--default-character-set=utf8mb4",
		"-u", "espocrm",
		"espocrm",
	}
	if gotArgv := fakeDockerArgv(t, logPath); strings.Join(gotArgv, "\n") != strings.Join(wantArgv, "\n") {
		t.Fatalf("unexpected docker argv:\ngot  %#v\nwant %#v", gotArgv, wantArgv)
	}

	log := mustReadFile(t, logPath)
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

func TestDockerComposeStartServicesRunsComposeUp(t *testing.T) {
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

	wantArgv := []string{
		"compose",
		"--env-file", filepath.Join(projectDir, ".env.prod"),
		"-f", filepath.Join(projectDir, "compose.yaml"),
		"up", "-d", "espocrm", "espocrm-daemon",
	}
	if gotArgv := fakeDockerArgv(t, logPath); strings.Join(gotArgv, "\n") != strings.Join(wantArgv, "\n") {
		t.Fatalf("unexpected docker argv:\ngot  %#v\nwant %#v", gotArgv, wantArgv)
	}
}

func TestDockerComposeStartServicesMissingServiceFails(t *testing.T) {
	projectDir := t.TempDir()
	logPath := installFakeDocker(t)
	setFakeDockerUpMissingService(t, logPath, "missing")

	rt := DockerCompose{}
	err := rt.StartServices(context.Background(), Target{
		ProjectDir:  projectDir,
		ComposeFile: filepath.Join(projectDir, "compose.yaml"),
		EnvFile:     filepath.Join(projectDir, ".env.prod"),
	}, []string{"espocrm", "missing"})
	if err == nil {
		t.Fatal("expected missing service error")
	}
	if !strings.Contains(err.Error(), "no such service: missing") {
		t.Fatalf("unexpected error: %v", err)
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

	wantArgv := []string{
		"compose",
		"--env-file", filepath.Join(projectDir, ".env.prod"),
		"-f", filepath.Join(projectDir, "compose.yaml"),
		"up", "-d", "db",
	}
	if gotArgv := fakeDockerArgv(t, logPath); strings.Join(gotArgv, "\n") != strings.Join(wantArgv, "\n") {
		t.Fatalf("unexpected docker argv:\ngot  %#v\nwant %#v", gotArgv, wantArgv)
	}
}

func TestDockerComposeServiceStatusesAcceptsJSONLines(t *testing.T) {
	projectDir := t.TempDir()
	logPath := installFakeDocker(t)
	setFakeDockerPSOutput(t, logPath, "{\"Service\":\"db\",\"State\":\"running\",\"Health\":\"healthy\"}\n{\"Service\":\"espocrm\",\"State\":\"restarting\",\"Health\":\"starting\"}\n")

	rt := DockerCompose{}
	statuses, err := rt.ServiceStatuses(context.Background(), Target{
		ProjectDir:  projectDir,
		ComposeFile: filepath.Join(projectDir, "compose.yaml"),
		EnvFile:     filepath.Join(projectDir, ".env.prod"),
	})
	if err != nil {
		t.Fatalf("ServiceStatuses failed: %v", err)
	}
	if len(statuses) != 2 {
		t.Fatalf("unexpected statuses length: %#v", statuses)
	}
	if statuses[0].Name != "db" || statuses[0].State != "running" || statuses[0].Health != "healthy" {
		t.Fatalf("unexpected status[0]: %#v", statuses[0])
	}
	if statuses[1].Name != "espocrm" || statuses[1].State != "restarting" || statuses[1].Health != "starting" {
		t.Fatalf("unexpected status[1]: %#v", statuses[1])
	}
}

func TestDockerComposeRequireHealthyServicesPassesForHealthyRunningServices(t *testing.T) {
	projectDir := t.TempDir()
	logPath := installFakeDocker(t)
	setFakeDockerPSOutput(t, logPath, strings.Join([]string{
		`{"Service":"db","State":"running","Health":"healthy"}`,
		`{"Service":"espocrm","State":"running","Health":"healthy"}`,
		"",
	}, "\n"))

	rt := DockerCompose{}
	err := rt.RequireHealthyServices(context.Background(), Target{
		ProjectDir:  projectDir,
		ComposeFile: filepath.Join(projectDir, "compose.yaml"),
		EnvFile:     filepath.Join(projectDir, ".env.prod"),
	}, []string{"db", "espocrm"})
	if err != nil {
		t.Fatalf("RequireHealthyServices failed: %v", err)
	}
}

func TestDockerComposeRequireHealthyServicesFailsWithoutHealthcheck(t *testing.T) {
	projectDir := t.TempDir()
	logPath := installFakeDocker(t)
	setFakeDockerPSOutput(t, logPath, `[{"Service":"db","State":"running","Health":""}]`)

	rt := DockerCompose{}
	err := rt.RequireHealthyServices(context.Background(), Target{
		ProjectDir:  projectDir,
		ComposeFile: filepath.Join(projectDir, "compose.yaml"),
		EnvFile:     filepath.Join(projectDir, ".env.prod"),
	}, []string{"db"})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), `service "db" has no docker compose health status`) {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDockerComposeRequireHealthyServicesFailsForUnhealthyStateOrMissingService(t *testing.T) {
	projectDir := t.TempDir()
	logPath := installFakeDocker(t)

	rt := DockerCompose{}
	target := Target{
		ProjectDir:  projectDir,
		ComposeFile: filepath.Join(projectDir, "compose.yaml"),
		EnvFile:     filepath.Join(projectDir, ".env.prod"),
	}

	for _, tc := range []struct {
		name   string
		output string
		want   string
	}{
		{
			name:   "unhealthy",
			output: `[{"Service":"db","State":"running","Health":"unhealthy"}]`,
			want:   `service "db" health is "unhealthy"`,
		},
		{
			name:   "exited",
			output: `[{"Service":"db","State":"exited","Health":"unhealthy"}]`,
			want:   `service "db" state is "exited"`,
		},
		{
			name:   "restarting",
			output: `[{"Service":"db","State":"restarting","Health":"starting"}]`,
			want:   `service "db" state is "restarting"`,
		},
		{
			name:   "missing",
			output: `[{"Service":"espocrm","State":"running","Health":"healthy"}]`,
			want:   `service "db" not found`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			setFakeDockerPSOutput(t, logPath, tc.output)
			err := rt.RequireHealthyServices(context.Background(), target, []string{"db"})
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestDockerComposeRequireStoppedServicesPassesForStoppedOrMissingServices(t *testing.T) {
	projectDir := t.TempDir()
	logPath := installFakeDocker(t)
	setFakeDockerPSOutput(t, logPath, `[{"Service":"espocrm","State":"exited","Health":""},{"Service":"worker","State":"created","Health":""}]`)

	rt := DockerCompose{}
	err := rt.RequireStoppedServices(context.Background(), Target{
		ProjectDir:  projectDir,
		ComposeFile: filepath.Join(projectDir, "compose.yaml"),
		EnvFile:     filepath.Join(projectDir, ".env.prod"),
	}, []string{"espocrm", "worker", "not-present"})
	if err != nil {
		t.Fatalf("RequireStoppedServices failed: %v", err)
	}
}

func TestDockerComposeRequireStoppedServicesFailsForRunningService(t *testing.T) {
	projectDir := t.TempDir()
	logPath := installFakeDocker(t)
	setFakeDockerPSOutput(t, logPath, `[{"Service":"espocrm","State":"running","Health":"healthy"}]`)

	rt := DockerCompose{}
	err := rt.RequireStoppedServices(context.Background(), Target{
		ProjectDir:  projectDir,
		ComposeFile: filepath.Join(projectDir, "compose.yaml"),
		EnvFile:     filepath.Join(projectDir, ".env.prod"),
	}, []string{"espocrm"})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), `service "espocrm" state is "running" (want stopped)`) {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDockerComposeServiceStatusesFailsForMalformedJSON(t *testing.T) {
	projectDir := t.TempDir()
	logPath := installFakeDocker(t)
	setFakeDockerPSOutput(t, logPath, `{"Service":"db"`)

	rt := DockerCompose{}
	_, err := rt.ServiceStatuses(context.Background(), Target{
		ProjectDir:  projectDir,
		ComposeFile: filepath.Join(projectDir, "compose.yaml"),
		EnvFile:     filepath.Join(projectDir, ".env.prod"),
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "decode docker compose ps output") {
		t.Fatalf("unexpected error: %v", err)
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

func TestDockerComposeMariadbExecUsesExplicitMySQLPwd(t *testing.T) {
	projectDir := t.TempDir()
	logPath := installFakeDocker(t)
	t.Setenv("MYSQL_PWD", "host-secret")

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

	envLog := mustReadFile(t, fakeDockerEnvLogPath(logPath))
	if envLogContainsEntry(envLog, "MYSQL_PWD=host-secret") {
		t.Fatalf("docker command inherited host MYSQL_PWD:\n%s", envLog)
	}
	if !envLogContainsEntry(envLog, "MYSQL_PWD=db-secret") {
		t.Fatalf("docker command did not receive explicit MYSQL_PWD:\n%s", envLog)
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
	setFakeDockerStdinLog(t, logPath, stdinLogPath)

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
	logPath := installFakeDocker(t)
	setFakeDockerExecFailure(t, logPath, "db ping failed with MYSQL_PWD=db-secret")

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
	logPath := installFakeDocker(t)
	setFakeDockerExecFailure(t, logPath, "reset failed with MYSQL_PWD=root-secret and root-secret")

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

	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return logPath
}

func installFakeTar(t *testing.T) string {
	t.Helper()

	rootDir := t.TempDir()
	binDir := filepath.Join(rootDir, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}

	scriptPath := filepath.Join(binDir, "tar")
	if err := os.WriteFile(scriptPath, []byte(fakeTarScript), 0o755); err != nil {
		t.Fatal(err)
	}

	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return rootDir
}

func setFakeDockerPSOutput(t *testing.T, logPath, output string) {
	t.Helper()
	writeFakeDockerControl(t, logPath, "ps-output", output)
}

func setFakeDockerStdinLog(t *testing.T, logPath, path string) {
	t.Helper()
	writeFakeDockerControl(t, logPath, "stdin-log-path", path)
}

func setFakeDockerExecFailure(t *testing.T, logPath, stderr string) {
	t.Helper()
	writeFakeDockerControl(t, logPath, "fail-exec", "1")
	writeFakeDockerControl(t, logPath, "fail-stderr", stderr)
}

func setFakeDockerUpMissingService(t *testing.T, logPath, service string) {
	t.Helper()
	writeFakeDockerControl(t, logPath, "up-missing-service", service)
}

func writeFakeDockerControl(t *testing.T, logPath, name, value string) {
	t.Helper()

	if err := os.WriteFile(filepath.Join(fakeDockerRoot(logPath), name), []byte(value), 0o644); err != nil {
		t.Fatal(err)
	}
}

func fakeDockerRoot(logPath string) string {
	return filepath.Dir(logPath)
}

func fakeDockerEnvLogPath(logPath string) string {
	return filepath.Join(fakeDockerRoot(logPath), "docker.env")
}

func fakeDockerArgv(t *testing.T, logPath string) []string {
	t.Helper()
	raw := mustReadFile(t, filepath.Join(fakeDockerRoot(logPath), "docker.argv"))
	return strings.Split(strings.TrimSuffix(raw, "\n"), "\n")
}

func mustReadFile(t *testing.T, path string) string {
	t.Helper()

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func runtimeContractEnvKeys() []string {
	return []string{
		"ADMIN_PASSWORD",
		"ADMIN_USERNAME",
		"APP_BIND_ADDRESS",
		"APP_PORT",
		"APP_SERVICES",
		"BACKUP_NAME_PREFIX",
		"BACKUP_RETENTION_DAYS",
		"BACKUP_ROOT",
		"COMPOSE_FILE",
		"COMPOSE_PROJECT_NAME",
		"DAEMON_CPUS",
		"DAEMON_MEM_LIMIT",
		"DAEMON_PIDS_LIMIT",
		"DB_CPUS",
		"DB_MEM_LIMIT",
		"DB_NAME",
		"DB_PASSWORD",
		"DB_PIDS_LIMIT",
		"DB_ROOT_PASSWORD",
		"DB_SERVICE",
		"DB_STORAGE_DIR",
		"DB_USER",
		"DOCKER_LOG_MAX_FILE",
		"DOCKER_LOG_MAX_SIZE",
		"ESPO_CONTOUR",
		"ESPO_DEFAULT_LANGUAGE",
		"ESPO_LOGGER_LEVEL",
		"ESPO_MEM_LIMIT",
		"ESPO_CPUS",
		"ESPO_PIDS_LIMIT",
		"ESPO_RUNTIME_GID",
		"ESPO_RUNTIME_UID",
		"ESPO_STORAGE_DIR",
		"ESPO_TIME_ZONE",
		"ESPOCRM_IMAGE",
		"MARIADB_IMAGE",
		"MIN_FREE_DISK_MB",
		"SITE_URL",
		"WS_BIND_ADDRESS",
		"WS_CPUS",
		"WS_MEM_LIMIT",
		"WS_PIDS_LIMIT",
		"WS_PORT",
		"WS_PUBLIC_URL",
	}
}

func envLogContainsKey(log, key string) bool {
	return envLogContainsEntryPrefix(log, key+"=")
}

func envLogContainsEntry(log, entry string) bool {
	return envLogContainsEntryPrefix(log, entry)
}

func envLogContainsEntryPrefix(log, prefix string) bool {
	for _, line := range strings.Split(log, "\n") {
		if strings.HasPrefix(line, prefix) {
			return true
		}
	}
	return false
}

const fakeDockerScript = `#!/usr/bin/env bash
set -Eeuo pipefail

script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
fake_root="$(cd -- "$script_dir/.." && pwd)"

printf '%s\n' "$*" >>"$fake_root/docker.log"
: >"$fake_root/docker.argv"
for arg in "$@"; do
  printf '%s\n' "$arg" >>"$fake_root/docker.argv"
done
env | LC_ALL=C sort >>"$fake_root/docker.env"
printf '\n' >>"$fake_root/docker.env"

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
    if [[ -f "$fake_root/ps-output" ]]; then
      cat "$fake_root/ps-output"
    else
      printf '[]'
    fi
    exit 0
    ;;
  up)
    [[ "${2:-}" == "-d" ]] || exit 1
    shift 2
    missing=""
    if [[ -f "$fake_root/up-missing-service" ]]; then
      missing="$(cat "$fake_root/up-missing-service")"
    fi
    for service in "$@"; do
      if [[ -n "$missing" && "$service" == "$missing" ]]; then
        printf 'no such service: %s\n' "$service" >&2
        exit 1
      fi
    done
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
    if [[ -f "$fake_root/fail-exec" ]]; then
      if [[ -f "$fake_root/fail-stderr" ]]; then
        cat "$fake_root/fail-stderr" >&2
        printf '\n' >&2
      else
        printf 'exec failed\n' >&2
      fi
      exit 1
    fi
    case "${1:-}" in
      mariadb-dump)
        [[ "${MYSQL_PWD:-}" == "db-secret" ]] || exit 1
        shift
        [[ "${1:-}" == "--single-transaction" ]] || exit 1
        shift
        [[ "${1:-}" == "--quick" ]] || exit 1
        shift
        [[ "${1:-}" == "--routines" ]] || exit 1
        shift
        [[ "${1:-}" == "--triggers" ]] || exit 1
        shift
        [[ "${1:-}" == "--events" ]] || exit 1
        shift
        [[ "${1:-}" == "--hex-blob" ]] || exit 1
        shift
        [[ "${1:-}" == "--default-character-set=utf8mb4" ]] || exit 1
        shift
        [[ "${1:-}" == "-u" ]] || exit 1
        shift
        [[ "${1:-}" == "espocrm" ]] || exit 1
        shift
        [[ "${1:-}" == "espocrm" ]] || exit 1
        shift
        [[ $# -eq 0 ]] || exit 1
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
            if [[ -f "$fake_root/stdin-log-path" ]]; then
              cat >"$(cat "$fake_root/stdin-log-path")"
            else
              cat >/dev/null
            fi
            exit 0
            ;;
        esac
        for arg in "$@"; do
          if [[ "$arg" == "-e" ]]; then
            exit 0
          fi
        done
        if [[ -f "$fake_root/stdin-log-path" ]]; then
          cat >"$(cat "$fake_root/stdin-log-path")"
        else
          cat >/dev/null
        fi
        exit 0
        ;;
    esac
    exit 1
    ;;
  stop)
    exit 0
    ;;
esac

printf 'unexpected docker invocation: %s\n' "$*" >&2
exit 1
`

const fakeTarScript = `#!/usr/bin/env bash
set -Eeuo pipefail

script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
fake_root="$(cd -- "$script_dir/.." && pwd)"

: >"$fake_root/tar.argv"
for arg in "$@"; do
  printf '%s\n' "$arg" >>"$fake_root/tar.argv"
done
cat >"$fake_root/tar.stdin"

dest=""
while [[ $# -gt 0 ]]; do
  if [[ "$1" == "-czf" ]]; then
    shift
    dest="${1:-}"
    break
  fi
  shift
done
[[ -n "$dest" ]] || exit 1
printf 'fake tar output\n' >"$dest"
`
