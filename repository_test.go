package repository_test

import (
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
)

const modulePath = "github.com/lazuale/espocrm-ops"

type listedPackage struct {
	ImportPath string
	Imports    []string
}

func TestInternalDependencyBoundaries(t *testing.T) {
	packages := listInternalPackages(t)

	for _, pkg := range packages {
		switch {
		case inLayer(pkg.ImportPath, "contract"):
			assertNoImports(t, pkg, []string{
				modulePath + "/internal/",
			})
		case inLayer(pkg.ImportPath, "cli"):
			assertNoImportsExcept(t, pkg, []string{
				modulePath + "/internal/domain",
				modulePath + "/internal/platform",
			}, []string{
				modulePath + "/internal/platform/appadapter",
				modulePath + "/internal/platform/backupstoreadapter",
				modulePath + "/internal/platform/envadapter",
				modulePath + "/internal/platform/runtimeadapter",
			})
		case inLayer(pkg.ImportPath, "app"):
			assertNoImportsExcept(t, pkg, []string{
				modulePath + "/internal/cli",
				modulePath + "/internal/contract",
				modulePath + "/internal/platform",
			}, []string{
				modulePath + "/internal/contract/apperr",
			})
		case inLayer(pkg.ImportPath, "domain"):
			assertNoImports(t, pkg, []string{
				modulePath + "/internal/",
			})
			assertStdlibOnly(t, pkg)
		case inLayer(pkg.ImportPath, "platform"):
			assertNoImportsExcept(t, pkg, []string{
				modulePath + "/internal/cli",
				modulePath + "/internal/contract",
				modulePath + "/internal/app",
			}, []string{
				modulePath + "/internal/app/ports",
			})
		}
	}
}

func TestCommandDependencyBoundaries(t *testing.T) {
	packages := listPackages(t, "./cmd/...")

	for _, pkg := range packages {
		assertNoImportsExcept(t, pkg, []string{
			modulePath + "/internal/",
		}, []string{
			modulePath + "/internal/cli",
			modulePath + "/internal/v3/cli",
			modulePath + "/internal/app/operation",
			modulePath + "/internal/app/operationtrace",
			modulePath + "/internal/platform/journalstore",
		})
	}
}

func TestAppAndDomainDoNotReadProcessEnv(t *testing.T) {
	root := repoRoot(t)
	fset := token.NewFileSet()

	for _, dir := range []string{
		filepath.Join(root, "internal", "app"),
		filepath.Join(root, "internal", "domain"),
	} {
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
					t.Fatalf("process env read in backend policy layer at %s", fset.Position(selector.Pos()))
				}
				return true
			})

			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
}

func TestProductionProcessEnvAccessSurfaceIsExplicit(t *testing.T) {
	root := repoRoot(t)
	fset := token.NewFileSet()
	allowedEnvironOwners := map[string]struct{}{
		filepath.Join(root, "internal", "platform", "docker", "docker.go"): {},
		filepath.Join(root, "internal", "v3", "runtime", "docker.go"):      {},
	}

	for _, path := range productionGoFiles(t, filepath.Join(root, "cmd"), filepath.Join(root, "internal")) {
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}

		ast.Inspect(file, func(node ast.Node) bool {
			selector, ok := node.(*ast.SelectorExpr)
			if !ok {
				return true
			}

			ident, ok := selector.X.(*ast.Ident)
			if !ok || ident.Name != "os" {
				return true
			}

			switch selector.Sel.Name {
			case "Getenv", "LookupEnv", "Getwd":
				t.Fatalf("production process env/path access %s must not appear at %s", selector.Sel.Name, fset.Position(selector.Pos()))
			case "Environ":
				if _, ok := allowedEnvironOwners[path]; !ok {
					t.Fatalf("os.Environ must stay in %v; found at %s", sortedStringSet(allowedEnvironOwners), fset.Position(selector.Pos()))
				}
			}

			return true
		})
	}
}

func TestProductionShellExecutionSurfaceIsExplicit(t *testing.T) {
	root := repoRoot(t)
	fset := token.NewFileSet()
	allowedExecOwners := map[string]struct{}{
		filepath.Join(root, "internal", "platform", "docker", "docker.go"):     {},
		filepath.Join(root, "internal", "platform", "fs", "archive_create.go"): {},
		filepath.Join(root, "internal", "v3", "runtime", "docker.go"):          {},
	}

	for _, path := range productionGoFiles(t, filepath.Join(root, "cmd"), filepath.Join(root, "internal")) {
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}

		ast.Inspect(file, func(node ast.Node) bool {
			selector, ok := node.(*ast.SelectorExpr)
			if !ok {
				return true
			}

			ident, ok := selector.X.(*ast.Ident)
			if !ok || ident.Name != "exec" {
				return true
			}

			switch selector.Sel.Name {
			case "Command", "CommandContext":
				if _, ok := allowedExecOwners[path]; !ok {
					t.Fatalf("shell execution seam %s must stay in %v; found at %s", selector.Sel.Name, sortedStringSet(allowedExecOwners), fset.Position(selector.Pos()))
				}
			}

			return true
		})
	}
}

