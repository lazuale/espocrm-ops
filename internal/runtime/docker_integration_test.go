//go:build integration

package runtime_test

import (
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	config "github.com/lazuale/espocrm-ops/internal/config"
	"github.com/lazuale/espocrm-ops/internal/ops"
	runtime "github.com/lazuale/espocrm-ops/internal/runtime"
)

const (
	integrationMariaDBImage = "mariadb:11.4"
	integrationAppImage     = "alpine:3.20"
)

func TestIntegrationRuntimeMariaDBLifecycle(t *testing.T) {
	project := newIntegrationProject(t, "dev")
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	rt := runtime.DockerCompose{}
	if err := rt.ComposeConfig(ctx, project.target()); err != nil {
		t.Fatalf("ComposeConfig failed against real compose project: %v", err)
	}
	if err := rt.UpService(ctx, project.target(), "db"); err != nil {
		t.Fatalf("UpService(db) failed: %v", err)
	}
	project.waitForDB(t, ctx)

	if err := rt.ResetDatabase(ctx, project.target()); err != nil {
		t.Fatalf("ResetDatabase failed: %v", err)
	}

	restoreSQL := strings.Join([]string{
		"CREATE TABLE runtime_integration (id INT PRIMARY KEY);",
		"INSERT INTO runtime_integration (id) VALUES (1);",
		"",
	}, "\n")
	if err := rt.RestoreDatabase(ctx, project.target(), strings.NewReader(restoreSQL)); err != nil {
		t.Fatalf("RestoreDatabase failed: %v", err)
	}
	if err := rt.DBPing(ctx, project.target()); err != nil {
		t.Fatalf("DBPing failed after restore: %v", err)
	}

	dumpPath := filepath.Join(t.TempDir(), "runtime.sql.gz")
	if err := rt.DumpDatabase(ctx, project.target(), dumpPath); err != nil {
		t.Fatalf("DumpDatabase failed: %v", err)
	}

	dumpSQL := readGzipFile(t, dumpPath)
	for _, needle := range []string{
		"CREATE TABLE `runtime_integration`",
		"INSERT INTO `runtime_integration` VALUES",
		"(1)",
	} {
		if !strings.Contains(dumpSQL, needle) {
			t.Fatalf("database dump missing %q:\n%s", needle, dumpSQL)
		}
	}
}

func TestIntegrationDoctorPassesAgainstRealComposeProject(t *testing.T) {
	project := newIntegrationProject(t, "dev")
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	rt := runtime.DockerCompose{}
	if err := rt.UpService(ctx, project.target(), "db"); err != nil {
		t.Fatalf("UpService(db) failed: %v", err)
	}
	if err := rt.UpService(ctx, project.target(), "app"); err != nil {
		t.Fatalf("UpService(app) failed: %v", err)
	}
	project.waitForDB(t, ctx)

	result, err := ops.Doctor(ctx, project.backupRequest(), rt)
	if err != nil {
		t.Fatalf("Doctor failed against real compose project: %v", err)
	}

	wantChecks := []string{
		"config",
		"backup_root",
		"storage_dir",
		"compose_config",
		"services",
		"db_ping",
	}
	if len(result.Checks) != len(wantChecks) {
		t.Fatalf("unexpected doctor checks: %#v", result.Checks)
	}
	for i, name := range wantChecks {
		if result.Checks[i].Name != name || !result.Checks[i].OK {
			t.Fatalf("unexpected doctor check[%d]: %#v", i, result.Checks[i])
		}
	}
}

type integrationProject struct {
	projectDir     string
	composeFile    string
	envFile        string
	scope          string
	backupRoot     string
	storageDir     string
	dbService      string
	dbUser         string
	dbPassword     string
	dbRootPassword string
	dbName         string
	preexisting    map[string]bool
}

