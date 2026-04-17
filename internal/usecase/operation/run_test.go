package operation

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/lazuale/espocrm-ops/internal/contract/result"
	domainjournal "github.com/lazuale/espocrm-ops/internal/domain/journal"
	domainoperation "github.com/lazuale/espocrm-ops/internal/domain/operation"
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

var _ Runtime = fixedRuntime{}

type memoryWriter struct {
	entries []domainjournal.Entry
	err     error
}

func (w *memoryWriter) Write(entry domainjournal.Entry) error {
	w.entries = append(w.entries, entry)
	return w.err
}

func TestExecutionFinishSuccessPopulatesResultAndJournalEntry(t *testing.T) {
	now := time.Date(2026, 4, 15, 12, 0, 0, 0, time.UTC)
	writer := &memoryWriter{}
	exec := Begin(fixedRuntime{now: now, id: "op-test-1"}, writer, "test-command")

	res, err := exec.FinishSuccess(result.Result{
		Message: "ok",
		DryRun:  true,
		Artifacts: struct {
			Manifest string `json:"manifest"`
		}{Manifest: "/tmp/manifest.json"},
		Details: map[string]any{
			"dry_run": true,
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	if res.Command != "test-command" || !res.OK {
		t.Fatalf("unexpected result identity: %#v", res)
	}
	if res.Timing == nil || res.Timing.StartedAt != now.Format(domainoperation.TimeFormat) || res.Timing.FinishedAt != now.Format(domainoperation.TimeFormat) {
		t.Fatalf("unexpected timing: %#v", res.Timing)
	}
	if len(writer.entries) != 1 {
		t.Fatalf("expected one journal entry, got %d", len(writer.entries))
	}
	entry := writer.entries[0]
	if entry.OperationID != "op-test-1" || entry.Command != "test-command" || !entry.OK || !entry.DryRun {
		t.Fatalf("unexpected journal entry: %#v", entry)
	}
	if entry.Artifacts["manifest"] != "/tmp/manifest.json" {
		t.Fatalf("unexpected artifacts: %#v", entry.Artifacts)
	}
	if entry.Details["dry_run"] != true {
		t.Fatalf("unexpected details: %#v", entry.Details)
	}
}

func TestExecutionFinishSuccessPropagatesSerializationWarnings(t *testing.T) {
	writer := &memoryWriter{}
	exec := Begin(fixedRuntime{
		now: time.Date(2026, 4, 15, 12, 0, 0, 0, time.UTC),
		id:  "op-test-2",
	}, writer, "test-command")

	res, err := exec.FinishSuccess(result.Result{
		Message:   "ok",
		Artifacts: func() {},
	})
	if err != nil {
		t.Fatal(err)
	}

	if len(res.Warnings) != 1 || !strings.Contains(res.Warnings[0], "failed to serialize journal artifacts") {
		t.Fatalf("expected serialization warning, got %#v", res.Warnings)
	}
	if len(writer.entries) != 1 {
		t.Fatalf("expected journal write, got %d entries", len(writer.entries))
	}
	if len(writer.entries[0].Warnings) != 1 {
		t.Fatalf("expected warning copied into journal entry, got %#v", writer.entries[0].Warnings)
	}
}

func TestExecutionFinishFailureReturnsJournalWriteError(t *testing.T) {
	wantErr := errors.New("journal down")
	writer := &memoryWriter{err: wantErr}
	exec := Begin(fixedRuntime{
		now: time.Date(2026, 4, 15, 12, 0, 0, 0, time.UTC),
		id:  "op-test-3",
	}, writer, "test-command")

	err := exec.FinishFailure(result.Result{}, errors.New("boom"), "test_failed")
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected journal error, got %v", err)
	}
	if len(writer.entries) != 1 {
		t.Fatalf("expected attempted journal write, got %d entries", len(writer.entries))
	}
	if writer.entries[0].ErrorCode != "test_failed" || writer.entries[0].ErrorMessage != "boom" {
		t.Fatalf("unexpected failure entry: %#v", writer.entries[0])
	}
}
