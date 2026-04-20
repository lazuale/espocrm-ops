package repository_test

import (
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const modulePath = "github.com/lazuale/espocrm-ops"

type listedPackage struct {
	ImportPath string
	Imports    []string
}

func TestInternalDependencyBoundaries(t *testing.T) {
	packages := listInternalPackages(t)

	for _, pkg := range packages {
		switch {
		case inLayer(pkg.ImportPath, "contract"):
			assertNoImports(t, pkg, []string{
				modulePath + "/internal/",
			})
		case inLayer(pkg.ImportPath, "cli"):
			assertNoImports(t, pkg, []string{
				modulePath + "/internal/domain",
				modulePath + "/internal/platform",
			})
		case inLayer(pkg.ImportPath, "usecase"):
			assertNoImports(t, pkg, []string{
				modulePath + "/internal/cli",
				modulePath + "/internal/contract/exitcode",
			})
		case inLayer(pkg.ImportPath, "domain"):
			assertNoImports(t, pkg, []string{
				modulePath + "/internal/",
			})
			assertStdlibOnly(t, pkg)
		case inLayer(pkg.ImportPath, "platform"):
			assertNoImports(t, pkg, []string{
				modulePath + "/internal/cli",
				modulePath + "/internal/contract",
				modulePath + "/internal/usecase",
			})
		}
	}
}

func TestNoProductionTypeAliases(t *testing.T) {
	root := repoRoot(t)
	internalDir := filepath.Join(root, "internal")

	err := filepath.WalkDir(internalDir, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}

		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for lineNo, line := range strings.Split(string(raw), "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "type ") && strings.Contains(line, " = ") {
				t.Fatalf("production type alias found at %s:%d: %s", path, lineNo+1, line)
			}
		}

		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestProductionCLIPackageHasNoPackageVars(t *testing.T) {
	root := repoRoot(t)
	cliDir := filepath.Join(root, "internal", "cli")
	fset := token.NewFileSet()

	err := filepath.WalkDir(cliDir, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}

		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			return err
		}

		for _, decl := range file.Decls {
			gen, ok := decl.(*ast.GenDecl)
			if !ok || gen.Tok != token.VAR {
				continue
			}

			var names []string
			for _, spec := range gen.Specs {
				valueSpec, ok := spec.(*ast.ValueSpec)
				if !ok {
					continue
				}
				for _, name := range valueSpec.Names {
					names = append(names, name.Name)
				}
			}

			t.Fatalf("production package-level var in internal/cli at %s: %s", fset.Position(gen.Pos()), strings.Join(names, ", "))
		}

		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestUsecaseAndDomainDoNotReadProcessEnv(t *testing.T) {
	root := repoRoot(t)
	fset := token.NewFileSet()

	for _, dir := range []string{
		filepath.Join(root, "internal", "usecase"),
		filepath.Join(root, "internal", "domain"),
	} {
		err := filepath.WalkDir(dir, func(path string, entry os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}

			file, err := parser.ParseFile(fset, path, nil, 0)
			if err != nil {
				return err
			}

			ast.Inspect(file, func(node ast.Node) bool {
				selector, ok := node.(*ast.SelectorExpr)
				if !ok {
					return true
				}
				switch selector.Sel.Name {
				case "Getenv", "LookupEnv", "Environ":
				default:
					return true
				}
				ident, ok := selector.X.(*ast.Ident)
				if ok && ident.Name == "os" {
					t.Fatalf("process env read in backend policy layer at %s", fset.Position(selector.Pos()))
				}
				return true
			})

			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
}

func TestProductionCLIEnvReadsStayAtEdgeHelpers(t *testing.T) {
	root := repoRoot(t)
	cliDir := filepath.Join(root, "internal", "cli")
	fset := token.NewFileSet()

	err := filepath.WalkDir(cliDir, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}

		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			return err
		}

		ast.Inspect(file, func(node ast.Node) bool {
			selector, ok := node.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			switch selector.Sel.Name {
			case "Getenv", "LookupEnv", "Environ":
			default:
				return true
			}
			ident, ok := selector.X.(*ast.Ident)
			if ok && ident.Name == "os" {
				t.Fatalf("production CLI env read must stay in env helpers at %s", fset.Position(selector.Pos()))
			}
			return true
		})

		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestDockerMySQLAdapterDoesNotUseProcessStdIOOrWholeEnv(t *testing.T) {
	root := repoRoot(t)
	path := filepath.Join(root, "internal", "platform", "docker", "mysql.go")

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)

	for _, forbidden := range []string{"os.Stdout", "os.Stderr", "os.Environ("} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("docker mysql adapter must not use %s directly", forbidden)
		}
	}
}

func TestOperationUsecaseDoesNotOwnPlatformJournalWiring(t *testing.T) {
	pkg := listPackage(t, "./internal/usecase/operation")

	assertNoImports(t, pkg, []string{
		modulePath + "/internal/platform/",
	})
}

func listInternalPackages(t *testing.T) []listedPackage {
	t.Helper()

	return listPackages(t, "./internal/...")
}

func listPackage(t *testing.T, pattern string) listedPackage {
	t.Helper()

	packages := listPackages(t, pattern)
	if len(packages) != 1 {
		t.Fatalf("expected exactly one package for %s, got %d", pattern, len(packages))
	}

	return packages[0]
}

func listPackages(t *testing.T, pattern string) []listedPackage {
	t.Helper()

	root := repoRoot(t)
	goBin, err := exec.LookPath("go")
	if err != nil {
		t.Fatalf("locate go binary: %v", err)
	}
	cmd := exec.Command(goBin, "list", "-json", pattern)
	cmd.Dir = root

	out, err := cmd.Output()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			t.Fatalf("go list failed: %v\n%s", err, ee.Stderr)
		}
		t.Fatalf("go list failed: %v", err)
	}

	dec := json.NewDecoder(strings.NewReader(string(out)))
	var packages []listedPackage
	for {
		var pkg listedPackage
		if err := dec.Decode(&pkg); err != nil {
			if err == io.EOF {
				break
			}
			t.Fatalf("decode go list output: %v", err)
		}
		packages = append(packages, pkg)
	}

	return packages
}

func repoRoot(t *testing.T) string {
	t.Helper()

	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.mod not found")
		}
		dir = parent
	}
}

func inLayer(importPath, layer string) bool {
	return importPath == modulePath+"/internal/"+layer ||
		strings.HasPrefix(importPath, modulePath+"/internal/"+layer+"/")
}

func assertNoImports(t *testing.T, pkg listedPackage, forbiddenPrefixes []string) {
	t.Helper()

	for _, imp := range pkg.Imports {
		for _, forbidden := range forbiddenPrefixes {
			if strings.HasPrefix(imp, forbidden) {
				t.Fatalf("%s imports forbidden package %s", pkg.ImportPath, imp)
			}
		}
	}
}

func assertStdlibOnly(t *testing.T, pkg listedPackage) {
	t.Helper()

	for _, imp := range pkg.Imports {
		if strings.Contains(imp, ".") {
			t.Fatalf("%s imports non-stdlib package %s", pkg.ImportPath, imp)
		}
	}
}
