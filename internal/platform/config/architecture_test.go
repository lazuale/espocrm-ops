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
	root := testutil.RepoRoot(t)
	configDir := filepath.Join(root, "internal", "platform", "config")
	fset := token.NewFileSet()
	methods := map[string]struct{}{}

	err := filepath.WalkDir(configDir, func(path string, entry os.DirEntry, err error) error {
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
			methods[configReceiverName(fn)] = struct{}{}
		}

		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(methods) != 0 {
		t.Fatalf("config adapter must not define ErrorCode methods, found: %v", configSortedKeys(methods))
	}
}

func assertConfigPackageTextAbsent(t *testing.T, needle string) {
	t.Helper()

	root := testutil.RepoRoot(t)
	configDir := filepath.Join(root, "internal", "platform", "config")

	err := filepath.WalkDir(configDir, func(path string, entry os.DirEntry, err error) error {
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
		if strings.Contains(string(raw), needle) {
			t.Fatalf("config adapter must not contain %s; found in %s", needle, path)
		}

		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
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
