package operation

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/lazuale/espocrm-ops/internal/testutil"
)

const modulePath = "github.com/lazuale/espocrm-ops"

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

func TestOperationLifecycleDoesNotImportJournalOrTraceBridge(t *testing.T) {
	assertOperationImportAbsent(t, modulePath+"/internal/domain/journal")
	assertOperationImportAbsent(t, modulePath+"/internal/app/operationtrace")
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
