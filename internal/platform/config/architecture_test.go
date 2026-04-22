package config

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

func TestConfigProductionFileSetStaysBounded(t *testing.T) {
	got := map[string]struct{}{}
	want := map[string]struct{}{
		"config.go":                        {},
		"errors.go":                        {},
		"load.go":                          {},
		"operation_env.go":                 {},
		"operation_env_contour.go":         {},
		"operation_env_errors.go":          {},
		"operation_env_file_validation.go": {},
		"operation_env_parse.go":           {},
		"operation_env_request.go":         {},
	}

	for _, path := range configPackageFiles(t) {
		got[filepath.Base(path)] = struct{}{}
	}

	if len(got) != len(want) {
		t.Fatalf("unexpected config production file set: got %v want %v", configSortedKeys(got), configSortedKeys(want))
	}
	for name := range want {
		if _, ok := got[name]; !ok {
			t.Fatalf("missing config production file %q in %v", name, configSortedKeys(got))
		}
	}
	for name := range got {
		if _, ok := want[name]; !ok {
			t.Fatalf("unexpected config production file %q in %v", name, configSortedKeys(got))
		}
	}
}

func TestConfigAdapterDoesNotReadProcessEnvironment(t *testing.T) {
	for _, forbidden := range []string{
		"os.Getenv(",
		"os.LookupEnv(",
		"os.Environ(",
		"os.Getwd(",
	} {
		assertConfigPackageTextAbsent(t, forbidden)
	}
}

func TestConfigAdapterDoesNotShellOutOrUseShellParsers(t *testing.T) {
	for _, forbidden := range []string{
		"exec.Command(",
		"godotenv",
		"\"sh\"",
		"\"-c\"",
	} {
		assertConfigPackageTextAbsent(t, forbidden)
	}
}

func TestConfigAdapterEnvFileChecksStayFailClosed(t *testing.T) {
	assertConfigPackageTextAbsent(t, "os.Stat(")
}

func TestConfigAdapterDoesNotOwnErrorCodes(t *testing.T) {
	fset := token.NewFileSet()
	methods := map[string]struct{}{}

	for _, path := range configPackageFiles(t) {
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Recv == nil || fn.Name.Name != "ErrorCode" {
				continue
			}
			methods[configReceiverName(fn)] = struct{}{}
		}
	}
	if len(methods) != 0 {
		t.Fatalf("config adapter must not define ErrorCode methods, found: %v", configSortedKeys(methods))
	}
}

func TestConfigExportedSurfaceIsIntentional(t *testing.T) {
	fset := token.NewFileSet()
	got := map[string]struct{}{}
	want := map[string]struct{}{
		"type DBConfig":                    {},
		"type PasswordSourceConflictError": {},
		"type PasswordFileReadError":       {},
		"type PasswordFileEmptyError":      {},
		"type PasswordRequiredError":       {},
		"func ResolveDBPassword":           {},
		"func ResolveDBRootPassword":       {},
		"type MissingEnvFileError":         {},
		"type UnsupportedContourError":     {},
		"type InvalidEnvFileError":         {},
		"type EnvParseError":               {},
		"type MissingEnvValueError":        {},
		"func LoadOperationEnv":            {},
	}

	for _, path := range configPackageFiles(t) {
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			t.Fatal(err)
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
				if typed.Recv == nil && typed.Name.IsExported() {
					got["func "+typed.Name.Name] = struct{}{}
				}
			}
		}
	}

	if len(got) != len(want) {
		t.Fatalf("unexpected config exported surface: got %v want %v", configSortedKeys(got), configSortedKeys(want))
	}
	for name := range want {
		if _, ok := got[name]; !ok {
			t.Fatalf("missing config exported symbol %q in %v", name, configSortedKeys(got))
		}
	}
	for name := range got {
		if _, ok := want[name]; !ok {
			t.Fatalf("unexpected config exported symbol %q in %v", name, configSortedKeys(got))
		}
	}
}

