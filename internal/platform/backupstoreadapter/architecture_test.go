package backupstoreadapter

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

func TestBackupStoreAdapterImportsStayExplicit(t *testing.T) {
	root := testutil.RepoRoot(t)
	dir := filepath.Join(root, "internal", "platform", "backupstoreadapter")
	fset := token.NewFileSet()
	allowedInternal := map[string]struct{}{
		modulePath + "/internal/app/ports/backupstoreport": {},
		modulePath + "/internal/domain/backup":             {},
		modulePath + "/internal/domain/failure":            {},
		modulePath + "/internal/platform/backupstore":      {},
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
				t.Fatalf("backup store adapter imports unexpected internal package %s at %s", resolved, fset.Position(imp.Pos()))
			}
		}

		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestBackupStoreAdapterExportedSurfaceStaysIntentional(t *testing.T) {
	root := testutil.RepoRoot(t)
	dir := filepath.Join(root, "internal", "platform", "backupstoreadapter")
	fset := token.NewFileSet()
	got := map[string]struct{}{}
	want := map[string]struct{}{
		"type BackupStore":                           {},
		"method BackupStore.VerifyManifestDetailed":  {},
		"method BackupStore.VerifyDirectDBBackup":    {},
		"method BackupStore.VerifyDirectFilesBackup": {},
		"method BackupStore.ManifestCandidates":      {},
		"method BackupStore.Groups":                  {},
		"method BackupStore.LoadManifest":            {},
		"method BackupStore.WriteManifest":           {},
		"method BackupStore.WriteSHA256Sidecar":      {},
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
				recv := backupStoreAdapterReceiverName(typed)
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
		t.Fatalf("unexpected exported backup store adapter surface: got %v want %v", backupStoreAdapterSortedKeys(got), backupStoreAdapterSortedKeys(want))
	}
	for name := range want {
		if _, ok := got[name]; !ok {
			t.Fatalf("missing exported backup store adapter symbol %q in %v", name, backupStoreAdapterSortedKeys(got))
		}
	}
	for name := range got {
		if _, ok := want[name]; !ok {
			t.Fatalf("unexpected exported backup store adapter symbol %q in %v", name, backupStoreAdapterSortedKeys(got))
		}
	}
}

func backupStoreAdapterReceiverName(fn *ast.FuncDecl) string {
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

func backupStoreAdapterSortedKeys(set map[string]struct{}) []string {
	keys := make([]string, 0, len(set))
	for key := range set {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
