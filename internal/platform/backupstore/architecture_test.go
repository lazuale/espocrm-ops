package backupstore

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/lazuale/espocrm-ops/internal/testutil"
)

const modulePath = "github.com/lazuale/espocrm-ops"

func TestBackupStoreProductionFileSetStaysBounded(t *testing.T) {
	root := testutil.RepoRoot(t)
	pkgDir := filepath.Join(root, "internal", "platform", "backupstore")
	files, err := packageFiles(pkgDir)
	if err != nil {
		t.Fatal(err)
	}

	got := map[string]struct{}{}
	want := map[string]struct{}{
		"artifact.go":   {},
		"candidates.go": {},
		"errors.go":     {},
		"files.go":      {},
		"groups.go":     {},
		"manifest.go":   {},
		"verify.go":     {},
	}

	for _, filePath := range files {
		got[filepath.Base(filePath)] = struct{}{}
	}

	if len(got) != len(want) {
		t.Fatalf("unexpected backupstore production file set: got %v want %v", sortedBackupStoreKeys(got), sortedBackupStoreKeys(want))
	}
	for name := range want {
		if _, ok := got[name]; !ok {
			t.Fatalf("missing backupstore production file %q in %v", name, sortedBackupStoreKeys(got))
		}
	}
	for name := range got {
		if _, ok := want[name]; !ok {
			t.Fatalf("unexpected backupstore production file %q in %v", name, sortedBackupStoreKeys(got))
		}
	}
}

func TestBackupStoreExportedSurfaceIsIntentional(t *testing.T) {
	got := exportedBackupStoreNames(t)
	want := []string{
		"GroupMode",
		"GroupModeAny",
		"GroupModeDB",
		"GroupModeFiles",
		"Groups",
		"LoadManifest",
		"ManifestCandidate",
		"ManifestCandidates",
		"ManifestError",
		"VerificationError",
		"VerifiedBackup",
		"VerifyDirectDBBackup",
		"VerifyDirectFilesBackup",
		"VerifyManifestDetailed",
		"WriteManifest",
		"WriteSHA256Sidecar",
	}

	if len(got) != len(want) {
		t.Fatalf("unexpected exported surface:\n got: %#v\nwant: %#v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("unexpected exported surface:\n got: %#v\nwant: %#v", got, want)
		}
	}
}

func TestBackupStoreDoesNotOwnErrorCodes(t *testing.T) {
	receivers := errorCodeReceivers(t)
	if len(receivers) != 0 {
		t.Fatalf("backupstore must not define ErrorCode methods, found %#v", receivers)
	}
}

func TestBackupStoreInternalImportsStayExact(t *testing.T) {
	root := testutil.RepoRoot(t)
	pkgDir := filepath.Join(root, "internal", "platform", "backupstore")
	fset := token.NewFileSet()
	files, err := packageFiles(pkgDir)
	if err != nil {
		t.Fatal(err)
	}

	allowedInternalByFile := map[string]map[string]struct{}{
		"artifact.go":   {},
		"candidates.go": {modulePath + "/internal/domain/backup": {}},
		"errors.go":     {},
		"files.go":      {},
		"groups.go":     {modulePath + "/internal/domain/backup": {}},
		"manifest.go":   {modulePath + "/internal/domain/backup": {}},
		"verify.go": {
			modulePath + "/internal/domain/backup": {},
			modulePath + "/internal/platform/fs":   {},
		},
	}

	for _, filePath := range files {
		fileName := filepath.Base(filePath)
		allowedInternal, ok := allowedInternalByFile[fileName]
		if !ok {
			t.Fatalf("unexpected backupstore production file %s", fileName)
		}

		file, err := parser.ParseFile(fset, filePath, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatal(err)
		}

		got := map[string]struct{}{}
		for _, imp := range file.Imports {
			resolved := strings.Trim(imp.Path.Value, `"`)
			if !strings.HasPrefix(resolved, modulePath+"/internal/") {
				continue
			}
			got[resolved] = struct{}{}
		}

		if len(got) != len(allowedInternal) {
			t.Fatalf("%s imports unexpected internal surface: got %v want %v", fileName, sortedBackupStoreKeys(got), sortedBackupStoreKeys(allowedInternal))
		}
		for name := range allowedInternal {
			if _, ok := got[name]; !ok {
				t.Fatalf("%s missing internal import %q in %v", fileName, name, sortedBackupStoreKeys(got))
			}
		}
		for name := range got {
			if _, ok := allowedInternal[name]; !ok {
				t.Fatalf("%s imports unexpected internal package %q in %v", fileName, name, sortedBackupStoreKeys(got))
			}
		}
	}
}

