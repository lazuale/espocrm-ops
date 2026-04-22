package locks

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

func TestLocksProductionFileSetStaysBounded(t *testing.T) {
	got := map[string]struct{}{}
	want := map[string]struct{}{
		"file_lock.go":      {},
		"operation_lock.go": {},
		"readiness.go":      {},
	}

	for _, path := range locksPackageFiles(t) {
		got[filepath.Base(path)] = struct{}{}
	}

	if len(got) != len(want) {
		t.Fatalf("unexpected locks production file set: got %v want %v", sortedLockKeys(got), sortedLockKeys(want))
	}
	for name := range want {
		if _, ok := got[name]; !ok {
			t.Fatalf("missing locks production file %q in %v", name, sortedLockKeys(got))
		}
	}
	for name := range got {
		if _, ok := want[name]; !ok {
			t.Fatalf("unexpected locks production file %q in %v", name, sortedLockKeys(got))
		}
	}
}

func TestLocksExportedSurfaceIsIntentional(t *testing.T) {
	files := locksPackageFiles(t)
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

	want := []string{
		"AcquireFileLock",
		"AcquireJournalPruneLock",
		"AcquireMaintenanceLock",
		"AcquireRestoreDBLock",
		"AcquireRestoreDBLockInDir",
		"AcquireRestoreFilesLock",
		"AcquireRestoreFilesLockInDir",
		"AcquireSharedOperationLock",
		"CheckMaintenanceReadiness",
		"CheckRestoreDBReadiness",
		"CheckRestoreDBReadinessInDir",
		"CheckRestoreFilesReadiness",
		"CheckRestoreFilesReadinessInDir",
		"CheckSharedOperationReadiness",
		"FileLock",
		"LockActive",
		"LockError",
		"LockReadiness",
		"LockReady",
		"LockStale",
		"MaintenanceConflictError",
		"MaintenanceLock",
		"OperationLock",
	}

	assertExactLockNames(t, "exported surface", exported, want)
}

func TestLocksPackageIsStdlibOnly(t *testing.T) {
	files := locksPackageFiles(t)
	fset := token.NewFileSet()

	for _, path := range files {
		file, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		for _, imp := range file.Imports {
			importPath := strings.Trim(imp.Path.Value, `"`)
			if strings.Contains(importPath, ".") {
				t.Fatalf("locks package must stay stdlib-only; found %s in %s", importPath, path)
			}
		}
	}
}

func TestLocksPackageDoesNotOwnErrorCodes(t *testing.T) {
	files := locksPackageFiles(t)
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
			methods[lockReceiverName(fn)] = struct{}{}
		}
	}

	if len(methods) != 0 {
		t.Fatalf("locks package must not define ErrorCode methods, found: %v", sortedLockKeys(methods))
	}
}

func TestLocksPackageDoesNotReadProcessEnvOrShellOut(t *testing.T) {
	for _, needle := range []string{
		"os.Getenv(",
		"os.LookupEnv(",
		"os.Environ(",
		"os.Getwd(",
		"exec.Command(",
	} {
		assertLocksTextAbsent(t, needle)
	}
}

