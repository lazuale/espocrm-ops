package repository_test

import (
	"bufio"
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
	expectedMariaDBTag         = "11.4"
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

func TestInternalPackagesAreFlat(t *testing.T) {
	got := listImportPaths(t, "list", "-f", "{{.ImportPath}}", "./internal/...")
	want := []string{
		modulePath + "/internal/cli",
		modulePath + "/internal/config",
		modulePath + "/internal/manifest",
		modulePath + "/internal/ops",
		modulePath + "/internal/runtime",
	}

	if !slices.Equal(got, want) {
		t.Fatalf("unexpected internal packages:\n got: %v\nwant: %v", got, want)
	}
}

func TestUnexpectedInternalDirectoriesAreAbsent(t *testing.T) {
	root := repoRoot(t)
	entries, err := os.ReadDir(filepath.Join(root, "internal"))
	if err != nil {
		t.Fatalf("read internal/: %v", err)
	}

	var got []string
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		got = append(got, entry.Name())
	}
	slices.Sort(got)

	want := []string{"cli", "config", "manifest", "ops", "runtime"}
	if !slices.Equal(got, want) {
		t.Fatalf("unexpected internal directories:\n got: %v\nwant: %v", got, want)
	}
}

func TestCommandDoesNotPullUnexpectedInternalPackages(t *testing.T) {
	deps := listImportPaths(t, "list", "-deps", "-f", "{{.ImportPath}}", "./cmd/espops")
	var unexpectedDeps []string
	for _, dep := range deps {
		switch {
		case dep == modulePath+"/internal/cli":
		case dep == modulePath+"/internal/config":
		case dep == modulePath+"/internal/manifest":
		case dep == modulePath+"/internal/ops":
		case dep == modulePath+"/internal/runtime":
		case strings.HasPrefix(dep, modulePath+"/internal/"):
			unexpectedDeps = append(unexpectedDeps, dep)
		}
	}

	if len(unexpectedDeps) > 0 {
		t.Fatalf("cmd/espops still pulls unexpected internal packages: %v", unexpectedDeps)
	}
}

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

