package repository_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

const modulePath = "github.com/lazuale/espocrm-ops"

func TestRetainedInternalPackagesAreV3Only(t *testing.T) {
	got := listImportPaths(t, "list", "-f", "{{.ImportPath}}", "./internal/...")
	want := []string{
		modulePath + "/internal/v3/cli",
		modulePath + "/internal/v3/config",
		modulePath + "/internal/v3/manifest",
		modulePath + "/internal/v3/ops",
		modulePath + "/internal/v3/runtime",
	}

	if !slices.Equal(got, want) {
		t.Fatalf("unexpected internal packages:\n got: %v\nwant: %v", got, want)
	}
}

func TestLegacyInternalDirectoriesAreGone(t *testing.T) {
	root := repoRoot(t)
	legacyDirs := []string{
		"internal/app",
		"internal/cli",
		"internal/contract",
		"internal/domain",
		"internal/manifest",
		"internal/model",
		"internal/ops",
		"internal/opsconfig",
		"internal/platform",
		"internal/runtime",
		"internal/store",
		"internal/testutil",
	}

	for _, rel := range legacyDirs {
		path := filepath.Join(root, rel)
		_, err := os.Stat(path)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			t.Fatalf("stat %s: %v", path, err)
		}
		t.Fatalf("legacy directory still exists: %s", path)
	}
}

func TestCommandDoesNotPullLegacyInternalPackages(t *testing.T) {
	deps := listImportPaths(t, "list", "-deps", "-f", "{{.ImportPath}}", "./cmd/espops")
	var gotLegacy []string
	for _, dep := range deps {
		switch {
		case dep == modulePath+"/internal/v3/cli":
		case dep == modulePath+"/internal/v3/config":
		case dep == modulePath+"/internal/v3/manifest":
		case dep == modulePath+"/internal/v3/ops":
		case dep == modulePath+"/internal/v3/runtime":
		case strings.HasPrefix(dep, modulePath+"/internal/"):
			gotLegacy = append(gotLegacy, dep)
		}
	}

	if len(gotLegacy) > 0 {
		t.Fatalf("cmd/espops still pulls legacy internal packages: %v", gotLegacy)
	}
}

func TestProductionProcessEnvAccessSurfaceIsExplicit(t *testing.T) {
	root := repoRoot(t)
	fset := token.NewFileSet()
	allowedEnvironOwner := filepath.Join(root, "internal", "v3", "runtime", "docker.go")

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
	allowedExecOwner := filepath.Join(root, "internal", "v3", "runtime", "docker.go")

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

func TestAuthorityDocsDoNotDescribeRemovedLayout(t *testing.T) {
	root := repoRoot(t)
	docs := []string{
		"README.md",
		"CONTRIBUTING.md",
		"ARCHITECTURE.md",
		"MICRO_MONOLITHS.md",
		"REPO_COMPLIANCE_CHECKLIST.md",
		"REPO_COMPLIANCE_BASELINE.md",
	}
	forbidden := []string{
		"internal/app/",
		"internal/cli/",
		"internal/contract/",
		"internal/domain/",
		"internal/platform/",
		"internal/model/",
		"internal/opsconfig/",
		"internal/runtime/",
		"internal/store/",
		"acceptance/v2",
		"V2_SCOPE",
	}

	for _, rel := range docs {
		path := filepath.Join(root, rel)
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}

		text := string(raw)
		for _, snippet := range forbidden {
			if strings.Contains(text, snippet) {
				t.Fatalf("doc %s still contains removed layout reference %q", rel, snippet)
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
