package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	operationtrace "github.com/lazuale/espocrm-ops/internal/app/operationtrace"
	lockport "github.com/lazuale/espocrm-ops/internal/app/ports/lockport"
	appadapter "github.com/lazuale/espocrm-ops/internal/platform/appadapter"
	"github.com/lazuale/espocrm-ops/internal/platform/journalstore"
	"github.com/spf13/cobra"
)

type fixedRuntime struct {
	now time.Time
	id  string
}

func (f fixedRuntime) Now() time.Time {
	return f.now
}

func (f fixedRuntime) NewOperationID() string {
	return f.id
}

var _ operationtrace.Runtime = fixedRuntime{}

type testAppConfig struct {
	runtime              operationtrace.Runtime
	journalWriterFactory JournalWriterFactory
	locks                lockport.Locks
	options              GlobalOptions
}

type testAppOption func(*testAppConfig)

func defaultTestAppConfig() testAppConfig {
	return testAppConfig{
		runtime: operationtrace.DefaultRuntime{},
		journalWriterFactory: func(dir string) JournalWriter {
			return journalstore.FSWriter{Dir: dir}
		},
		options: defaultGlobalOptions(),
	}
}

func withTestRuntime(runtime operationtrace.Runtime) testAppOption {
	return func(cfg *testAppConfig) {
		cfg.runtime = runtime
	}
}

func withFixedTestRuntime(now time.Time, id string) testAppOption {
	return withTestRuntime(fixedRuntime{
		now: now,
		id:  id,
	})
}

func withJournalWriterFactory(factory JournalWriterFactory) testAppOption {
	return func(cfg *testAppConfig) {
		cfg.journalWriterFactory = factory
	}
}

func withJSONOutput() testAppOption {
	return func(cfg *testAppConfig) {
		cfg.options.JSON = true
	}
}

func withRestoreLockDir(dir string) testAppOption {
	return func(cfg *testAppConfig) {
		cfg.locks = appadapter.Locks{RestoreLockDir: dir}
	}
}

func runRootCommand(t *testing.T, args ...string) (string, error) {
	return runRootCommandWithOptions(t, nil, args...)
}

func runRootCommandWithOptions(t *testing.T, opts []testAppOption, args ...string) (string, error) {
	t.Helper()

	root := newTestRootCmd(opts...)
	out := &bytes.Buffer{}
	errBuf := &bytes.Buffer{}

	root.SetOut(out)
	root.SetErr(errBuf)
	root.SetArgs(args)

	err := root.Execute()
	return out.String(), err
}

func newTestApp(opts ...testAppOption) *App {
	cfg := defaultTestAppConfig()
	for _, opt := range opts {
		opt(&cfg)
	}

	app := NewApp(Dependencies{
		Runtime:              cfg.runtime,
		JournalWriterFactory: cfg.journalWriterFactory,
		Locks:                cfg.locks,
	})
	app.options = cfg.options
	return app
}

func newTestRootCmd(opts ...testAppOption) *cobra.Command {
	return newTestApp(opts...).NewRootCmd()
}

func bindTestApp(cmd *cobra.Command, opts ...testAppOption) *cobra.Command {
	return bindApp(cmd, newTestApp(opts...))
}

func assertGoldenJSON(t *testing.T, got []byte, goldenPath string) {
	t.Helper()

	var gotObj any
	if err := json.Unmarshal(got, &gotObj); err != nil {
		t.Fatalf("invalid json output: %v\n%s", err, string(got))
	}
	assertNoLegacyWorkflowVocabularyInJSON(t, gotObj)

	want, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("read golden: %v", err)
	}

	var wantObj any
	if err := json.Unmarshal(want, &wantObj); err != nil {
		t.Fatalf("invalid golden json: %v", err)
	}

	gotNorm, err := json.MarshalIndent(gotObj, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	wantNorm, err := json.MarshalIndent(wantObj, "", "  ")
	if err != nil {
		t.Fatal(err)
	}

	if string(gotNorm) != string(wantNorm) {
		t.Fatalf("golden mismatch\nGOT:\n%s\n\nWANT:\n%s", gotNorm, wantNorm)
	}
}

func assertNoLegacyWorkflowVocabularyInJSON(t *testing.T, v any) {
	t.Helper()
	assertNoLegacyWorkflowVocabularyAtPath(t, v, "$")
}

func assertNoLegacyWorkflowVocabularyAtPath(t *testing.T, v any, path string) {
	t.Helper()

	switch typed := v.(type) {
	case map[string]any:
		for key, value := range typed {
			if key == "would_run" || key == "not_run" {
				t.Fatalf("legacy workflow key %q found at %s", key, path)
			}
			assertNoLegacyWorkflowVocabularyAtPath(t, value, path+"."+key)
		}
	case []any:
		for i, value := range typed {
			assertNoLegacyWorkflowVocabularyAtPath(t, value, path+"["+strconv.Itoa(i)+"]")
		}
	case string:
		if typed == "would_run" || typed == "not_run" {
			t.Fatalf("legacy workflow value %q found at %s", typed, path)
		}
	}
}

