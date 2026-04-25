package repository_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

const modulePath = "github.com/lazuale/espocrm-ops"

var (
	exampleEnvFiles = []string{
		"env/.env.dev.example",
		"env/.env.prod.example",
	}
	goRequiredEnvKeys = []string{
		"BACKUP_ROOT",
		"BACKUP_NAME_PREFIX",
		"BACKUP_RETENTION_DAYS",
		"MIN_FREE_DISK_MB",
		"ESPO_STORAGE_DIR",
		"APP_SERVICES",
		"DB_SERVICE",
		"DB_USER",
		"DB_PASSWORD",
		"DB_NAME",
	}
	goRestoreEnvKeys = []string{
		"DB_ROOT_PASSWORD",
		"ESPO_RUNTIME_UID",
		"ESPO_RUNTIME_GID",
	}
	goOptionalEnvKeys = []string{
		"COMPOSE_FILE",
		"ESPO_CONTOUR",
	}
)

func TestProductionProcessEnvAccessSurfaceIsExplicit(t *testing.T) {
	root := repoRoot(t)
	fset := token.NewFileSet()
	allowedEnvironOwner := filepath.Join(root, "internal", "runtime", "docker.go")

	for _, path := range productionGoFiles(t, filepath.Join(root, "cmd"), filepath.Join(root, "internal")) {
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}

		ast.Inspect(file, func(node ast.Node) bool {
			selector, ok := node.(*ast.SelectorExpr)
			if !ok {
				return true
			}

			ident, ok := selector.X.(*ast.Ident)
			if !ok || ident.Name != "os" {
				return true
			}

			switch selector.Sel.Name {
			case "Getenv", "LookupEnv", "Getwd":
				t.Fatalf("production process env/path access %s must not appear at %s", selector.Sel.Name, fset.Position(selector.Pos()))
			case "Environ":
				if path != allowedEnvironOwner {
					t.Fatalf("os.Environ must stay in %s; found at %s", allowedEnvironOwner, fset.Position(selector.Pos()))
				}
			}

			return true
		})
	}
}

func TestCommandEntrypointUsesOnlyCLI(t *testing.T) {
	imports := goListLines(t, "list", "-f", "{{range .Imports}}{{.}}\n{{end}}", "./cmd/espops")
	if !slices.Contains(imports, modulePath+"/internal/cli") {
		t.Fatal("cmd/espops must enter the product through internal/cli")
	}

	var unexpected []string
	for _, imp := range imports {
		if strings.HasPrefix(imp, modulePath+"/internal/") && imp != modulePath+"/internal/cli" {
			unexpected = append(unexpected, imp)
		}
	}
	if len(unexpected) > 0 {
		t.Fatalf("cmd/espops must not bypass internal/cli: %v", unexpected)
	}
}

func TestProductionShellExecutionSurfaceIsExplicit(t *testing.T) {
	root := repoRoot(t)
	fset := token.NewFileSet()
	allowedExecOwner := filepath.Join(root, "internal", "runtime", "docker.go")

	for _, path := range productionGoFiles(t, filepath.Join(root, "cmd"), filepath.Join(root, "internal")) {
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}

		ast.Inspect(file, func(node ast.Node) bool {
			selector, ok := node.(*ast.SelectorExpr)
			if !ok {
				return true
			}

			ident, ok := selector.X.(*ast.Ident)
			if !ok || ident.Name != "exec" {
				return true
			}

			switch selector.Sel.Name {
			case "Command", "CommandContext":
				if path != allowedExecOwner {
					t.Fatalf("shell execution seam %s must stay in %s; found at %s", selector.Sel.Name, allowedExecOwner, fset.Position(selector.Pos()))
				}
			}

			return true
		})
	}
}

func TestGoSumIsTracked(t *testing.T) {
	files := trackedFiles(t)
	if !slices.Contains(files, "go.sum") {
		t.Fatal("go.sum must be tracked")
	}
}

func TestRepositoryHasTaggedIntegrationTests(t *testing.T) {
	root := repoRoot(t)
	var found bool

	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || !strings.HasSuffix(path, "_integration_test.go") {
			return nil
		}

		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		text := string(raw)
		if strings.Contains(text, "//go:build integration") && strings.Contains(text, "func TestIntegration") {
			found = true
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk integration tests: %v", err)
	}
	if !found {
		t.Fatal("expected at least one tagged integration test file")
	}
}

func TestComposeVariablesExistInExampleEnvFiles(t *testing.T) {
	composeKeys := composeEnvKeys(t)

	for _, rel := range exampleEnvFiles {
		env := readEnvAssignmentsFile(t, rel)
		for _, key := range composeKeys {
			value, ok := env[key]
			if !ok || strings.TrimSpace(value) == "" {
				t.Fatalf("%s must define compose env key %s", rel, key)
			}
		}
	}
}

