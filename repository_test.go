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
	"strconv"
	"strings"
	"testing"
)

const modulePath = "github.com/lazuale/espocrm-ops"

const (
	expectedMariaDBImage       = "mariadb:11.4"
	expectedRuntimeAppServices = "espocrm,espocrm-daemon,espocrm-websocket"
)

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

func TestMakefileKeepsBasicHealthCommands(t *testing.T) {
	text := string(readRepoFile(t, "Makefile"))

	if strings.Contains(text, "go test ./... -run Integration") {
		t.Fatal("Makefile integration target must not use fake Integration name filtering")
	}
	for _, target := range []string{
		"build",
		"mod-verify",
		"test-readonly",
		"test-race",
		"vet",
		"staticcheck",
		"lint",
		"integration-preflight",
		"pull-images",
		"integration",
		"ci-fast",
		"ci-integration",
		"mod-clean-check",
		"ci",
	} {
		assertMakefileTargetExists(t, text, target)
	}
	assertMakefileTargetDependsOn(t, text, "pull-images", "integration-preflight")
	assertMakefileTargetDependsOn(t, text, "integration", "pull-images")
	for _, dep := range []string{"build", "mod-verify", "test-readonly", "test-race", "vet", "staticcheck", "lint", "mod-clean-check"} {
		assertMakefileTargetDependsOn(t, text, "ci-fast", dep)
	}
	for _, dep := range []string{"pull-images", "integration"} {
		assertMakefileTargetDependsOn(t, text, "ci-integration", dep)
	}
	for _, dep := range []string{"ci-fast", "ci-integration"} {
		assertMakefileTargetDependsOn(t, text, "ci", dep)
	}
	for _, tokens := range [][]string{
		{"go", "build"},
		{"go", "mod", "verify"},
		{"go", "test", "-mod=readonly"},
		{"go", "test", "-race"},
		{"go", "vet"},
		{"staticcheck"},
		{"golangci-lint", "run"},
		{"docker", "info"},
		{"docker", "compose", "version"},
		{"docker", "pull"},
		{"go", "test", "-tags=integration"},
		{"git", "diff", "go.mod", "go.sum"},
	} {
		assertAnyLineHasTokens(t, "Makefile", strings.Split(text, "\n"), tokens...)
	}
}