func TestLocksDefinitionsStayExplicit(t *testing.T) {
	assertLocksTextOwnership(t, "type FileLock struct", map[string]struct{}{
		"file_lock.go": {},
	})
	assertLocksTextOwnership(t, "func AcquireJournalPruneLock(", map[string]struct{}{
		"file_lock.go": {},
	})
	assertLocksTextOwnership(t, "func AcquireRestoreDBLock(", map[string]struct{}{
		"file_lock.go": {},
	})
	assertLocksTextOwnership(t, "func AcquireRestoreFilesLock(", map[string]struct{}{
		"file_lock.go": {},
	})
	assertLocksTextOwnership(t, "func AcquireRestoreDBLockInDir(", map[string]struct{}{
		"file_lock.go": {},
	})
	assertLocksTextOwnership(t, "func AcquireRestoreFilesLockInDir(", map[string]struct{}{
		"file_lock.go": {},
	})
	assertLocksTextOwnership(t, "func AcquireFileLock(", map[string]struct{}{
		"file_lock.go": {},
	})
	assertLocksTextOwnership(t, "func pruneLockPath(", map[string]struct{}{
		"file_lock.go": {},
	})
	assertLocksTextOwnership(t, "func restoreLockPathInDir(", map[string]struct{}{
		"file_lock.go": {},
	})
	assertLocksTextOwnership(t, "type OperationLock struct", map[string]struct{}{
		"operation_lock.go": {},
	})
	assertLocksTextOwnership(t, "type MaintenanceLock struct", map[string]struct{}{
		"operation_lock.go": {},
	})
	assertLocksTextOwnership(t, "type MaintenanceConflictError struct", map[string]struct{}{
		"operation_lock.go": {},
	})
	assertLocksTextOwnership(t, "func AcquireSharedOperationLock(", map[string]struct{}{
		"operation_lock.go": {},
	})
	assertLocksTextOwnership(t, "func AcquireMaintenanceLock(", map[string]struct{}{
		"operation_lock.go": {},
	})
	assertLocksTextOwnership(t, "func sharedOperationLockPaths(", map[string]struct{}{
		"operation_lock.go": {},
	})
	assertLocksTextOwnership(t, "func predictedSharedOperationLockPaths(", map[string]struct{}{
		"operation_lock.go": {},
	})
	assertLocksTextOwnership(t, "func metadataLockHandlePath(", map[string]struct{}{
		"operation_lock.go": {},
	})
	assertLocksTextOwnership(t, "func metadataLockState(", map[string]struct{}{
		"operation_lock.go": {},
	})
	assertLocksTextOwnership(t, "func writeLockMetadataFile(", map[string]struct{}{
		"operation_lock.go": {},
	})
	assertLocksTextOwnership(t, "func releaseMetadataLock(", map[string]struct{}{
		"operation_lock.go": {},
	})
	assertLocksTextOwnership(t, "func lockFileOwnerPID(", map[string]struct{}{
		"operation_lock.go": {},
	})
	assertLocksTextOwnership(t, "func splitLines(", map[string]struct{}{
		"operation_lock.go": {},
	})
	assertLocksTextOwnership(t, "func lockInfo(", map[string]struct{}{
		"operation_lock.go": {},
	})
	assertLocksTextOwnership(t, "func lockWarn(", map[string]struct{}{
		"operation_lock.go": {},
	})
	assertLocksTextOwnership(t, "type LockReadiness struct", map[string]struct{}{
		"readiness.go": {},
	})
	assertLocksTextOwnership(t, "func CheckSharedOperationReadiness(", map[string]struct{}{
		"readiness.go": {},
	})
	assertLocksTextOwnership(t, "func CheckMaintenanceReadiness(", map[string]struct{}{
		"readiness.go": {},
	})
	assertLocksTextOwnership(t, "func CheckRestoreDBReadiness(", map[string]struct{}{
		"readiness.go": {},
	})
	assertLocksTextOwnership(t, "func CheckRestoreFilesReadiness(", map[string]struct{}{
		"readiness.go": {},
	})
	assertLocksTextOwnership(t, "func CheckRestoreDBReadinessInDir(", map[string]struct{}{
		"readiness.go": {},
	})
	assertLocksTextOwnership(t, "func CheckRestoreFilesReadinessInDir(", map[string]struct{}{
		"readiness.go": {},
	})
	assertLocksTextOwnership(t, "func checkFileLockReadiness(", map[string]struct{}{
		"readiness.go": {},
	})
	assertLocksTextOwnership(t, "func closeLockHandle(", map[string]struct{}{
		"readiness.go": {},
	})
}

func locksPackageFiles(t *testing.T) []string {
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

func assertLocksTextOwnership(t *testing.T, needle string, owners map[string]struct{}) {
	t.Helper()

	for _, path := range locksPackageFiles(t) {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		if !strings.Contains(string(raw), needle) {
			continue
		}
		if _, ok := owners[filepath.Base(path)]; !ok {
			t.Fatalf("%s must stay owner-local to %v; found in %s", needle, sortedLockKeys(owners), path)
		}
	}
}

func assertLocksTextAbsent(t *testing.T, needle string) {
	t.Helper()

	for _, path := range locksPackageFiles(t) {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		if strings.Contains(string(raw), needle) {
			t.Fatalf("locks package must not contain %s; found in %s", needle, path)
		}
	}
}

func lockReceiverName(fn *ast.FuncDecl) string {
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

func assertExactLockNames(t *testing.T, label string, got map[string]struct{}, want []string) {
	t.Helper()

	wantSet := map[string]struct{}{}
	for _, name := range want {
		wantSet[name] = struct{}{}
	}

	if len(got) != len(wantSet) {
		t.Fatalf("unexpected %s count: got %v want %v", label, sortedLockKeys(got), want)
	}

	for name := range wantSet {
		if _, ok := got[name]; !ok {
			t.Fatalf("missing %s entry %q in %v", label, name, sortedLockKeys(got))
		}
	}

	for name := range got {
		if _, ok := wantSet[name]; !ok {
			t.Fatalf("unexpected %s entry %q in %v", label, name, sortedLockKeys(got))
		}
	}
}

func sortedLockKeys(set map[string]struct{}) []string {
	keys := make([]string, 0, len(set))
	for key := range set {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