func assertUsageErrorOutput(t *testing.T, outcome execOutcome, messagePart string) {
	t.Helper()

	assertCLIErrorOutput(t, outcome, 2, "usage_error", messagePart)
}

func assertCLIErrorOutput(t *testing.T, outcome execOutcome, wantExitCode int, wantErrorCode, messagePart string) {
	t.Helper()

	if outcome.ExitCode != wantExitCode {
		t.Fatalf("expected exit code %d, got %d stdout=%s stderr=%s", wantExitCode, outcome.ExitCode, outcome.Stdout, outcome.Stderr)
	}

	var obj map[string]any
	if err := json.Unmarshal([]byte(outcome.Stdout), &obj); err != nil {
		t.Fatal(err)
	}

	requireJSONPath(t, obj, "command")
	requireJSONPath(t, obj, "ok")
	requireJSONPath(t, obj, "error", "code")
	requireJSONPath(t, obj, "error", "exit_code")
	requireJSONPath(t, obj, "error", "message")

	if ok, _ := obj["ok"].(bool); ok {
		t.Fatalf("expected ok=false")
	}
	if code := requireJSONPath(t, obj, "error", "code"); code != wantErrorCode {
		t.Fatalf("expected %s, got %v", wantErrorCode, code)
	}
	exitCode, _ := requireJSONPath(t, obj, "error", "exit_code").(float64)
	if int(exitCode) != wantExitCode {
		t.Fatalf("expected json error exit_code %d, got %v", wantExitCode, exitCode)
	}
	message, _ := requireJSONPath(t, obj, "error", "message").(string)
	if !strings.Contains(message, messagePart) {
		t.Fatalf("expected error message to contain %q, got %q", messagePart, message)
	}
}

func assertNoJournalFiles(t *testing.T, journalDir string) {
	t.Helper()

	var paths []string
	err := filepath.WalkDir(journalDir, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() && filepath.Ext(entry.Name()) == ".json" {
			paths = append(paths, path)
		}
		return nil
	})
	if os.IsNotExist(err) {
		return
	}
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) != 0 {
		t.Fatalf("expected no journal files, got %+v", paths)
	}
}

func closeCLIArchiveResource(t *testing.T, label string, closer interface{ Close() error }) {
	t.Helper()

	if err := closer.Close(); err != nil {
		t.Fatalf("close %s: %v", label, err)
	}
}

func replaceKnownPaths(text string, replacements map[string]string) string {
	keys := make([]string, 0, len(replacements))
	for key := range replacements {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		return len(keys[i]) > len(keys[j])
	})

	out := text
	for _, key := range keys {
		out = strings.ReplaceAll(out, key, replacements[key])
	}

	return out
}

func parseCLIJSONBytes(t *testing.T, raw []byte) map[string]any {
	t.Helper()

	var obj map[string]any
	if err := json.Unmarshal(raw, &obj); err != nil {
		t.Fatalf("invalid json output: %v\n%s", err, string(raw))
	}

	return obj
}

func normalizeArtifactPlaceholders(obj map[string]any, placeholders map[string]string) map[string]string {
	replacements := map[string]string{}
	artifacts, ok := obj["artifacts"].(map[string]any)
	if !ok {
		return replacements
	}

	for key, placeholder := range placeholders {
		value, ok := artifacts[key].(string)
		if !ok || value == "" {
			continue
		}
		replacements[value] = placeholder
		artifacts[key] = placeholder
	}

	return replacements
}

func normalizeWarningsPaths(obj map[string]any, replacements map[string]string) {
	warnings, ok := obj["warnings"].([]any)
	if !ok {
		return
	}

	for idx, rawWarning := range warnings {
		warning, ok := rawWarning.(string)
		if !ok {
			continue
		}
		warnings[idx] = replaceKnownPaths(warning, replacements)
	}
}

func normalizeItemStringFields(obj map[string]any, replacements map[string]string, fields ...string) {
	items, ok := obj["items"].([]any)
	if !ok {
		return
	}

	for _, rawItem := range items {
		item, ok := rawItem.(map[string]any)
		if !ok {
			continue
		}
		for _, field := range fields {
			value, ok := item[field].(string)
			if !ok {
				continue
			}
			item[field] = replaceKnownPaths(value, replacements)
		}
	}
}

func marshalCLIJSON(t *testing.T, obj map[string]any) []byte {
	t.Helper()

	out, err := json.Marshal(obj)
	if err != nil {
		t.Fatal(err)
	}

	return out
}