func newIntegrationProject(t *testing.T, scope string) integrationProject {
	t.Helper()

	requireDockerIntegration(t)

	projectDir := t.TempDir()
	backupRoot := filepath.Join(projectDir, "backups", scope)
	storageDir := filepath.Join(projectDir, "runtime", scope, "espo")
	for _, dir := range []string{backupRoot, storageDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("create integration directory %s: %v", dir, err)
		}
	}

	project := integrationProject{
		projectDir:     projectDir,
		composeFile:    filepath.Join(projectDir, "compose.yaml"),
		envFile:        filepath.Join(projectDir, ".env."+scope),
		scope:          scope,
		backupRoot:     backupRoot,
		storageDir:     storageDir,
		dbService:      "db",
		dbUser:         "espocrm",
		dbPassword:     "db-secret",
		dbRootPassword: "root-secret",
		dbName:         "espocrm_" + scope,
		preexisting:    integrationPreexistingImages(t),
	}

	if err := os.WriteFile(project.composeFile, []byte(project.composeYAML()), 0o644); err != nil {
		t.Fatalf("write integration compose file: %v", err)
	}
	if err := os.WriteFile(project.envFile, []byte(project.envBody()), 0o644); err != nil {
		t.Fatalf("write integration env file: %v", err)
	}

	t.Cleanup(func() {
		project.cleanup(t)
	})

	return project
}

func (p integrationProject) target() runtime.Target {
	return runtime.Target{
		ProjectDir:     p.projectDir,
		ComposeFile:    p.composeFile,
		EnvFile:        p.envFile,
		DBService:      p.dbService,
		DBUser:         p.dbUser,
		DBPassword:     p.dbPassword,
		DBRootPassword: p.dbRootPassword,
		DBName:         p.dbName,
	}
}

func (p integrationProject) backupRequest() config.BackupRequest {
	return config.BackupRequest{
		Scope:      p.scope,
		ProjectDir: p.projectDir,
	}
}

func (p integrationProject) composeYAML() string {
	return strings.Join([]string{
		"services:",
		"  db:",
		"    image: " + integrationMariaDBImage,
		"    environment:",
		"      MARIADB_ROOT_PASSWORD: ${DB_ROOT_PASSWORD}",
		"      MARIADB_DATABASE: ${DB_NAME}",
		"      MARIADB_USER: ${DB_USER}",
		"      MARIADB_PASSWORD: ${DB_PASSWORD}",
		"  app:",
		"    image: " + integrationAppImage,
		"    command: [\"sh\", \"-c\", \"trap 'exit 0' TERM INT; while true; do sleep 3600; done\"]",
		"",
	}, "\n")
}

func (p integrationProject) envBody() string {
	projectName := sanitizeComposeProjectName(filepath.Base(p.projectDir))
	return strings.Join([]string{
		"COMPOSE_PROJECT_NAME=" + projectName,
		"ESPO_CONTOUR=" + p.scope,
		"BACKUP_ROOT=./backups/" + p.scope,
		"BACKUP_NAME_PREFIX=integration-" + p.scope,
		"BACKUP_RETENTION_DAYS=0",
		"MIN_FREE_DISK_MB=1",
		"ESPO_STORAGE_DIR=./runtime/" + p.scope + "/espo",
		"APP_SERVICES=app",
		"DB_SERVICE=" + p.dbService,
		"DB_USER=" + p.dbUser,
		"DB_PASSWORD=" + p.dbPassword,
		"DB_ROOT_PASSWORD=" + p.dbRootPassword,
		"DB_NAME=" + p.dbName,
		"",
	}, "\n")
}

func (p integrationProject) waitForDB(t *testing.T, ctx context.Context) {
	t.Helper()

	rt := runtime.DockerCompose{}
	deadline := time.Now().Add(90 * time.Second)
	var lastErr error
	for time.Now().Before(deadline) {
		if err := rt.DBPing(ctx, p.target()); err == nil {
			return
		} else {
			lastErr = err
		}
		time.Sleep(2 * time.Second)
	}
	t.Fatalf("database never became reachable: %v", lastErr)
}

