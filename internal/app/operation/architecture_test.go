package operation

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

func TestOperationLifecycleRootFileSetStaysResidual(t *testing.T) {
	root := testutil.RepoRoot(t)
	paths, err := filepath.Glob(filepath.Join(root, "internal", "app", "operation", "*.go"))
	if err != nil {
		t.Fatal(err)
	}

	got := map[string]struct{}{}
	want := map[string]struct{}{
		"context_runtime_dirs.go": {},
		"operation_context.go":    {},
	}

	for _, path := range paths {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		got[filepath.Base(path)] = struct{}{}
	}

	if len(got) != len(want) {
		t.Fatalf("unexpected operation lifecycle production file set: got %v want %v", operationSortedKeys(got), operationSortedKeys(want))
	}
	for name := range want {
		if _, ok := got[name]; !ok {
			t.Fatalf("missing operation lifecycle production file %q in %v", name, operationSortedKeys(got))
		}
	}
	for name := range got {
		if _, ok := want[name]; !ok {
			t.Fatalf("unexpected operation lifecycle production file %q in %v", name, operationSortedKeys(got))
		}
	}
}

func TestOperationAppDoesNotOwnPlatformJournalStoreWiring(t *testing.T) {
	root := testutil.RepoRoot(t)
	dir := filepath.Join(root, "internal", "app", "operation")
	fset := token.NewFileSet()

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
			importPath, err := strconv.Unquote(imp.Path.Value)
			if err != nil {
				return err
			}
			if importPath == modulePath+"/internal/platform/journalstore" {
				t.Fatalf("internal/app/operation imports forbidden package %s at %s", importPath, fset.Position(imp.Pos()))
			}
		}

		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestOperationLifecycleImportSurfaceStaysExplicit(t *testing.T) {
	assertOperationImportOwnership(t, modulePath+"/internal/app/ports/envport", map[string]struct{}{
		"operation_context.go": {},
	})
	assertOperationImportOwnership(t, modulePath+"/internal/app/ports/filesport", map[string]struct{}{
		"operation_context.go": {},
	})
	assertOperationImportOwnership(t, modulePath+"/internal/app/ports/lockport", map[string]struct{}{
		"operation_context.go": {},
	})
	assertOperationImportOwnership(t, modulePath+"/internal/domain/env", map[string]struct{}{
		"context_runtime_dirs.go": {},
		"operation_context.go":    {},
	})
	assertOperationImportOwnership(t, modulePath+"/internal/domain/failure", map[string]struct{}{
		"operation_context.go": {},
	})
}

func TestOperationLifecycleRootImportsStayExact(t *testing.T) {
	assertOperationFileInternalImportsExactly(t, "operation_context.go", map[string]struct{}{
		modulePath + "/internal/app/ports/envport":   {},
		modulePath + "/internal/app/ports/filesport": {},
		modulePath + "/internal/app/ports/lockport":  {},
		modulePath + "/internal/domain/env":          {},
		modulePath + "/internal/domain/failure":      {},
	})
	assertOperationFileInternalImportsExactly(t, "context_runtime_dirs.go", map[string]struct{}{
		modulePath + "/internal/domain/env": {},
	})
}

func TestOperationLifecycleDoesNotImportJournalOrTraceBridge(t *testing.T) {
	assertOperationImportAbsent(t, modulePath+"/internal/domain/journal")
	assertOperationImportAbsent(t, modulePath+"/internal/app/operationtrace")
}

func TestOperationLifecycleExportedSurfaceStaysIntentional(t *testing.T) {
	root := testutil.RepoRoot(t)
	dir := filepath.Join(root, "internal", "app", "operation")
	fset := token.NewFileSet()
	got := map[string]struct{}{}
	want := map[string]struct{}{
		"type OperationContextRequest":    {},
		"type Dependencies":               {},
		"type Service":                    {},
		"type OperationContext":           {},
		"func NewService":                 {},
		"method Service.PrepareOperation": {},
		"method OperationContext.Release": {},
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
				recv := operationReceiverName(typed)
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
		t.Fatalf("unexpected operation lifecycle exported surface: got %v want %v", operationSortedKeys(got), operationSortedKeys(want))
	}
	for name := range want {
		if _, ok := got[name]; !ok {
			t.Fatalf("missing exported operation lifecycle symbol %q in %v", name, operationSortedKeys(got))
		}
	}
	for name := range got {
		if _, ok := want[name]; !ok {
			t.Fatalf("unexpected exported operation lifecycle symbol %q in %v", name, operationSortedKeys(got))
		}
	}
}

