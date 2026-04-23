package cli

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

func TestProductionCLIRootFileSetStaysResidual(t *testing.T) {
	root := testutil.RepoRoot(t)
	paths, err := filepath.Glob(filepath.Join(root, "internal", "cli", "*.go"))
	if err != nil {
		t.Fatal(err)
	}

	got := map[string]struct{}{}
	want := map[string]struct{}{
		"backup.go":        {},
		"backup_verify.go": {},
		"deps.go":          {},
		"doctor.go":        {},
		"execute.go":       {},
		"input.go":         {},
		"migrate.go":       {},
		"options.go":       {},
		"restore.go":       {},
		"root.go":          {},
		"runner.go":        {},
	}

	for _, path := range paths {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		got[filepath.Base(path)] = struct{}{}
	}

	if len(got) != len(want) {
		t.Fatalf("unexpected root cli production file set: got %v want %v", cliSortedKeys(got), cliSortedKeys(want))
	}
	for name := range want {
		if _, ok := got[name]; !ok {
			t.Fatalf("missing root cli production file %q in %v", name, cliSortedKeys(got))
		}
	}
	for name := range got {
		if _, ok := want[name]; !ok {
			t.Fatalf("unexpected root cli production file %q in %v", name, cliSortedKeys(got))
		}
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
			if !ok || ident.Name != "operationtrace" {
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
	assertCLITextOwnership(t, "journalbridge.RecordFromResult(", map[string]struct{}{
		"runner.go": {},
	})
	assertCLITextOwnership(t, "journalbridge.ApplyExecutionCompletion(", map[string]struct{}{
		"runner.go": {},
	})
}

func TestProductionCLIAppOperationBridgeFilesStayExplicit(t *testing.T) {
	assertCLIImportOwnership(t, "github.com/lazuale/espocrm-ops/internal/app/operation", map[string]struct{}{
		"deps.go": {},
	})
	assertCLIImportOwnership(t, "github.com/lazuale/espocrm-ops/internal/app/operationtrace", map[string]struct{}{
		"deps.go":           {},
		"journal_record.go": {},
		"runner.go":         {},
	})
}

func TestProductionCLIPlatformAdapterBridgeFilesStayExplicit(t *testing.T) {
	assertCLIImportOwnership(t, "github.com/lazuale/espocrm-ops/internal/platform/appadapter", map[string]struct{}{
		"deps.go": {},
	})
	assertCLIImportOwnership(t, "github.com/lazuale/espocrm-ops/internal/platform/envadapter", map[string]struct{}{
		"deps.go": {},
	})
	assertCLIImportOwnership(t, "github.com/lazuale/espocrm-ops/internal/platform/runtimeadapter", map[string]struct{}{
		"deps.go": {},
	})
	assertCLIImportOwnership(t, "github.com/lazuale/espocrm-ops/internal/platform/backupstoreadapter", map[string]struct{}{
		"deps.go": {},
	})
}

func TestProductionCLIRunnerImportsStayExplicit(t *testing.T) {
	assertCLIFileInternalImportsExactly(t, "runner.go", map[string]struct{}{
		"github.com/lazuale/espocrm-ops/internal/app/operationtrace": {},
		"github.com/lazuale/espocrm-ops/internal/cli/errortransport": {},
		"github.com/lazuale/espocrm-ops/internal/cli/journalbridge":  {},
		"github.com/lazuale/espocrm-ops/internal/cli/resultbridge":   {},
		"github.com/lazuale/espocrm-ops/internal/contract/result":    {},
	})
}

func TestProductionCLIExecuteImportsStayExplicit(t *testing.T) {
	assertCLIFileInternalImportsExactly(t, "execute.go", map[string]struct{}{
		"github.com/lazuale/espocrm-ops/internal/cli/errortransport": {},
		"github.com/lazuale/espocrm-ops/internal/contract/exitcode":  {},
		"github.com/lazuale/espocrm-ops/internal/contract/result":    {},
	})
}

func TestProductionCLIDepsImportsStayExplicit(t *testing.T) {
	assertCLIFileInternalImportsExactly(t, "deps.go", map[string]struct{}{
		"github.com/lazuale/espocrm-ops/internal/app":                         {},
		"github.com/lazuale/espocrm-ops/internal/app/doctor":                  {},
		"github.com/lazuale/espocrm-ops/internal/app/migrate":                 {},
		"github.com/lazuale/espocrm-ops/internal/app/operation":               {},
		"github.com/lazuale/espocrm-ops/internal/app/operationtrace":          {},
		"github.com/lazuale/espocrm-ops/internal/app/ports/lockport":          {},
		"github.com/lazuale/espocrm-ops/internal/app/restore":                 {},
		"github.com/lazuale/espocrm-ops/internal/platform/appadapter":         {},
		"github.com/lazuale/espocrm-ops/internal/platform/backupstoreadapter": {},
		"github.com/lazuale/espocrm-ops/internal/platform/envadapter":         {},
		"github.com/lazuale/espocrm-ops/internal/platform/runtimeadapter":     {},
		"github.com/lazuale/espocrm-ops/internal/runtime":                     {},
		"github.com/lazuale/espocrm-ops/internal/store":                       {},
	})
}

func TestProductionCLIErrorTransportBridgeFilesStayExplicit(t *testing.T) {
	assertCLIImportOwnership(t, "github.com/lazuale/espocrm-ops/internal/cli/errortransport", map[string]struct{}{
		"doctor.go":  {},
		"execute.go": {},
		"input.go":   {},
		"runner.go":  {},
	})
}

func TestProductionCLIResultBridgeFilesStayExplicit(t *testing.T) {
	assertCLIImportOwnership(t, "github.com/lazuale/espocrm-ops/internal/cli/resultbridge", map[string]struct{}{
		"backup.go":        {},
		"backup_verify.go": {},
		"doctor.go":        {},
		"migrate.go":       {},
		"restore.go":       {},
		"runner.go":        {},
	})
}

func TestProductionCLIErrorTransportRouteStaysCanonical(t *testing.T) {
	assertCLITextOwnership(t, "errortransport.ErrorResult(", map[string]struct{}{
		"execute.go": {},
	})
	assertCLITextOwnership(t, ".CommandResult()", map[string]struct{}{
		"execute.go": {},
	})
	assertCLITextOwnership(t, "errortransport.ResultCodeError{", map[string]struct{}{
		"runner.go": {},
	})
}

func TestProductionCLIResultBridgeRouteStaysCanonical(t *testing.T) {
	assertCLITextOwnership(t, "resultbridge.RenderWarnings(", map[string]struct{}{
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
		"execute.go": {},
	})
	assertCLITextOwnership(t, "func usageError(", map[string]struct{}{
		"input.go": {},
	})
	assertCLITextOwnership(t, "func requiredFlagError(", map[string]struct{}{
		"input.go": {},
	})
	assertCLITextOwnership(t, "func requireNonBlankFlag(", map[string]struct{}{
		"input.go": {},
	})
	assertCLITextOwnership(t, "func normalizeOptionalStringFlag(", map[string]struct{}{
		"input.go": {},
	})
	assertCLITextOwnership(t, "func noArgs(", map[string]struct{}{
		"input.go": {},
	})
	assertCLITextOwnership(t, "func appendCommandWarning(", map[string]struct{}{
		"runner.go": {},
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

func assertCLIFileInternalImportsExactly(t *testing.T, fileName string, allowed map[string]struct{}) {
	t.Helper()

	root := testutil.RepoRoot(t)
	path := filepath.Join(root, "internal", "cli", fileName)
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
	if err != nil {
		t.Fatal(err)
	}

	got := map[string]struct{}{}
	for _, imp := range file.Imports {
		resolved, err := strconv.Unquote(imp.Path.Value)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.HasPrefix(resolved, "github.com/lazuale/espocrm-ops/internal/") {
			continue
		}
		got[resolved] = struct{}{}
	}

	if len(got) != len(allowed) {
		t.Fatalf("%s imports unexpected internal surface: got %v want %v", fileName, cliSortedKeys(got), cliSortedKeys(allowed))
	}
	for name := range allowed {
		if _, ok := got[name]; !ok {
			t.Fatalf("%s missing internal import %q in %v", fileName, name, cliSortedKeys(got))
		}
	}
	for name := range got {
		if _, ok := allowed[name]; !ok {
			t.Fatalf("%s imports unexpected internal package %q in %v", fileName, name, cliSortedKeys(got))
		}
	}
}

func ownerNames(owners map[string]struct{}) []string {
	names := make([]string, 0, len(owners))
	for name := range owners {
		names = append(names, name)
	}
	return names
}

func cliSortedKeys(set map[string]struct{}) []string {
	keys := make([]string, 0, len(set))
	for key := range set {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