func (p integrationProject) cleanup(t *testing.T) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	if t.Failed() {
		if logs, err := p.runDocker(ctx, "compose", "--env-file", p.envFile, "-f", p.composeFile, "logs", "--no-color"); err == nil {
			t.Logf("docker compose logs:\n%s", strings.TrimSpace(logs))
		}
	}
	if _, err := p.runDocker(ctx, "compose", "--env-file", p.envFile, "-f", p.composeFile, "down", "-v", "--remove-orphans"); err != nil {
		t.Logf("docker compose down failed: %v", err)
	}
	for _, image := range []string{integrationMariaDBImage, integrationAppImage} {
		if p.preexisting[image] {
			continue
		}
		p.removePulledImage(t, image)
	}
}

func (p integrationProject) runDocker(ctx context.Context, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "docker", args...)
	cmd.Dir = p.projectDir
	output, err := cmd.CombinedOutput()
	if err != nil {
		return string(output), fmt.Errorf("docker %s: %s: %w", strings.Join(args, " "), strings.TrimSpace(string(output)), err)
	}
	return string(output), nil
}

func requireDockerIntegration(t *testing.T) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	for _, command := range [][]string{
		{"docker", "info"},
		{"docker", "compose", "version"},
	} {
		cmd := exec.CommandContext(ctx, command[0], command[1:]...)
		output, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("integration requires %s to succeed: %v\n%s", strings.Join(command, " "), err, strings.TrimSpace(string(output)))
		}
	}

	for _, image := range []string{integrationMariaDBImage, integrationAppImage} {
		exists, err := dockerImageExists(ctx, image)
		if err != nil {
			t.Fatalf("integration could not check local image %s: %v", image, err)
		}
		if !exists {
			t.Fatalf(
				"integration requires local image %s; run `make pull-images` first or restore Docker Hub access. Missing local images means the registry path is not proven, not that Go code is broken.",
				image,
			)
		}
	}
}

func integrationPreexistingImages(t *testing.T) map[string]bool {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	present := make(map[string]bool, 2)
	for _, image := range []string{integrationMariaDBImage, integrationAppImage} {
		exists, err := dockerImageExists(ctx, image)
		if err != nil {
			t.Fatalf("detect docker image %s: %v", image, err)
		}
		present[image] = exists
	}
	return present
}

func (p integrationProject) removePulledImage(t *testing.T, image string) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	if _, err := p.runDocker(ctx, "image", "rm", image); err != nil {
		stillPresent, inspectErr := dockerImageExists(ctx, image)
		if inspectErr != nil {
			t.Errorf("re-check docker image %s after cleanup failure: %v", image, inspectErr)
			return
		}
		if stillPresent {
			t.Errorf("integration cleanup left pulled image %s behind: %v", image, err)
		}
	}
}

func dockerImageExists(ctx context.Context, image string) (bool, error) {
	cmd := exec.CommandContext(ctx, "docker", "image", "inspect", image)
	output, err := cmd.CombinedOutput()
	if err == nil {
		return true, nil
	}

	text := strings.ToLower(strings.TrimSpace(string(output)))
	if strings.Contains(text, "no such image") || strings.Contains(text, "no such object") {
		return false, nil
	}
	return false, fmt.Errorf("docker image inspect %s: %s: %w", image, strings.TrimSpace(string(output)), err)
}

func readGzipFile(t *testing.T, path string) string {
	t.Helper()

	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if closeErr := file.Close(); closeErr != nil {
			t.Fatal(closeErr)
		}
	}()

	reader, err := gzip.NewReader(file)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if closeErr := reader.Close(); closeErr != nil {
			t.Fatal(closeErr)
		}
	}()

	body, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}

func sanitizeComposeProjectName(value string) string {
	var out strings.Builder
	for _, ch := range value {
		switch {
		case 'a' <= ch && ch <= 'z':
			out.WriteRune(ch)
		case 'A' <= ch && ch <= 'Z':
			out.WriteRune(ch + ('a' - 'A'))
		case '0' <= ch && ch <= '9':
			out.WriteRune(ch)
		default:
			out.WriteByte('-')
		}
	}
	name := strings.Trim(out.String(), "-")
	if name == "" {
		return "espops-integration"
	}
	return "espops-" + name
}
