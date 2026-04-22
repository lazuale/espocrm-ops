package fs

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
)

func TestFSProductionFileSetStaysBounded(t *testing.T) {
	t.Parallel()

	got := map[string]struct{}{}
	want := map[string]struct{}{
		"archive.go":        {},
		"archive_create.go": {},
		"checksum.go":       {},
		"errors.go":         {},
		"preflight.go":      {},
		"readiness.go":      {},
		"replace_tree.go":   {},
		"staging.go":        {},
	}

	for _, path := range packageSourceFiles(t) {
		got[filepath.Base(path)] = struct{}{}
	}

	if len(got) != len(want) {
		t.Fatalf("unexpected fs production file set: got %v want %v", sortedKeys(got), sortedKeys(want))
	}
	for name := range want {
		if _, ok := got[name]; !ok {
			t.Fatalf("missing fs production file %q in %v", name, sortedKeys(got))
		}
	}
	for name := range got {
		if _, ok := want[name]; !ok {
			t.Fatalf("unexpected fs production file %q in %v", name, sortedKeys(got))
		}
	}
}

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

func TestFSShellExecutionStaysInArchiveCreateGo(t *testing.T) {
	t.Parallel()

	assertFSTextOwnership(t, "exec.Command(", map[string]struct{}{
		"archive_create.go": {},
	})
}

func TestFSArchiveCreateDefinitionsStayLocal(t *testing.T) {
	t.Parallel()

	assertFSTextOwnership(t, "func CreateTarGz(", map[string]struct{}{
		"archive_create.go": {},
	})
	assertFSTextOwnership(t, "func tarCommandErrorSuffix(", map[string]struct{}{
		"archive_create.go": {},
	})
	assertFSTextOwnership(t, "func tarLastNonBlankLine(", map[string]struct{}{
		"archive_create.go": {},
	})
}

func TestFSArchiveCreateDoesNotImportInternalPackages(t *testing.T) {
	t.Parallel()

	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, filepath.Join(".", "archive_create.go"), nil, parser.ImportsOnly)
	if err != nil {
		t.Fatalf("parse archive_create.go: %v", err)
	}

	for _, imp := range file.Imports {
		resolved, err := strconv.Unquote(imp.Path.Value)
		if err != nil {
			t.Fatalf("unquote import: %v", err)
		}
		if strings.HasPrefix(resolved, "github.com/lazuale/espocrm-ops/internal/") {
			t.Fatalf("archive_create.go must not import internal packages; found %s at %s", resolved, fset.Position(imp.Pos()))
		}
	}
}

func TestFSPackageDoesNotImportInternalPackages(t *testing.T) {
	t.Parallel()

	for _, path := range packageSourceFiles(t) {
		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}

		for _, imp := range file.Imports {
			resolved, err := strconv.Unquote(imp.Path.Value)
			if err != nil {
				t.Fatalf("unquote import: %v", err)
			}
			if strings.HasPrefix(resolved, "github.com/lazuale/espocrm-ops/internal/") {
				t.Fatalf("fs package must not import internal packages; found %s in %s at %s", resolved, path, fset.Position(imp.Pos()))
			}
		}
	}
}

func TestFSDefinitionsStayExplicit(t *testing.T) {
	t.Parallel()

	assertFSTextOwnership(t, "func VerifyGzipReadable(", map[string]struct{}{
		"archive.go": {},
	})
	assertFSTextOwnership(t, "func VerifyTarGzReadable(", map[string]struct{}{
		"archive.go": {},
	})
	assertFSTextOwnership(t, "func UnpackTarGz(", map[string]struct{}{
		"archive.go": {},
	})
	assertFSTextOwnership(t, "func SHA256File(", map[string]struct{}{
		"checksum.go": {},
	})
	assertFSTextOwnership(t, "type pathStatError struct", map[string]struct{}{
		"errors.go": {},
	})
	assertFSTextOwnership(t, "func EnsureNonEmptyFile(", map[string]struct{}{
		"preflight.go": {},
	})
	assertFSTextOwnership(t, "func EnsureWritableDir(", map[string]struct{}{
		"preflight.go": {},
	})
	assertFSTextOwnership(t, "func EnsureFreeSpace(", map[string]struct{}{
		"preflight.go": {},
	})
	assertFSTextOwnership(t, "type DirReadiness struct", map[string]struct{}{
		"readiness.go": {},
	})
	assertFSTextOwnership(t, "func InspectDirReadiness(", map[string]struct{}{
		"readiness.go": {},
	})
	assertFSTextOwnership(t, "func ReplaceTree(", map[string]struct{}{
		"replace_tree.go": {},
	})
	assertFSTextOwnership(t, "type Stage struct", map[string]struct{}{
		"staging.go": {},
	})
	assertFSTextOwnership(t, "func NewSiblingStage(", map[string]struct{}{
		"staging.go": {},
	})
	assertFSTextOwnership(t, "func PreparedTreeRoot(", map[string]struct{}{
		"staging.go": {},
	})
	assertFSTextOwnership(t, "func PreparedTreeRootExact(", map[string]struct{}{
		"staging.go": {},
	})
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

func assertFSTextOwnership(t *testing.T, needle string, owners map[string]struct{}) {
	t.Helper()

	files := packageSourceFiles(t)
	for _, path := range files {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		if !strings.Contains(string(raw), needle) {
			continue
		}
		if _, ok := owners[filepath.Base(path)]; !ok {
			t.Fatalf("%s must stay owner-local to %v; found in %s", needle, sortedKeys(owners), path)
		}
	}
}