func TestCIWorkflowRunsBasicHealthCommands(t *testing.T) {
	workflow := string(readRepoFile(t, ".github/workflows/ci.yml"))
	commands := workflowRunCommandsFromText(t, workflow)
	for _, tokens := range [][]string{
		{"make", "ci-fast"},
		{"make", "ci-integration"},
	} {
		assertAnyLineHasTokens(t, ".github/workflows/ci.yml run command", commands, tokens...)
	}
	if !strings.Contains(workflow, "pull_request:") {
		t.Fatal("workflow must keep pull_request coverage")
	}
	dockerJob := workflowJobBlock(t, workflow, "docker-integration")
	if !lineWithTokensExists(dockerJob, "if:", "pull_request", "!=") {
		t.Fatal("docker integration job must not run for pull_request events")
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
		if env["MARIADB_IMAGE"] != expectedMariaDBImage {
			t.Fatalf("%s must set MARIADB_IMAGE=%s, got %q", rel, expectedMariaDBImage, env["MARIADB_IMAGE"])
		}
		if _, ok := env["MARIADB_TAG"]; ok {
			t.Fatalf("%s must not contain deprecated MARIADB_TAG", rel)
		}
	}

	integration := string(readRepoFile(t, "internal/runtime/docker_integration_test.go"))
	if !strings.Contains(integration, `integrationMariaDBImage = "mariadb:11.4"`) {
		t.Fatal("integration fixture must target mariadb:11.4")
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

func TestWebsocketRuntimeContractIsExplicit(t *testing.T) {
	compose := string(readRepoFile(t, "compose.yaml"))
	for _, needle := range []string{
		"  espocrm:\n",
		"  espocrm-daemon:\n",
		"  espocrm-websocket:\n",
	} {
		if !strings.Contains(compose, needle) {
			t.Fatalf("compose.yaml must keep explicit application service %q", strings.TrimSpace(needle))
		}
	}

	for _, rel := range exampleEnvFiles {
		env := readEnvAssignmentsFile(t, rel)
		if env["APP_SERVICES"] != expectedRuntimeAppServices {
			t.Fatalf("%s must set APP_SERVICES=%s, got %q", rel, expectedRuntimeAppServices, env["APP_SERVICES"])
		}
	}
}

func TestExampleDBMemoryIsNotBelowMariaDBBufferPool(t *testing.T) {
	bufferPoolBytes := mariaDBBufferPoolBytes(t)

	for _, rel := range exampleEnvFiles {
		env := readEnvAssignmentsFile(t, rel)
		dbMemBytes := parseByteSize(t, rel, "DB_MEM_LIMIT", env["DB_MEM_LIMIT"])
		if dbMemBytes < bufferPoolBytes {
			t.Fatalf("%s sets DB_MEM_LIMIT=%s below innodb_buffer_pool_size", rel, env["DB_MEM_LIMIT"])
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

func assertMakefileTargetExists(t *testing.T, text, target string) {
	t.Helper()

	makefileTargetDeps(t, text, target)
}

func assertMakefileTargetDependsOn(t *testing.T, text, target, dep string) {
	t.Helper()

	deps := makefileTargetDeps(t, text, target)
	if !slices.Contains(deps, dep) {
		t.Fatalf("Makefile target %s must depend on %s; deps: %v", target, dep, deps)
	}
}

func makefileTargetDeps(t *testing.T, text, target string) []string {
	t.Helper()

	pattern := regexp.MustCompile(`(?m)^` + regexp.QuoteMeta(target) + `\s*:(.*)$`)
	matches := pattern.FindAllStringSubmatch(text, -1)
	if len(matches) != 1 {
		t.Fatalf("Makefile must contain exactly one %s target, found %d", target, len(matches))
	}
	return strings.Fields(matches[0][1])
}

func workflowRunCommandsFromText(t *testing.T, text string) []string {
	t.Helper()

	lines := strings.Split(text, "\n")
	var commands []string
	for i := 0; i < len(lines); i++ {
		line := lines[i]
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "run:") {
			continue
		}

		command := strings.TrimSpace(strings.TrimPrefix(trimmed, "run:"))
		if command != "" && command != "|" {
			commands = append(commands, command)
			continue
		}

		indent := leadingSpaces(line)
		for i+1 < len(lines) && leadingSpaces(lines[i+1]) > indent {
			i++
			blockLine := strings.TrimSpace(lines[i])
			if blockLine != "" && !strings.HasPrefix(blockLine, "#") {
				commands = append(commands, blockLine)
			}
		}
	}
	if len(commands) == 0 {
		t.Fatal("workflow must contain run commands")
	}
	return commands
}

func workflowJobBlock(t *testing.T, text, job string) []string {
	t.Helper()

	lines := strings.Split(text, "\n")
	header := "  " + job + ":"
	for i, line := range lines {
		if line != header {
			continue
		}

		block := []string{line}
		for _, next := range lines[i+1:] {
			if strings.HasPrefix(next, "  ") && !strings.HasPrefix(next, "    ") && strings.TrimSpace(next) != "" {
				break
			}
			block = append(block, next)
		}
		return block
	}
	t.Fatalf("workflow missing %s job", job)
	return nil
}

func assertAnyLineHasTokens(t *testing.T, label string, lines []string, tokens ...string) {
	t.Helper()

	if lineWithTokensExists(lines, tokens...) {
		return
	}
	t.Fatalf("%s missing line containing tokens %v", label, tokens)
}

func lineWithTokensExists(lines []string, tokens ...string) bool {
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		if lineHasTokens(trimmed, tokens...) {
			return true
		}
	}
	return false
}

func lineHasTokens(line string, tokens ...string) bool {
	for _, token := range tokens {
		if !strings.Contains(line, token) {
			return false
		}
	}
	return true
}

func leadingSpaces(line string) int {
	return len(line) - len(strings.TrimLeft(line, " "))
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

func mariaDBBufferPoolBytes(t *testing.T) int64 {
	t.Helper()

	pattern := regexp.MustCompile(`(?m)^\s*innodb_buffer_pool_size\s*=\s*([0-9]+[KMGkmg]?)\s*$`)
	match := pattern.FindStringSubmatch(string(readRepoFile(t, "deploy/mariadb/z-custom.cnf")))
	if len(match) != 2 {
		t.Fatal("deploy/mariadb/z-custom.cnf must define innodb_buffer_pool_size")
	}
	return parseByteSize(t, "deploy/mariadb/z-custom.cnf", "innodb_buffer_pool_size", match[1])
}

func parseByteSize(t *testing.T, rel, key, raw string) int64 {
	t.Helper()

	value := strings.TrimSpace(raw)
	if value == "" {
		t.Fatalf("%s must define %s", rel, key)
	}

	multiplier := int64(1)
	switch suffix := value[len(value)-1]; suffix {
	case 'k', 'K':
		multiplier = 1024
		value = value[:len(value)-1]
	case 'm', 'M':
		multiplier = 1024 * 1024
		value = value[:len(value)-1]
	case 'g', 'G':
		multiplier = 1024 * 1024 * 1024
		value = value[:len(value)-1]
	}

	number, err := strconv.ParseInt(value, 10, 64)
	if err != nil || number <= 0 {
		t.Fatalf("%s has invalid %s size %q", rel, key, raw)
	}
	return number * multiplier
}

func mapKeysSorted(values map[string]struct{}) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	return keys
}