func TestDocsUseOnlyCurrentInternalLayout(t *testing.T) {
	root := repoRoot(t)
	docs := []string{
		"AGENTS.md",
		"README.md",
		"CONTRIBUTING.md",
	}
	allowed := map[string]struct{}{
		"internal/cli/":      {},
		"internal/config/":   {},
		"internal/manifest/": {},
		"internal/ops/":      {},
		"internal/runtime/":  {},
	}
	pattern := regexp.MustCompile(`internal/[a-z0-9_]+/`)

	for _, rel := range docs {
		path := filepath.Join(root, rel)
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}

		text := string(raw)
		for _, match := range pattern.FindAllString(text, -1) {
			if _, ok := allowed[match]; !ok {
				t.Fatalf("doc %s contains unexpected internal layout path %q", rel, match)
			}
		}
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

func TestMakefileIntegrationTargetIsReal(t *testing.T) {
	raw := readRepoFile(t, "Makefile")
	text := string(raw)

	if strings.Contains(text, "go test ./... -run Integration") {
		t.Fatal("Makefile integration target must not use fake Integration name filtering")
	}
	for _, needle := range []string{
		"integration-preflight:",
		"docker info >/dev/null",
		"docker compose version >/dev/null",
		"go test -count=1 -p 1 -tags=integration $(INTEGRATION_PKGS)",
		"ci: build mod-verify test-readonly test-race vet staticcheck lint integration mod-clean-check",
	} {
		if !strings.Contains(text, needle) {
			t.Fatalf("Makefile missing required integration/ci contract %q", needle)
		}
	}
}

func TestCIWorkflowRunsExplicitHealthChecks(t *testing.T) {
	text := string(readRepoFile(t, ".github/workflows/ci.yml"))
	for _, needle := range []string{
		"go mod verify",
		"go test ./...",
		"go test -mod=readonly ./...",
		"go test -race ./...",
		"go vet ./...",
		"staticcheck ./...",
		"golangci-lint run --no-config ./...",
		"git diff --exit-code -- go.mod go.sum",
		"docker info",
		"docker compose version",
		"make integration",
	} {
		if !strings.Contains(text, needle) {
			t.Fatalf("workflow missing required health command %q", needle)
		}
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

func TestReadmeRuntimeContractMentionsComposeAndGoKeys(t *testing.T) {
	readme := string(readRepoFile(t, "README.md"))
	keys := append([]string{}, composeEnvKeys(t)...)
	keys = append(keys, goRequiredEnvKeys...)
	keys = append(keys, goRestoreEnvKeys...)

	for _, key := range uniqueSorted(keys) {
		if !strings.Contains(readme, "`"+key+"`") {
			t.Fatalf("README runtime contract must mention %s", key)
		}
	}
}

func TestMariaDBContractIsConsistent(t *testing.T) {
	compose := string(readRepoFile(t, "compose.yaml"))
	if !strings.Contains(compose, "image: mariadb:${MARIADB_TAG}") {
		t.Fatal("compose.yaml must consume MARIADB_TAG for the database image")
	}

	for _, rel := range exampleEnvFiles {
		env := readEnvAssignmentsFile(t, rel)
		if env["MARIADB_TAG"] != expectedMariaDBTag {
			t.Fatalf("%s must set MARIADB_TAG=%s, got %q", rel, expectedMariaDBTag, env["MARIADB_TAG"])
		}
	}

	readme := string(readRepoFile(t, "README.md"))
	if !strings.Contains(readme, "MARIADB_TAG=11.4") {
		t.Fatal("README must document MARIADB_TAG=11.4")
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

	readme := string(readRepoFile(t, "README.md"))
	for _, needle := range []string{"`APP_BIND_ADDRESS`", "`WS_BIND_ADDRESS`", "`127.0.0.1`", "`0.0.0.0`"} {
		if !strings.Contains(readme, needle) {
			t.Fatalf("README must document bind address contract %q", needle)
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

	readme := string(readRepoFile(t, "README.md"))
	if !strings.Contains(readme, "`APP_SERVICES` must explicitly list `espocrm,espocrm-daemon,espocrm-websocket`") {
		t.Fatal("README must document the explicit websocket APP_SERVICES contract")
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

func TestTrackedFilesDoNotContainHistoricalResidue(t *testing.T) {
	files := trackedFiles(t)
	patterns := buildHistoricalResiduePatterns()

	for _, rel := range files {
		raw, err := os.ReadFile(rel)
		if err != nil {
			t.Fatalf("read %s: %v", rel, err)
		}

		scanner := bufio.NewScanner(strings.NewReader(string(raw)))
		lineNo := 0
		for scanner.Scan() {
			lineNo++
			line := scanner.Text()
			if allowedResidueLine(rel, line) {
				continue
			}
			for _, pattern := range patterns {
				if pattern.MatchString(line) {
					t.Fatalf("tracked residue match in %s:%d: %q", rel, lineNo, strings.TrimSpace(line))
				}
			}
			if containsDisallowedRecoveryTerms(line) {
				t.Fatalf("tracked residue match in %s:%d: %q", rel, lineNo, strings.TrimSpace(line))
			}
		}
		if err := scanner.Err(); err != nil {
			t.Fatalf("scan %s: %v", rel, err)
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

func listImportPaths(t *testing.T, args ...string) []string {
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

func buildHistoricalResiduePatterns() []*regexp.Regexp {
	terms := []string{
		joinParts("v", "3"),
		joinParts("le", "gacy"),
		joinParts("cut", "over"),
		joinParts("fr", "eeze"),
		joinParts("re", "tained"),
		joinParts("or", "acle"),
		joinParts("ref", "erence"),
		joinParts("gold", "en"),
		joinParts("accept", "ance"),
		joinParts("result", "bridge"),
		joinParts("contract", "/", "result"),
		joinParts("journal", "bridge"),
		joinParts("error", "transport"),
		joinParts("arch", "itecture"),
		joinParts("repo", "_", "compliance"),
		joinParts("mi", "gration"),
		joinParts("sh", "im"),
		joinParts("old", " ", "world"),
		joinParts("old", "-", "world"),
		joinParts("dual", "-", "path"),
		joinParts("dual", " ", "path"),
	}

	patterns := make([]*regexp.Regexp, 0, len(terms)+2)
	for _, term := range terms {
		patterns = append(patterns, regexp.MustCompile(`(?i)\b`+regexp.QuoteMeta(term)+`\b`))
	}
	patterns = append(patterns,
		regexp.MustCompile(`(?i)\b`+joinParts("micro")+`[_ -]?`+joinParts("monolith")+`\b`),
		regexp.MustCompile(`(?i)\b`+joinParts("repo")+`[_ -]?`+joinParts("compliance")+`\b`),
	)
	return patterns
}

func allowedResidueLine(path, line string) bool {
	lower := strings.ToLower(strings.TrimSpace(line))
	if strings.HasPrefix(path, "go.sum") {
		return strings.Contains(lower, joinParts("/", "v", "2", " ")) || strings.Contains(lower, joinParts("yaml.", "v", "3"))
	}
	if path == "Makefile" {
		return strings.Contains(lower, joinParts("golangci-lint/", "v", "2")) || strings.Contains(lower, joinParts("v", "2", ".11.4"))
	}
	return false
}

func containsDisallowedRecoveryTerms(line string) bool {
	lower := strings.ToLower(line)
	compatTerm := joinParts("compat", "ibility")
	if strings.Contains(lower, compatTerm) && !strings.Contains(lower, joinParts("no ", compatTerm)) {
		return true
	}
	altPathTerm := joinParts("fall", "back")
	if strings.Contains(lower, altPathTerm) && !strings.Contains(lower, joinParts("no ", altPathTerm)) {
		return true
	}
	return false
}

func joinParts(parts ...string) string {
	return strings.Join(parts, "")
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

func uniqueSorted(values []string) []string {
	sorted := append([]string{}, values...)
	slices.Sort(sorted)
	return slices.Compact(sorted)
}