func TestProductionWorkflowStatusVocabularyIsCanonical(t *testing.T) {
	root := repoRoot(t)
	forbidden := []*regexp.Regexp{
		regexp.MustCompile(`\bwould_run\b`),
		regexp.MustCompile(`\bnot_run\b`),
		regexp.MustCompile(`\bWouldRun\b`),
		regexp.MustCompile(`\bNotRun\b`),
	}

	for _, dir := range []string{
		filepath.Join(root, "cmd"),
		filepath.Join(root, "internal"),
	} {
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
			text := string(raw)
			for _, token := range forbidden {
				if token.MatchString(text) {
					t.Fatalf("production file %s contains legacy workflow vocabulary matching %q", path, token.String())
				}
			}

			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
}

func TestProductionErrorCodeOwnershipIsCanonical(t *testing.T) {
	root := repoRoot(t)
	fset := token.NewFileSet()
	got := map[string]struct{}{}
	want := map[string]struct{}{
		"internal/contract/apperr.Error":        {},
		"internal/cli/errortransport.CodeError": {},
	}

	for _, dir := range []string{
		filepath.Join(root, "cmd"),
		filepath.Join(root, "internal"),
	} {
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
				fn, ok := decl.(*ast.FuncDecl)
				if !ok || fn.Recv == nil || fn.Name.Name != "ErrorCode" {
					continue
				}

				rel, err := filepath.Rel(root, path)
				if err != nil {
					return err
				}
				owner := filepath.ToSlash(filepath.Dir(rel)) + "." + receiverBaseName(fn.Recv)
				got[owner] = struct{}{}
			}

			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}

	if len(got) != len(want) {
		t.Fatalf("unexpected ErrorCode owners: got %v want %v", sortedStringSet(got), sortedStringSet(want))
	}
	for owner := range want {
		if _, ok := got[owner]; !ok {
			t.Fatalf("missing canonical ErrorCode owner %q in %v", owner, sortedStringSet(got))
		}
	}
	for owner := range got {
		if _, ok := want[owner]; !ok {
			t.Fatalf("unexpected ErrorCode owner %q in %v", owner, sortedStringSet(got))
		}
	}
}

func TestAppBoundaryPackagesExposeOnlyCanonicalSurface(t *testing.T) {
	root := repoRoot(t)
	allowedServiceMethods := map[string]map[string]struct{}{
		"backup": {
			"Execute": {},
		},
		"backupverify": {
			"Diagnose": {},
		},
		"restore": {
			"Execute": {},
		},
		"migrate": {
			"Execute": {},
		},
		"doctor": {
			"Diagnose": {},
		},
	}
	allowedResultMethods := map[string]map[string]map[string]struct{}{
		"backup": {
			"ExecuteInfo": {
				"Counts": {},
				"Ready":  {},
			},
		},
		"restore": {
			"ExecuteInfo": {
				"Counts": {},
				"Ready":  {},
			},
		},
		"migrate": {
			"ExecuteInfo": {
				"Counts": {},
				"Ready":  {},
			},
		},
		"doctor": {
			"Report": {
				"Counts": {},
				"Ready":  {},
			},
		},
	}

	fset := token.NewFileSet()
	for pkgName, serviceMethods := range allowedServiceMethods {
		dir := filepath.Join(root, "internal", "app", pkgName)
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
				fn, ok := decl.(*ast.FuncDecl)
				if !ok || fn.Name == nil || !fn.Name.IsExported() {
					continue
				}

				receiver := receiverBaseName(fn.Recv)
				if receiver != "" && !ast.IsExported(receiver) {
					continue
				}
				switch {
				case receiver == "" && fn.Name.Name == "NewService":
					continue
				case receiver == "Service" && hasName(serviceMethods, fn.Name.Name):
					continue
				case receiver != "" && hasNestedName(allowedResultMethods[pkgName], receiver, fn.Name.Name):
					continue
				}

				t.Fatalf("%s exports non-canonical app API symbol %s", path, exportedSymbolName(fn))
			}

			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
}

func TestTopLevelAppBoundaryImportEdgesAreCanonical(t *testing.T) {
	root := repoRoot(t)
	allowed := map[string]map[string]struct{}{
		"backup": {
			modulePath + "/internal/app/operation":           {},
			modulePath + "/internal/app/internal/backupflow": {},
		},
		"backupverify": {},
		"restore": {
			modulePath + "/internal/app/operation":            {},
			modulePath + "/internal/app/internal/backupflow":  {},
			modulePath + "/internal/app/internal/restoreflow": {},
		},
		"migrate": {
			modulePath + "/internal/app/operation":            {},
			modulePath + "/internal/app/internal/restoreflow": {},
		},
		"doctor": {},
	}

	for pkgName, allowedImports := range allowed {
		assertAppImportEdges(t, filepath.Join(root, "internal", "app", pkgName), allowedImports)
	}
}

