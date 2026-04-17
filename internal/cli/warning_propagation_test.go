package cli

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/lazuale/espocrm-ops/internal/contract/result"
	domainjournal "github.com/lazuale/espocrm-ops/internal/domain/journal"
	"github.com/spf13/cobra"
)

type jsonWarningFailingWriter struct{}

func (jsonWarningFailingWriter) Write(entry domainjournal.Entry) error {
	return errors.New("journal write failed")
}

func TestWarnings_Propagate_ToJSON(t *testing.T) {
	opts := []testAppOption{
		withFixedTestRuntime(time.Date(2026, 4, 15, 12, 0, 0, 0, time.UTC), "op-warning-1"),
		withJournalWriterFactory(func(dir string) JournalWriter {
			return jsonWarningFailingWriter{}
		}),
		withJSONOutput(),
	}

	cmd := &cobra.Command{}
	bindTestApp(cmd, opts...)
	out := &strings.Builder{}
	cmd.SetOut(out)

	err := RunCommand(cmd, CommandSpec{
		Name:      "test-command",
		ErrorCode: "test_failed",
		ExitCode:  5,
	}, func() (result.Result, error) {
		return result.Result{
			Message: "ok",
		}, nil
	})
	if err != nil {
		t.Fatalf("RunCommand returned error: %v", err)
	}

	var obj map[string]any
	if err := json.Unmarshal([]byte(out.String()), &obj); err != nil {
		t.Fatal(err)
	}

	warningsAny := requireJSONPath(t, obj, "warnings")
	warnings, ok := warningsAny.([]any)
	if !ok || len(warnings) == 0 {
		t.Fatalf("expected warnings in json, got %#v", warningsAny)
	}
}
