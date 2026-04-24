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

func repoRoot(t *testing.T) string {
	t.Helper()

	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git rev-parse --show-toplevel: %v\n%s", err, output)
	}

	return strings.TrimSpace(string(output))
}