func TestSharedWorkflowKernelImportEdgesAreCanonical(t *testing.T) {
	root := repoRoot(t)
	allowed := map[string]map[string]struct{}{
		filepath.Join(root, "internal", "app", "internal", "backupflow"): {
			modulePath + "/internal/app/operation": {},
		},
		filepath.Join(root, "internal", "app", "internal", "restoreflow"): {
			modulePath + "/internal/app/operation": {},
		},
	}

	for dir, allowedImports := range allowed {
		assertAppImportEdges(t, dir, allowedImports)
	}
}

func listInternalPackages(t *testing.T) []listedPackage {
	t.Helper()

	return listPackages(t, "./internal/...")
}

func assertAppImportEdges(t *testing.T, dir string, allowedImports map[string]struct{}) {
	t.Helper()

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
			if !strings.HasPrefix(importPath, modulePath+"/internal/app/") {
				continue
			}
			if strings.HasPrefix(importPath, modulePath+"/internal/app/ports/") {
				continue
			}
			if _, ok := allowedImports[importPath]; ok {
				continue
			}

			t.Fatalf("%s imports non-canonical app package %s", path, importPath)
		}

		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func listPackages(t *testing.T, pattern string) []listedPackage {
	t.Helper()

	root := repoRoot(t)
	goBin, err := exec.LookPath("go")
	if err != nil {
		t.Fatalf("locate go binary: %v", err)
	}
	cmd := exec.Command(goBin, "list", "-json", pattern)
	cmd.Dir = root

	out, err := cmd.Output()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			t.Fatalf("go list failed: %v\n%s", err, ee.Stderr)
		}
		t.Fatalf("go list failed: %v", err)
	}

	dec := json.NewDecoder(strings.NewReader(string(out)))
	var packages []listedPackage
	for {
		var pkg listedPackage
		if err := dec.Decode(&pkg); err != nil {
			if err == io.EOF {
				break
			}
			t.Fatalf("decode go list output: %v", err)
		}
		packages = append(packages, pkg)
	}

	return packages
}

func productionGoFiles(t *testing.T, dirs ...string) []string {
	t.Helper()

	var files []string
	for _, dir := range dirs {
		err := filepath.WalkDir(dir, func(path string, entry os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if entry.IsDir() {
				if filepath.Base(path) == "testutil" {
					return filepath.SkipDir
				}
				return nil
			}
			if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			files = append(files, path)
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}

	sort.Strings(files)
	return files
}

func repoRoot(t *testing.T) string {
	t.Helper()

	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.mod not found")
		}
		dir = parent
	}
}

func inLayer(importPath, layer string) bool {
	return importPath == modulePath+"/internal/"+layer ||
		strings.HasPrefix(importPath, modulePath+"/internal/"+layer+"/")
}

func assertNoImports(t *testing.T, pkg listedPackage, forbiddenPrefixes []string) {
	t.Helper()

	for _, imp := range pkg.Imports {
		for _, forbidden := range forbiddenPrefixes {
			if strings.HasPrefix(imp, forbidden) {
				t.Fatalf("%s imports forbidden package %s", pkg.ImportPath, imp)
			}
		}
	}
}

func assertNoImportsExcept(t *testing.T, pkg listedPackage, forbiddenPrefixes, allowedPrefixes []string) {
	t.Helper()

allowed:
	for _, imp := range pkg.Imports {
		for _, allowed := range allowedPrefixes {
			if strings.HasPrefix(imp, allowed) {
				continue allowed
			}
		}
		for _, forbidden := range forbiddenPrefixes {
			if strings.HasPrefix(imp, forbidden) {
				t.Fatalf("%s imports forbidden package %s", pkg.ImportPath, imp)
			}
		}
	}
}

func assertStdlibOnly(t *testing.T, pkg listedPackage) {
	t.Helper()

	for _, imp := range pkg.Imports {
		if strings.Contains(imp, ".") {
			t.Fatalf("%s imports non-stdlib package %s", pkg.ImportPath, imp)
		}
	}
}

func receiverBaseName(list *ast.FieldList) string {
	if list == nil || len(list.List) == 0 {
		return ""
	}

	switch typ := list.List[0].Type.(type) {
	case *ast.Ident:
		return typ.Name
	case *ast.StarExpr:
		if ident, ok := typ.X.(*ast.Ident); ok {
			return ident.Name
		}
	}

	return ""
}

func hasName(allowed map[string]struct{}, name string) bool {
	_, ok := allowed[name]
	return ok
}

func hasNestedName(allowed map[string]map[string]struct{}, receiver, name string) bool {
	methods, ok := allowed[receiver]
	if !ok {
		return false
	}
	_, ok = methods[name]
	return ok
}

func exportedSymbolName(fn *ast.FuncDecl) string {
	if fn == nil || fn.Name == nil {
		return ""
	}
	receiver := receiverBaseName(fn.Recv)
	if receiver == "" {
		return fn.Name.Name
	}
	return receiver + "." + fn.Name.Name
}

func sortedStringSet(set map[string]struct{}) []string {
	keys := make([]string, 0, len(set))
	for key := range set {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
