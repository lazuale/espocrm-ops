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
	"strings"
	"testing"
)

const modulePath = "github.com/lazuale/espocrm-ops"

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
