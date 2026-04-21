package cli

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lazuale/espocrm-ops/internal/testutil"
)

func TestProductionCLIPackageHasNoPackageVars(t *testing.T) {
	root := testutil.RepoRoot(t)
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

func TestProductionCLIEnvReadsStayAtEdgeHelpers(t *testing.T) {
	root := testutil.RepoRoot(t)
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

func TestProductionCLIExecutionBeginsOnlyInRunner(t *testing.T) {
	root := testutil.RepoRoot(t)
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
			if selector.Sel == nil || selector.Sel.Name != "Begin" {
				return true
			}
			ident, ok := selector.X.(*ast.Ident)
			if !ok || ident.Name != "operationusecase" {
				return true
			}
			if filepath.Base(path) != "runner.go" {
				t.Fatalf("production CLI operation execution must begin only in runner.go; found at %s", fset.Position(selector.Pos()))
			}
			return true
		})

		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