func TestBackupStoreDefinitionsStayExplicit(t *testing.T) {
	assertBackupStoreTextOwnership(t, "type artifactInspection struct", map[string]struct{}{
		"artifact.go": {},
	})
	assertBackupStoreTextOwnership(t, "func inspectBackupArtifact(", map[string]struct{}{
		"artifact.go": {},
	})
	assertBackupStoreTextOwnership(t, "type ManifestCandidate struct", map[string]struct{}{
		"candidates.go": {},
	})
	assertBackupStoreTextOwnership(t, "func ManifestCandidates(", map[string]struct{}{
		"candidates.go": {},
	})
	assertBackupStoreTextOwnership(t, "type VerificationError struct", map[string]struct{}{
		"errors.go": {},
	})
	assertBackupStoreTextOwnership(t, "type manifestCoherenceError struct", map[string]struct{}{
		"errors.go": {},
	})
	assertBackupStoreTextOwnership(t, "type fileInfo struct", map[string]struct{}{
		"files.go": {},
	})
	assertBackupStoreTextOwnership(t, "func inspectFile(", map[string]struct{}{
		"files.go": {},
	})
	assertBackupStoreTextOwnership(t, "type GroupMode int", map[string]struct{}{
		"groups.go": {},
	})
	assertBackupStoreTextOwnership(t, "func Groups(", map[string]struct{}{
		"groups.go": {},
	})
	assertBackupStoreTextOwnership(t, "type ManifestError struct", map[string]struct{}{
		"manifest.go": {},
	})
	assertBackupStoreTextOwnership(t, "func LoadManifest(", map[string]struct{}{
		"manifest.go": {},
	})
	assertBackupStoreTextOwnership(t, "func WriteManifest(", map[string]struct{}{
		"manifest.go": {},
	})
	assertBackupStoreTextOwnership(t, "func WriteSHA256Sidecar(", map[string]struct{}{
		"manifest.go": {},
	})
	assertBackupStoreTextOwnership(t, "type VerifiedBackup struct", map[string]struct{}{
		"verify.go": {},
	})
	assertBackupStoreTextOwnership(t, "func VerifyManifestDetailed(", map[string]struct{}{
		"verify.go": {},
	})
	assertBackupStoreTextOwnership(t, "func VerifyDirectDBBackup(", map[string]struct{}{
		"verify.go": {},
	})
	assertBackupStoreTextOwnership(t, "func VerifyDirectFilesBackup(", map[string]struct{}{
		"verify.go": {},
	})
	assertBackupStoreTextOwnership(t, "func verifyArtifactChecksum(", map[string]struct{}{
		"verify.go": {},
	})
}

func exportedBackupStoreNames(t *testing.T) []string {
	t.Helper()

	root := testutil.RepoRoot(t)
	pkgDir := filepath.Join(root, "internal", "platform", "backupstore")
	fset := token.NewFileSet()
	files, err := packageFiles(pkgDir)
	if err != nil {
		t.Fatal(err)
	}

	var names []string
	for _, filePath := range files {
		file, err := parser.ParseFile(fset, filePath, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, decl := range file.Decls {
			switch typed := decl.(type) {
			case *ast.FuncDecl:
				if typed.Recv == nil && ast.IsExported(typed.Name.Name) {
					names = append(names, typed.Name.Name)
				}
			case *ast.GenDecl:
				for _, spec := range typed.Specs {
					switch ts := spec.(type) {
					case *ast.TypeSpec:
						if ast.IsExported(ts.Name.Name) {
							names = append(names, ts.Name.Name)
						}
					case *ast.ValueSpec:
						for _, name := range ts.Names {
							if ast.IsExported(name.Name) {
								names = append(names, name.Name)
							}
						}
					}
				}
			}
		}
	}

	sort.Strings(names)
	return names
}

func errorCodeReceivers(t *testing.T) []string {
	t.Helper()

	root := testutil.RepoRoot(t)
	pkgDir := filepath.Join(root, "internal", "platform", "backupstore")
	fset := token.NewFileSet()
	files, err := packageFiles(pkgDir)
	if err != nil {
		t.Fatal(err)
	}

	var receivers []string
	for _, filePath := range files {
		file, err := parser.ParseFile(fset, filePath, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Name.Name != "ErrorCode" || fn.Recv == nil || len(fn.Recv.List) != 1 {
				continue
			}

			switch expr := fn.Recv.List[0].Type.(type) {
			case *ast.Ident:
				receivers = append(receivers, expr.Name)
			case *ast.StarExpr:
				if ident, ok := expr.X.(*ast.Ident); ok {
					receivers = append(receivers, ident.Name)
				}
			}
		}
	}

	sort.Strings(receivers)
	return receivers
}

func assertBackupStoreTextOwnership(t *testing.T, needle string, owners map[string]struct{}) {
	t.Helper()

	root := testutil.RepoRoot(t)
	pkgDir := filepath.Join(root, "internal", "platform", "backupstore")
	files, err := packageFiles(pkgDir)
	if err != nil {
		t.Fatal(err)
	}

	for _, filePath := range files {
		raw, err := os.ReadFile(filePath)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(raw), needle) {
			continue
		}
		if _, ok := owners[filepath.Base(filePath)]; !ok {
			t.Fatalf("%s must stay owner-local to %v; found in %s", needle, sortedBackupStoreKeys(owners), filePath)
		}
	}
}

func packageFiles(pkgDir string) ([]string, error) {
	paths, err := filepath.Glob(filepath.Join(pkgDir, "*.go"))
	if err != nil {
		return nil, err
	}

	files := make([]string, 0, len(paths))
	for _, path := range paths {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		files = append(files, path)
	}

	return files, nil
}

func sortedBackupStoreKeys(set map[string]struct{}) []string {
	keys := make([]string, 0, len(set))
	for key := range set {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
