package journalstore

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

const modulePath = "github.com/lazuale/espocrm-ops"

func TestJournalStoreProductionFileSetStaysBounded(t *testing.T) {
	got := map[string]struct{}{}
	want := map[string]struct{}{
		"fs.go": {},
	}

	for _, path := range journalStoreFiles(t) {
		got[filepath.Base(path)] = struct{}{}
	}

	if len(got) != len(want) {
		t.Fatalf("unexpected journalstore production file set: got %v want %v", sortedJournalStoreKeys(got), sortedJournalStoreKeys(want))
	}
	for name := range want {
		if _, ok := got[name]; !ok {
			t.Fatalf("missing journalstore production file %q in %v", name, sortedJournalStoreKeys(got))
		}
	}
	for name := range got {
		if _, ok := want[name]; !ok {
			t.Fatalf("unexpected journalstore production file %q in %v", name, sortedJournalStoreKeys(got))
		}
	}
}

func TestJournalStoreExportedSurfaceIsIntentional(t *testing.T) {
	files := journalStoreFiles(t)
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
					if ts, ok := spec.(*ast.TypeSpec); ok && ast.IsExported(ts.Name.Name) {
						exported[ts.Name.Name] = struct{}{}
					}
				}
			}
		}
	}

	want := []string{"FSWriter"}
	assertExactJournalStoreNames(t, "exported surface", exported, want)
}

func TestJournalStoreImportsStayNarrow(t *testing.T) {
	files := journalStoreFiles(t)
	fset := token.NewFileSet()

	for _, path := range files {
		file, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		for _, imp := range file.Imports {
			importPath := strings.Trim(imp.Path.Value, `"`)
			switch {
			case !strings.Contains(importPath, "."):
			case importPath == modulePath+"/internal/domain/journal":
			default:
				t.Fatalf("journalstore imports forbidden package %s in %s", importPath, path)
			}
		}
	}
}

func TestJournalStoreDoesNotOwnErrorCodes(t *testing.T) {
	files := journalStoreFiles(t)
	fset := token.NewFileSet()
	methods := map[string]struct{}{}

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
			methods[journalStoreReceiverName(fn)] = struct{}{}
		}
	}

	if len(methods) != 0 {
		t.Fatalf("journalstore must not define ErrorCode methods, found: %v", sortedJournalStoreKeys(methods))
	}
}

func TestJournalStoreDoesNotReadProcessEnvOrShellOut(t *testing.T) {
	for _, needle := range []string{
		"os.Getenv(",
		"os.LookupEnv(",
		"os.Environ(",
		"os.Getwd(",
		"exec.Command(",
	} {
		assertJournalStoreTextAbsent(t, needle)
	}
}

func TestJournalStoreDefinitionsStayExplicit(t *testing.T) {
	assertJournalStoreTextOwnership(t, "type FSWriter struct", map[string]struct{}{
		"fs.go": {},
	})
	assertJournalStoreTextOwnership(t, "func (w FSWriter) Write(", map[string]struct{}{
		"fs.go": {},
	})
}

func journalStoreFiles(t *testing.T) []string {
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

func assertJournalStoreTextAbsent(t *testing.T, needle string) {
	t.Helper()

	for _, path := range journalStoreFiles(t) {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		if strings.Contains(string(raw), needle) {
			t.Fatalf("journalstore must not contain %s; found in %s", needle, path)
		}
	}
}

func assertJournalStoreTextOwnership(t *testing.T, needle string, owners map[string]struct{}) {
	t.Helper()

	for _, path := range journalStoreFiles(t) {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		if !strings.Contains(string(raw), needle) {
			continue
		}
		if _, ok := owners[filepath.Base(path)]; !ok {
			t.Fatalf("%s must stay owner-local to %v; found in %s", needle, sortedJournalStoreKeys(owners), path)
		}
	}
}

func journalStoreReceiverName(fn *ast.FuncDecl) string {
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

func assertExactJournalStoreNames(t *testing.T, label string, got map[string]struct{}, want []string) {
	t.Helper()

	wantSet := map[string]struct{}{}
	for _, name := range want {
		wantSet[name] = struct{}{}
	}

	if len(got) != len(wantSet) {
		t.Fatalf("unexpected %s count: got %v want %v", label, sortedJournalStoreKeys(got), want)
	}

	for name := range wantSet {
		if _, ok := got[name]; !ok {
			t.Fatalf("missing %s entry %q in %v", label, name, sortedJournalStoreKeys(got))
		}
	}

	for name := range got {
		if _, ok := wantSet[name]; !ok {
			t.Fatalf("unexpected %s entry %q in %v", label, name, sortedJournalStoreKeys(got))
		}
	}
}

func sortedJournalStoreKeys(set map[string]struct{}) []string {
	keys := make([]string, 0, len(set))
	for key := range set {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
