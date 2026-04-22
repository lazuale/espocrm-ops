package cli

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/lazuale/espocrm-ops/internal/testutil"
)

func TestProductionCLIPackageHasNoPackageVars(t *testing.T) {
	root := testutil.RepoRoot(t)
	cliDir := filepath.Join(root, "internal", "cli")
	fset := token.NewFileSet()

	err := filepath.WalkDir(cliDir, func(path string, entry os.DirEntry, err error) error {
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
			gen, ok := decl.(*ast.GenDecl)
			if !ok || gen.Tok != token.VAR {
				continue
			}

			var names []string
			for _, spec := range gen.Specs {
				valueSpec, ok := spec.(*ast.ValueSpec)
				if !ok {
					continue
				}
				for _, name := range valueSpec.Names {
					names = append(names, name.Name)
				}
			}

			t.Fatalf("production package-level var in internal/cli at %s: %s", fset.Position(gen.Pos()), strings.Join(names, ", "))
		}

		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestProductionCLIEnvReadsStayAtEdgeHelpers(t *testing.T) {
	root := testutil.RepoRoot(t)
	cliDir := filepath.Join(root, "internal", "cli")
	fset := token.NewFileSet()

	err := filepath.WalkDir(cliDir, func(path string, entry os.DirEntry, err error) error {
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

		ast.Inspect(file, func(node ast.Node) bool {
			selector, ok := node.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			switch selector.Sel.Name {
			case "Getenv", "LookupEnv", "Environ":
			default:
				return true
			}
			ident, ok := selector.X.(*ast.Ident)
			if ok && ident.Name == "os" {
				t.Fatalf("production CLI env read must stay in env helpers at %s", fset.Position(selector.Pos()))
			}
			return true
		})

		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestProductionCLIExecutionBeginsOnlyInRunner(t *testing.T) {
	root := testutil.RepoRoot(t)
	cliDir := filepath.Join(root, "internal", "cli")
	fset := token.NewFileSet()

	err := filepath.WalkDir(cliDir, func(path string, entry os.DirEntry, err error) error {
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

		ast.Inspect(file, func(node ast.Node) bool {
			selector, ok := node.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			if selector.Sel == nil || selector.Sel.Name != "Begin" {
				return true
			}
			ident, ok := selector.X.(*ast.Ident)
			if !ok || ident.Name != "operationusecase" {
				return true
			}
			if filepath.Base(path) != "runner.go" {
				t.Fatalf("production CLI operation execution must begin only in runner.go; found at %s", fset.Position(selector.Pos()))
			}
			return true
		})

		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestProductionCLIExecutionFinishesOnlyInRunner(t *testing.T) {
	assertCLITextOwnership(t, ".FinishSuccess(", map[string]struct{}{
		"runner.go": {},
	})
	assertCLITextOwnership(t, ".FinishFailure(", map[string]struct{}{
		"runner.go": {},
	})
}

func TestProductionCLIJournalProjectionStaysBehindRunner(t *testing.T) {
	assertCLITextOwnership(t, "journalRecordFromResult(", map[string]struct{}{
		"journal_record.go": {},
		"runner.go":         {},
	})
}

func TestProductionCLIAppOperationBridgeFilesStayExplicit(t *testing.T) {
	assertCLIImportOwnership(t, "github.com/lazuale/espocrm-ops/internal/app/operation", map[string]struct{}{
		"deps.go":           {},
		"journal_record.go": {},
		"runner.go":         {},
	})
}

func TestProductionCLIErrorTransportRouteStaysCanonical(t *testing.T) {
	assertCLITextOwnership(t, "ErrorResult(", map[string]struct{}{
		"errors.go":  {},
		"execute.go": {},
	})
	assertCLITextOwnership(t, ".CommandResult()", map[string]struct{}{
		"execute.go": {},
	})
	assertCLITextOwnership(t, "ResultCodeError{", map[string]struct{}{
		"runner.go": {},
	})
}

func TestProductionCLITransportBridgeDefinitionsStayExplicit(t *testing.T) {
	assertCLITextOwnership(t, "func renderExecutionError(", map[string]struct{}{
		"execute.go": {},
	})
	assertCLITextOwnership(t, "type textErrorSuppressor interface", map[string]struct{}{
		"execute.go": {},
	})
	assertCLITextOwnership(t, "type resultCarrier interface", map[string]struct{}{
		"result_error.go": {},
	})
	assertCLITextOwnership(t, "type ResultCodeError struct", map[string]struct{}{
		"result_error.go": {},
	})
	assertCLITextOwnership(t, "type silentCodeError struct", map[string]struct{}{
		"errors.go": {},
	})
	assertCLITextOwnership(t, "func usageError(", map[string]struct{}{
		"errors.go": {},
	})
	assertCLITextOwnership(t, "func ErrorResult(", map[string]struct{}{
		"errors.go": {},
	})
	assertCLITextOwnership(t, "func IsUsageError(", map[string]struct{}{
		"errors.go": {},
	})
	assertCLITextOwnership(t, "func appendCommandWarning(", map[string]struct{}{
		"runner.go": {},
	})
	assertCLITextOwnership(t, "func renderWarnings(", map[string]struct{}{
		"runner.go": {},
	})
	assertCLITextOwnership(t, "func journalRecordFromResult(", map[string]struct{}{
		"journal_record.go": {},
	})
	assertCLITextOwnership(t, "func applyExecutionCompletion(", map[string]struct{}{
		"journal_record.go": {},
	})
}

func TestCLITestDockerHarnessStaysSingleOwner(t *testing.T) {
	root := testutil.RepoRoot(t)
	cliDir := filepath.Join(root, "internal", "cli")
	owners := map[string]struct{}{
		"recovery_test_helpers_test.go": {},
	}
	legacyPrefix := "DOCKER_" + "MOCK_"
	harnessPrefix := "DOCKER_" + "TEST_"

	err := filepath.WalkDir(cliDir, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || !strings.HasSuffix(path, "_test.go") {
			return nil
		}

		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		text := string(raw)
		if strings.Contains(text, legacyPrefix) {
			t.Fatalf("legacy docker mock env dialect remains in %s", path)
		}
		if !strings.Contains(text, harnessPrefix) {
			return nil
		}
		if _, ok := owners[filepath.Base(path)]; !ok {
			t.Fatalf("docker harness env plumbing must stay in recovery_test_helpers_test.go; found in %s", path)
		}

		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestCLISchemaTestsDoNotAssertGoldenSnapshots(t *testing.T) {
	root := testutil.RepoRoot(t)
	cliDir := filepath.Join(root, "internal", "cli")

	err := filepath.WalkDir(cliDir, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || !strings.HasSuffix(path, "_schema_test.go") {
			return nil
		}

		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if strings.Contains(string(raw), "assertGoldenJSON(") {
			t.Fatalf("schema tests must stay separate from golden assertions; found in %s", path)
		}

		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func assertCLITextOwnership(t *testing.T, needle string, owners map[string]struct{}) {
	t.Helper()

	root := testutil.RepoRoot(t)
	cliDir := filepath.Join(root, "internal", "cli")

	err := filepath.WalkDir(cliDir, func(path string, entry os.DirEntry, err error) error {
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
			t.Fatalf("%s must stay owner-local to %v; found in %s", needle, ownerNames(owners), path)
		}

		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func assertCLIImportOwnership(t *testing.T, importPath string, owners map[string]struct{}) {
	t.Helper()

	root := testutil.RepoRoot(t)
	cliDir := filepath.Join(root, "internal", "cli")
	fset := token.NewFileSet()

	err := filepath.WalkDir(cliDir, func(path string, entry os.DirEntry, err error) error {
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
				t.Fatalf("%s import must stay owner-local to %v; found in %s", importPath, ownerNames(owners), path)
			}
		}

		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func ownerNames(owners map[string]struct{}) []string {
	names := make([]string, 0, len(owners))
	for name := range owners {
		names = append(names, name)
	}
	return names
}
