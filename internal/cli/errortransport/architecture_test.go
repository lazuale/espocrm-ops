package errortransport

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

func TestErrorTransportImportsStayExplicit(t *testing.T) {
	root := testutil.RepoRoot(t)
	dir := filepath.Join(root, "internal", "cli", "errortransport")
	fset := token.NewFileSet()
	allowedInternal := map[string]struct{}{
		modulePath + "/internal/contract/apperr":   {},
		modulePath + "/internal/contract/exitcode": {},
		modulePath + "/internal/contract/result":   {},
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
				t.Fatalf("error transport imports unexpected internal package %s at %s", resolved, fset.Position(imp.Pos()))
			}
		}

		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestErrorTransportExportedSurfaceStaysIntentional(t *testing.T) {
	root := testutil.RepoRoot(t)
	dir := filepath.Join(root, "internal", "cli", "errortransport")
	fset := token.NewFileSet()
	got := map[string]struct{}{}
	want := map[string]struct{}{
		"type CodeError":                       {},
		"type ResultCodeError":                 {},
		"func UsageError":                      {},
		"func CodeForError":                    {},
		"func ErrorCodeForError":               {},
		"func ErrorKindForError":               {},
		"func ErrorResult":                     {},
		"func WarningMessages":                 {},
		"func IsUsageError":                    {},
		"func Silent":                          {},
		"method CodeError.Error":               {},
		"method CodeError.Unwrap":              {},
		"method CodeError.ExitCode":            {},
		"method CodeError.ErrorCode":           {},
		"method CodeError.WarningMessages":     {},
		"method ResultCodeError.CommandResult": {},
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
					typeSpec, ok := spec.(*ast.TypeSpec)
					if ok && typeSpec.Name.IsExported() {
						got["type "+typeSpec.Name.Name] = struct{}{}
					}
				}
			case *ast.FuncDecl:
				if typed.Recv == nil {
					if typed.Name.IsExported() {
						got["func "+typed.Name.Name] = struct{}{}
					}
					continue
				}
				recv := errorTransportReceiverName(typed)
				if recv == "" || !typed.Name.IsExported() || !ast.IsExported(recv) {
					continue
				}
				got["method "+recv+"."+typed.Name.Name] = struct{}{}
			}
		}

		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	if len(got) != len(want) {
		t.Fatalf("unexpected exported error transport surface: got %v want %v", errorTransportSortedKeys(got), errorTransportSortedKeys(want))
	}
	for name := range want {
		if _, ok := got[name]; !ok {
			t.Fatalf("missing exported error transport symbol %q in %v", name, errorTransportSortedKeys(got))
		}
	}
	for name := range got {
		if _, ok := want[name]; !ok {
			t.Fatalf("unexpected exported error transport symbol %q in %v", name, errorTransportSortedKeys(got))
		}
	}
}

func errorTransportReceiverName(fn *ast.FuncDecl) string {
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

func errorTransportSortedKeys(set map[string]struct{}) []string {
	keys := make([]string, 0, len(set))
	for key := range set {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
