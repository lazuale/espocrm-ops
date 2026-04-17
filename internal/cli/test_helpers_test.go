package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/lazuale/espocrm-ops/internal/domain/operation"
	platformclock "github.com/lazuale/espocrm-ops/internal/platform/clock"
	"github.com/lazuale/espocrm-ops/internal/platform/journalstore"
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

var _ operation.Runtime = fixedRuntime{}

type fixedClock struct {
	now time.Time
}

func (f fixedClock) Now() time.Time {
	return f.now
}

var _ platformclock.Clock = fixedClock{}

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

	restore := platformclock.SetForTest(fixedClock{now: now})
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

func writeJournalEntryFile(t *testing.T, dir, name string, entry any) {
	t.Helper()

	raw, err := json.MarshalIndent(entry, "", "  ")
	if err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(dir, name), raw, 0o644); err != nil {
		t.Fatal(err)
	}
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
