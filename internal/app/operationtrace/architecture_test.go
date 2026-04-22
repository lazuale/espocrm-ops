package operationtrace

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/lazuale/espocrm-ops/internal/testutil"
)

const modulePath = "github.com/lazuale/espocrm-ops"

func TestOperationTraceImportsStayExplicit(t *testing.T) {
	root := testutil.RepoRoot(t)
	dir := filepath.Join(root, "internal", "app", "operationtrace")
	fset := token.NewFileSet()
	allowedInternal := map[string]struct{}{
		modulePath + "/internal/domain/journal": {},
	}

	err := filepath.WalkDir(dir, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}

		file, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		for _, imp := range file.Imports {
			resolved, err := strconv.Unquote(imp.Path.Value)
			if err != nil {
				return err
			}
			if !strings.HasPrefix(resolved, modulePath+"/internal/") {
				continue
			}
			if _, ok := allowedInternal[resolved]; !ok {
				t.Fatalf("operation trace package imports unexpected internal package %s at %s", resolved, fset.Position(imp.Pos()))
			}
		}

		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestOperationTraceExportedSurfaceStaysIntentional(t *testing.T) {
	root := testutil.RepoRoot(t)
	dir := filepath.Join(root, "internal", "app", "operationtrace")
	fset := token.NewFileSet()
	got := map[string]struct{}{}
	want := map[string]struct{}{
		"const TimeFormat":              {},
		"type Runtime":                  {},
		"type Writer":                   {},
		"var ErrJournalWriterDisabled":  {},
		"type DisabledWriter":           {},
		"type JournalPayload":           {},
		"type JournalRecord":            {},
		"type Completion":               {},
		"type Execution":                {},
		"func Begin":                    {},
		"type DefaultRuntime":           {},
		"func NewOperationID":           {},
		"method DisabledWriter.Write":   {},
		"method Execution.FinishSuccess": {},
		"method Execution.FinishFailure": {},
		"method DefaultRuntime.Now":     {},
		"method DefaultRuntime.NewOperationID": {},
	}

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
		for _, decl := range file.Decls {
			switch typed := decl.(type) {
			case *ast.GenDecl:
				for _, spec := range typed.Specs {
					switch spec := spec.(type) {
					case *ast.TypeSpec:
						if spec.Name.IsExported() {
							got["type "+spec.Name.Name] = struct{}{}
						}
					case *ast.ValueSpec:
						for _, name := range spec.Names {
							if !name.IsExported() {
								continue
							}
							switch typed.Tok {
							case token.CONST:
								got["const "+name.Name] = struct{}{}
							case token.VAR:
								got["var "+name.Name] = struct{}{}
							}
						}
					}
				}
			case *ast.FuncDecl:
				if typed.Recv == nil {
					if typed.Name.IsExported() {
						got["func "+typed.Name.Name] = struct{}{}
					}
					continue
				}
				recv := operationTraceReceiverName(typed)
				if recv == "" {
					continue
				}
				if typed.Name.IsExported() {
					got["method "+recv+"."+typed.Name.Name] = struct{}{}
				}
			}
		}

		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	if len(got) != len(want) {
		t.Fatalf("unexpected exported operation trace surface: got %v want %v", operationTraceSortedKeys(got), operationTraceSortedKeys(want))
	}
	for name := range want {
		if _, ok := got[name]; !ok {
			t.Fatalf("missing exported operation trace symbol %q in %v", name, operationTraceSortedKeys(got))
		}
	}
	for name := range got {
		if _, ok := want[name]; !ok {
			t.Fatalf("unexpected exported operation trace symbol %q in %v", name, operationTraceSortedKeys(got))
		}
	}
}

func operationTraceReceiverName(fn *ast.FuncDecl) string {
	if fn.Recv == nil || len(fn.Recv.List) == 0 {
		return ""
	}

	switch expr := fn.Recv.List[0].Type.(type) {
	case *ast.Ident:
		return expr.Name
	case *ast.StarExpr:
		if ident, ok := expr.X.(*ast.Ident); ok {
			return ident.Name
		}
	}

	return ""
}

func operationTraceSortedKeys(set map[string]struct{}) []string {
	keys := make([]string, 0, len(set))
	for key := range set {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