func TestRequiredGoConfigKeysExistInExampleEnvFiles(t *testing.T) {
	requiredKeys := append([]string{}, goRequiredEnvKeys...)
	requiredKeys = append(requiredKeys, goRestoreEnvKeys...)

	for _, rel := range exampleEnvFiles {
		env := readEnvAssignmentsFile(t, rel)
		for _, key := range requiredKeys {
			value, ok := env[key]
			if !ok || strings.TrimSpace(value) == "" {
				t.Fatalf("%s must define required Go config key %s", rel, key)
			}
		}
	}
}

func TestEnvExamplesDoNotContainUnknownKeys(t *testing.T) {
	allowed := make(map[string]struct{})
	for _, key := range composeEnvKeys(t) {
		allowed[key] = struct{}{}
	}
	for _, key := range goRequiredEnvKeys {
		allowed[key] = struct{}{}
	}
	for _, key := range goRestoreEnvKeys {
		allowed[key] = struct{}{}
	}
	for _, key := range goOptionalEnvKeys {
		allowed[key] = struct{}{}
	}

	for _, rel := range exampleEnvFiles {
		env := readEnvAssignmentsFile(t, rel)
		for key := range env {
			if _, ok := allowed[key]; !ok {
				t.Fatalf("%s contains unknown env key %s", rel, key)
			}
		}
	}
}

func TestImageContractIsConsistent(t *testing.T) {
	compose := string(readRepoFile(t, "compose.yaml"))
	for _, needle := range []string{
		"image: ${ESPOCRM_IMAGE}",
		"image: ${MARIADB_IMAGE}",
	} {
		if !strings.Contains(compose, needle) {
			t.Fatalf("compose.yaml must consume image env key %q", needle)
		}
	}
	if strings.Contains(compose, "MARIADB_TAG") {
		t.Fatal("compose.yaml must not keep MARIADB_TAG after moving to MARIADB_IMAGE")
	}

	for _, rel := range exampleEnvFiles {
		env := readEnvAssignmentsFile(t, rel)
		if _, ok := env["MARIADB_TAG"]; ok {
			t.Fatalf("%s must not contain deprecated MARIADB_TAG", rel)
		}
	}
}

func TestComposePortsUseExplicitBindAddress(t *testing.T) {
	compose := string(readRepoFile(t, "compose.yaml"))
	for _, needle := range []string{
		`"${APP_BIND_ADDRESS}:${APP_PORT}:80"`,
		`"${WS_BIND_ADDRESS}:${WS_PORT}:8080"`,
	} {
		if !strings.Contains(compose, needle) {
			t.Fatalf("compose.yaml must use explicit bind address contract %q", needle)
		}
	}

	for _, rel := range exampleEnvFiles {
		env := readEnvAssignmentsFile(t, rel)
		if env["APP_BIND_ADDRESS"] != "127.0.0.1" {
			t.Fatalf("%s must set APP_BIND_ADDRESS=127.0.0.1, got %q", rel, env["APP_BIND_ADDRESS"])
		}
		if env["WS_BIND_ADDRESS"] != "127.0.0.1" {
			t.Fatalf("%s must set WS_BIND_ADDRESS=127.0.0.1, got %q", rel, env["WS_BIND_ADDRESS"])
		}
	}
}

func TestExampleRuntimeServicesExistAndHaveHealthchecks(t *testing.T) {
	serviceBlocks := composeServiceBlocks(t)

	for _, rel := range exampleEnvFiles {
		env := readEnvAssignmentsFile(t, rel)
		for _, service := range append([]string{requiredEnvValue(t, env, rel, "DB_SERVICE")}, requiredEnvServices(t, env, rel, "APP_SERVICES")...) {
			block, ok := serviceBlocks[service]
			if !ok {
				t.Fatalf("%s references service %q, but compose.yaml does not define it", rel, service)
			}
			if !strings.Contains(block, "    healthcheck:\n") {
				t.Fatalf("%s references service %q, but compose.yaml does not define a healthcheck for it", rel, service)
			}
		}
	}
}

func TestRuntimeContractDoesNotAdvertisePasswordFileKeys(t *testing.T) {
	for _, rel := range []string{
		"README.md",
		"CONTRIBUTING.md",
		"AGENTS.md",
		"compose.yaml",
		"env/.env.dev.example",
		"env/.env.prod.example",
		"internal/config/config.go",
	} {
		text := string(readRepoFile(t, rel))
		for _, needle := range []string{"DB_PASSWORD_FILE", "DB_ROOT_PASSWORD_FILE"} {
			if strings.Contains(text, needle) {
				t.Fatalf("%s must not advertise decorative password file key %s", rel, needle)
			}
		}
	}
}

