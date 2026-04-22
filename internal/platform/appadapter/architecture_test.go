package appadapter

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

func TestAppAdapterProductionFileSetStaysResidual(t *testing.T) {
	root := testutil.RepoRoot(t)
	dir := filepath.Join(root, "internal", "platform", "appadapter")
	got := map[string]struct{}{}
	want := map[string]struct{}{
		"files.go": {},
		"locks.go": {},
	}

	err := filepath.WalkDir(dir, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		got[filepath.Base(path)] = struct{}{}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	if len(got) != len(want) {
		t.Fatalf("unexpected appadapter production file set: got %v want %v", appAdapterSortedKeys(got), appAdapterSortedKeys(want))
	}
	for name := range want {
		if _, ok := got[name]; !ok {
			t.Fatalf("missing residual appadapter file %q in %v", name, appAdapterSortedKeys(got))
		}
	}
	for name := range got {
		if _, ok := want[name]; !ok {
			t.Fatalf("unexpected appadapter production file %q in %v", name, appAdapterSortedKeys(got))
		}
	}
}

func TestAppAdapterImportsStayExplicit(t *testing.T) {
	root := testutil.RepoRoot(t)
	dir := filepath.Join(root, "internal", "platform", "appadapter")
	fset := token.NewFileSet()
	allowedInternalByFile := map[string]map[string]struct{}{
		"files.go": {
			modulePath + "/internal/app/ports/filesport": {},
			modulePath + "/internal/platform/fs":         {},
		},
		"locks.go": {
			modulePath + "/internal/app/ports/lockport": {},
			modulePath + "/internal/platform/locks":     {},
		},
	}

	err := filepath.WalkDir(dir, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}

		fileName := filepath.Base(path)
		allowedInternal, ok := allowedInternalByFile[fileName]
		if !ok {
			t.Fatalf("unexpected appadapter production file %s", fileName)
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
				t.Fatalf("appadapter file %s imports unexpected internal package %s at %s", fileName, resolved, fset.Position(imp.Pos()))
			}
		}

		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestAppAdapterExportedSurfaceStaysIntentional(t *testing.T) {
	root := testutil.RepoRoot(t)
	dir := filepath.Join(root, "internal", "platform", "appadapter")
	fset := token.NewFileSet()
	got := map[string]struct{}{}
	want := map[string]struct{}{
		"type Files":                                 {},
		"type Locks":                                 {},
		"method Files.CreateTarGz":                   {},
		"method Files.SHA256File":                    {},
		"method Files.InspectDirReadiness":           {},
		"method Files.EnsureNonEmptyFile":            {},
		"method Files.EnsureWritableDir":             {},
		"method Files.EnsureFreeSpace":               {},
		"method Files.NewSiblingStage":               {},
		"method Files.UnpackTarGz":                   {},
		"method Files.PreparedTreeRoot":              {},
		"method Files.PreparedTreeRootExact":         {},
		"method Files.ReplaceTree":                   {},
		"method Locks.AcquireSharedOperationLock":    {},
		"method Locks.AcquireMaintenanceLock":        {},
		"method Locks.AcquireRestoreDBLock":          {},
		"method Locks.AcquireRestoreFilesLock":       {},
		"method Locks.CheckSharedOperationReadiness": {},
		"method Locks.CheckMaintenanceReadiness":     {},
		"method Locks.CheckRestoreDBReadiness":       {},
		"method Locks.CheckRestoreFilesReadiness":    {},
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
					continue
				}
				recv := appAdapterReceiverName(typed)
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
		t.Fatalf("unexpected exported appadapter surface: got %v want %v", appAdapterSortedKeys(got), appAdapterSortedKeys(want))
	}
	for name := range want {
		if _, ok := got[name]; !ok {
			t.Fatalf("missing exported appadapter symbol %q in %v", name, appAdapterSortedKeys(got))
		}
	}
	for name := range got {
		if _, ok := want[name]; !ok {
			t.Fatalf("unexpected exported appadapter symbol %q in %v", name, appAdapterSortedKeys(got))
		}
	}
}

func TestAppAdapterBridgeDefinitionsStayLocal(t *testing.T) {
	assertAppAdapterTextOwnership(t, "type stageAdapter struct", map[string]struct{}{
		"files.go": {},
	})
	assertAppAdapterTextOwnership(t, "func adaptLockReadiness(", map[string]struct{}{
		"locks.go": {},
	})
}

func assertAppAdapterTextOwnership(t *testing.T, needle string, owners map[string]struct{}) {
	t.Helper()

	root := testutil.RepoRoot(t)
	dir := filepath.Join(root, "internal", "platform", "appadapter")

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
			t.Fatalf("%s must stay owner-local to %v; found in %s", needle, appAdapterSortedKeys(owners), path)
		}

		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func appAdapterReceiverName(fn *ast.FuncDecl) string {
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

func appAdapterSortedKeys(set map[string]struct{}) []string {
	keys := make([]string, 0, len(set))
	for key := range set {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