func TestConfigInternalImportsStayExact(t *testing.T) {
	fset := token.NewFileSet()
	allowedInternalByFile := map[string]map[string]struct{}{
		"config.go":                        {},
		"errors.go":                        {},
		"load.go":                          {},
		"operation_env.go":                 {modulePath + "/internal/domain/env": {}},
		"operation_env_contour.go":         {},
		"operation_env_errors.go":          {},
		"operation_env_file_validation.go": {},
		"operation_env_parse.go":           {},
		"operation_env_request.go":         {},
	}

	for _, path := range configPackageFiles(t) {
		fileName := filepath.Base(path)
		allowedInternal, ok := allowedInternalByFile[fileName]
		if !ok {
			t.Fatalf("unexpected config production file %s", fileName)
		}

		file, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
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
			t.Fatalf("%s imports unexpected internal surface: got %v want %v", fileName, configSortedKeys(got), configSortedKeys(allowedInternal))
		}
		for name := range allowedInternal {
			if _, ok := got[name]; !ok {
				t.Fatalf("%s missing internal import %q in %v", fileName, name, configSortedKeys(got))
			}
		}
		for name := range got {
			if _, ok := allowedInternal[name]; !ok {
				t.Fatalf("%s imports unexpected internal package %q in %v", fileName, name, configSortedKeys(got))
			}
		}
	}
}

func TestConfigDefinitionsStayExplicit(t *testing.T) {
	assertConfigTextOwnership(t, "type DBConfig struct", map[string]struct{}{
		"config.go": {},
	})
	assertConfigTextOwnership(t, "type PasswordSourceConflictError struct", map[string]struct{}{
		"errors.go": {},
	})
	assertConfigTextOwnership(t, "type MissingEnvFileError struct", map[string]struct{}{
		"operation_env_errors.go": {},
	})
	assertConfigTextOwnership(t, "func ResolveDBPassword(", map[string]struct{}{
		"load.go": {},
	})
	assertConfigTextOwnership(t, "func ResolveDBRootPassword(", map[string]struct{}{
		"load.go": {},
	})
	assertConfigTextOwnership(t, "func LoadOperationEnv(", map[string]struct{}{
		"operation_env.go": {},
	})
	assertConfigTextOwnership(t, "func resolveLoadedEnvContour(", map[string]struct{}{
		"operation_env_contour.go": {},
	})
	assertConfigTextOwnership(t, "func validateEnvFileForLoading(", map[string]struct{}{
		"operation_env_file_validation.go": {},
	})
	assertConfigTextOwnership(t, "func loadEnvAssignments(", map[string]struct{}{
		"operation_env_parse.go": {},
	})
	assertConfigTextOwnership(t, "func resolveOperationEnvPath(", map[string]struct{}{
		"operation_env_request.go": {},
	})
}

func assertConfigPackageTextAbsent(t *testing.T, needle string) {
	t.Helper()

	for _, path := range configPackageFiles(t) {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(raw), needle) {
			t.Fatalf("config adapter must not contain %s; found in %s", needle, path)
		}
	}
}

func assertConfigTextOwnership(t *testing.T, needle string, owners map[string]struct{}) {
	t.Helper()

	for _, path := range configPackageFiles(t) {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(raw), needle) {
			continue
		}
		if _, ok := owners[filepath.Base(path)]; !ok {
			t.Fatalf("%s must stay owner-local to %v; found in %s", needle, configSortedKeys(owners), path)
		}
	}
}

func configPackageFiles(t *testing.T) []string {
	t.Helper()

	root := testutil.RepoRoot(t)
	configDir := filepath.Join(root, "internal", "platform", "config")
	paths, err := filepath.Glob(filepath.Join(configDir, "*.go"))
	if err != nil {
		t.Fatal(err)
	}

	files := make([]string, 0, len(paths))
	for _, path := range paths {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		files = append(files, path)
	}
	sort.Strings(files)
	return files
}

func configReceiverName(fn *ast.FuncDecl) string {
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

func configSortedKeys(set map[string]struct{}) []string {
	keys := make([]string, 0, len(set))
	for key := range set {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