func TestOperationLifecycleDefinitionsStayExplicit(t *testing.T) {
	assertOperationTextOwnership(t, "func (s Service) PrepareOperation(", map[string]struct{}{
		"operation_context.go": {},
	})
	assertOperationTextOwnership(t, "func (s Service) verifyRuntimePaths(", map[string]struct{}{
		"context_runtime_dirs.go": {},
	})
	assertOperationTextOwnership(t, "func classifyOperationEnvError(", map[string]struct{}{
		"operation_context.go": {},
	})
	assertOperationTextOwnership(t, "func classifyOperationLockError(", map[string]struct{}{
		"operation_context.go": {},
	})
}

func assertOperationImportOwnership(t *testing.T, importPath string, owners map[string]struct{}) {
	t.Helper()

	root := testutil.RepoRoot(t)
	dir := filepath.Join(root, "internal", "app", "operation")
	fset := token.NewFileSet()

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
			if resolved != importPath {
				continue
			}
			if _, ok := owners[filepath.Base(path)]; !ok {
				t.Fatalf("%s import must stay owner-local to %v; found in %s", importPath, operationOwnerNames(owners), path)
			}
		}

		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func assertOperationImportAbsent(t *testing.T, importPath string) {
	t.Helper()

	root := testutil.RepoRoot(t)
	dir := filepath.Join(root, "internal", "app", "operation")
	fset := token.NewFileSet()

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
			if resolved == importPath {
				t.Fatalf("operation lifecycle package must not import %s; found in %s at %s", importPath, path, fset.Position(imp.Pos()))
			}
		}

		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func assertOperationFileInternalImportsExactly(t *testing.T, fileName string, allowed map[string]struct{}) {
	t.Helper()

	root := testutil.RepoRoot(t)
	path := filepath.Join(root, "internal", "app", "operation", fileName)
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
	if err != nil {
		t.Fatal(err)
	}

	got := map[string]struct{}{}
	for _, imp := range file.Imports {
		resolved, err := strconv.Unquote(imp.Path.Value)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.HasPrefix(resolved, modulePath+"/internal/") {
			continue
		}
		got[resolved] = struct{}{}
	}

	if len(got) != len(allowed) {
		t.Fatalf("%s imports unexpected internal surface: got %v want %v", fileName, operationSortedKeys(got), operationSortedKeys(allowed))
	}
	for name := range allowed {
		if _, ok := got[name]; !ok {
			t.Fatalf("%s missing internal import %q in %v", fileName, name, operationSortedKeys(got))
		}
	}
	for name := range got {
		if _, ok := allowed[name]; !ok {
			t.Fatalf("%s imports unexpected internal package %q in %v", fileName, name, operationSortedKeys(got))
		}
	}
}

func assertOperationTextOwnership(t *testing.T, needle string, owners map[string]struct{}) {
	t.Helper()

	root := testutil.RepoRoot(t)
	dir := filepath.Join(root, "internal", "app", "operation")

	err := filepath.WalkDir(dir, func(path string, entry os.DirEntry, err error) error {
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
		if !strings.Contains(string(raw), needle) {
			return nil
		}
		if _, ok := owners[filepath.Base(path)]; !ok {
			t.Fatalf("%s must stay owner-local to %v; found in %s", needle, operationOwnerNames(owners), path)
		}

		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func operationOwnerNames(owners map[string]struct{}) []string {
	names := make([]string, 0, len(owners))
	for name := range owners {
		names = append(names, name)
	}
	return names
}

func operationReceiverName(fn *ast.FuncDecl) string {
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

func operationSortedKeys(set map[string]struct{}) []string {
	keys := make([]string, 0, len(set))
	for key := range set {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
