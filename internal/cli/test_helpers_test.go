package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/lazuale/espocrm-ops/internal/platform/journalstore"
	backupusecase "github.com/lazuale/espocrm-ops/internal/usecase/backup"
	operationusecase "github.com/lazuale/espocrm-ops/internal/usecase/operation"
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

var _ operationusecase.Runtime = fixedRuntime{}

type testAppConfig struct {
	runtime              operationusecase.Runtime
	journalWriterFactory JournalWriterFactory
	options              GlobalOptions
}

type testAppOption func(*testAppConfig)

func defaultTestAppConfig() testAppConfig {
	return testAppConfig{
		runtime: operationusecase.DefaultRuntime{},
		journalWriterFactory: func(dir string) JournalWriter {
			return journalstore.FSWriter{Dir: dir}
		},
		options: defaultGlobalOptions(),
	}
}

func withTestRuntime(runtime operationusecase.Runtime) testAppOption {
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

func useJournalClockForTest(t *testing.T, now time.Time) {
	t.Helper()

	restore := backupusecase.SetNowForTest(func() time.Time { return now })
	t.Cleanup(restore)
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
