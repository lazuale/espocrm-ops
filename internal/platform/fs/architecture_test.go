package fs

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

func TestFSExportedSurfaceIsIntentional(t *testing.T) {
	t.Parallel()

	files := packageSourceFiles(t)
	exported := map[string]struct{}{}
	fset := token.NewFileSet()

	for _, path := range files {
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}

		for _, decl := range file.Decls {
			switch d := decl.(type) {
			case *ast.FuncDecl:
				if d.Recv == nil && ast.IsExported(d.Name.Name) {
					exported[d.Name.Name] = struct{}{}
				}
			case *ast.GenDecl:
				for _, spec := range d.Specs {
					switch s := spec.(type) {
					case *ast.TypeSpec:
						if ast.IsExported(s.Name.Name) {
							exported[s.Name.Name] = struct{}{}
						}
					case *ast.ValueSpec:
						for _, name := range s.Names {
							if ast.IsExported(name.Name) {
								exported[name.Name] = struct{}{}
							}
						}
					}
				}
			}
		}
	}

	expected := []string{
		"CreateTarGz",
		"DirReadiness",
		"EnsureFreeSpace",
		"EnsureNonEmptyFile",
		"EnsureWritableDir",
		"InspectDirReadiness",
		"NewSiblingStage",
		"PreparedTreeRoot",
		"PreparedTreeRootExact",
		"ReplaceTree",
		"SHA256File",
		"Stage",
		"UnpackTarGz",
		"VerifyGzipReadable",
		"VerifyTarGzReadable",
	}

	assertExactNames(t, "exported surface", exported, expected)
}

func TestFSPackageDoesNotOwnErrorCodes(t *testing.T) {
	t.Parallel()

	files := packageSourceFiles(t)
	methods := map[string]struct{}{}
	fset := token.NewFileSet()

	for _, path := range files {
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}

		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Recv == nil || fn.Name.Name != "ErrorCode" {
				continue
			}
			methods[receiverName(fn)] = struct{}{}
		}
	}

	if len(methods) != 0 {
		t.Fatalf("fs package must not define ErrorCode methods, found: %v", sortedKeys(methods))
	}
}

func packageSourceFiles(t *testing.T) []string {
	t.Helper()

	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read package dir: %v", err)
	}

	var files []string
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		files = append(files, filepath.Join(".", name))
	}
	sort.Strings(files)
	return files
}

func receiverName(fn *ast.FuncDecl) string {
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

func assertExactNames(t *testing.T, label string, got map[string]struct{}, want []string) {
	t.Helper()

	wantSet := map[string]struct{}{}
	for _, name := range want {
		wantSet[name] = struct{}{}
	}

	if len(got) != len(wantSet) {
		t.Fatalf("unexpected %s count: got %v want %v", label, sortedKeys(got), want)
	}

	for name := range wantSet {
		if _, ok := got[name]; !ok {
			t.Fatalf("missing %s entry %q in %v", label, name, sortedKeys(got))
		}
	}

	for name := range got {
		if _, ok := wantSet[name]; !ok {
			t.Fatalf("unexpected %s entry %q in %v", label, name, sortedKeys(got))
		}
	}
}

func sortedKeys(set map[string]struct{}) []string {
	keys := make([]string, 0, len(set))
	for key := range set {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