func productionGoFiles(t *testing.T, roots ...string) []string {
	t.Helper()

	var files []string
	for _, root := range roots {
		err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			files = append(files, path)
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}

	slices.Sort(files)
	return files
}

func goListLines(t *testing.T, args ...string) []string {
	t.Helper()

	cmd := exec.Command("go", args...)
	cmd.Dir = repoRoot(t)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go %s: %v\n%s", strings.Join(args, " "), err, output)
	}

	lines := strings.Fields(string(output))
	slices.Sort(lines)
	return slices.Compact(lines)
}

func trackedFiles(t *testing.T) []string {
	t.Helper()

	cmd := exec.Command("git", "ls-files")
	cmd.Dir = repoRoot(t)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git ls-files: %v\n%s", err, output)
	}

	lines := strings.Fields(string(output))
	slices.Sort(lines)
	return slices.Compact(lines)
}

func readRepoFile(t *testing.T, rel string) []byte {
	t.Helper()

	raw, err := os.ReadFile(filepath.Join(repoRoot(t), rel))
	if err != nil {
		t.Fatalf("read %s: %v", rel, err)
	}
	return raw
}

func repoRoot(t *testing.T) string {
	t.Helper()

	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git rev-parse --show-toplevel: %v\n%s", err, output)
	}

	return strings.TrimSpace(string(output))
}

func composeEnvKeys(t *testing.T) []string {
	t.Helper()

	pattern := regexp.MustCompile(`\$\{([A-Z0-9_]+)\}`)
	matches := pattern.FindAllStringSubmatch(string(readRepoFile(t, "compose.yaml")), -1)
	keys := make(map[string]struct{}, len(matches))
	for _, match := range matches {
		keys[match[1]] = struct{}{}
	}
	return mapKeysSorted(keys)
}

func readEnvAssignmentsFile(t *testing.T, rel string) map[string]string {
	t.Helper()

	lines := strings.Split(string(readRepoFile(t, rel)), "\n")
	values := make(map[string]string)
	for lineNo, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}

		key, value, ok := strings.Cut(trimmed, "=")
		if !ok {
			t.Fatalf("%s:%d: expected KEY=VALUE", rel, lineNo+1)
		}
		key = strings.TrimSpace(key)
		if key == "" {
			t.Fatalf("%s:%d: empty env key", rel, lineNo+1)
		}
		if _, exists := values[key]; exists {
			t.Fatalf("%s:%d: duplicate env key %s", rel, lineNo+1, key)
		}
		values[key] = strings.TrimSpace(value)
	}
	return values
}

func requiredEnvValue(t *testing.T, env map[string]string, rel, key string) string {
	t.Helper()

	value := strings.TrimSpace(env[key])
	if value == "" {
		t.Fatalf("%s must define %s", rel, key)
	}
	return value
}

func requiredEnvServices(t *testing.T, env map[string]string, rel, key string) []string {
	t.Helper()

	raw := requiredEnvValue(t, env, rel, key)
	parts := strings.Split(raw, ",")
	services := make([]string, 0, len(parts))
	for _, part := range parts {
		service := strings.TrimSpace(part)
		if service == "" {
			t.Fatalf("%s must define non-empty service names in %s", rel, key)
		}
		services = append(services, service)
	}
	return services
}

func composeServiceBlocks(t *testing.T) map[string]string {
	t.Helper()

	serviceHeader := regexp.MustCompile(`^  ([A-Za-z0-9._-]+):\s*$`)
	blocks := make(map[string]string)
	inServices := false
	var current string
	var block strings.Builder

	for _, line := range strings.Split(string(readRepoFile(t, "compose.yaml")), "\n") {
		if line == "services:" {
			inServices = true
			current = ""
			block.Reset()
			continue
		}
		if !inServices {
			continue
		}
		if strings.HasPrefix(line, "  ") {
			if match := serviceHeader.FindStringSubmatch(line); len(match) == 2 {
				if current != "" {
					blocks[current] = block.String()
					block.Reset()
				}
				current = match[1]
			}
			if current != "" {
				block.WriteString(line)
				block.WriteByte('\n')
			}
			continue
		}
		if current != "" && strings.TrimSpace(line) != "" {
			blocks[current] = block.String()
			break
		}
	}
	if current != "" {
		blocks[current] = block.String()
	}
	return blocks
}

func mapKeysSorted(values map[string]struct{}) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	return keys
}
