package backupstore

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/lazuale/espocrm-ops/internal/testutil"
)

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

func TestBackupStoreErrorCodesStayOnWrapperErrors(t *testing.T) {
	receivers := errorCodeReceivers(t)
	want := []string{"ManifestError", "VerificationError"}

	if len(receivers) != len(want) {
		t.Fatalf("unexpected ErrorCode receivers: got %#v want %#v", receivers, want)
	}
	for i := range want {
		if receivers[i] != want[i] {
			t.Fatalf("unexpected ErrorCode receivers: got %#v want %#v", receivers, want)
		}
	}
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
