package operation

import (
	"errors"
	"strings"
	"testing"
	"time"

	domainjournal "github.com/lazuale/espocrm-ops/internal/domain/journal"
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

	completion, err := exec.FinishSuccess(JournalRecord{
		Message: "ok",
		DryRun:  true,
		Payload: JournalPayload{
			Artifacts: map[string]any{
				"manifest": "/tmp/manifest.json",
			},
			Details: map[string]any{
				"dry_run": true,
			},
			Items: []any{
				map[string]any{
					"code":    "doctor",
					"status":  "completed",
					"summary": "Doctor completed",
				},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	if completion.StartedAt != now.Format(TimeFormat) || completion.FinishedAt != now.Format(TimeFormat) || completion.DurationMS != 0 {
		t.Fatalf("unexpected completion: %#v", completion)
	}
	if len(writer.entries) != 1 {
		t.Fatalf("expected one journal entry, got %d", len(writer.entries))
	}
	entry := writer.entries[0]
	if entry.OperationID != "op-test-1" || entry.Command != "test-command" || !entry.OK || !entry.DryRun {
		t.Fatalf("unexpected journal entry: %#v", entry)
	}
	if entry.Message != "ok" {
		t.Fatalf("unexpected message: %q", entry.Message)
	}
	if entry.Artifacts["manifest"] != "/tmp/manifest.json" {
		t.Fatalf("unexpected artifacts: %#v", entry.Artifacts)
	}
	if entry.Details["dry_run"] != true {
		t.Fatalf("unexpected details: %#v", entry.Details)
	}
	if len(entry.Items) != 1 {
		t.Fatalf("unexpected items: %#v", entry.Items)
	}
	item, ok := entry.Items[0].(map[string]any)
	if !ok {
		t.Fatalf("unexpected item type: %T", entry.Items[0])
	}
	if item["code"] != "doctor" || item["status"] != "completed" {
		t.Fatalf("unexpected item payload: %#v", item)
	}
}

func TestExecutionFinishSuccessPropagatesWarningsToCompletionAndJournalEntry(t *testing.T) {
	writer := &memoryWriter{}
	exec := Begin(fixedRuntime{
		now: time.Date(2026, 4, 15, 12, 0, 0, 0, time.UTC),
		id:  "op-test-2",
	}, writer, "test-command")

	completion, err := exec.FinishSuccess(JournalRecord{
		Message:  "ok",
		Warnings: []string{"payload warning"},
	})
	if err != nil {
		t.Fatal(err)
	}

	if len(completion.Warnings) != 1 || completion.Warnings[0] != "payload warning" {
		t.Fatalf("expected propagated warning, got %#v", completion.Warnings)
	}
	if len(writer.entries) != 1 {
		t.Fatalf("expected journal write, got %d entries", len(writer.entries))
	}
	if len(writer.entries[0].Warnings) != 1 || writer.entries[0].Warnings[0] != "payload warning" {
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

	err := exec.FinishFailure(JournalRecord{}, errors.New("boom"), "test_failed")
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

func TestExecutionFinishSuccessAppendsJournalWriteWarning(t *testing.T) {
	writer := &memoryWriter{err: errors.New("journal down")}
	exec := Begin(fixedRuntime{
		now: time.Date(2026, 4, 15, 12, 0, 0, 0, time.UTC),
		id:  "op-test-4",
	}, writer, "test-command")

	completion, err := exec.FinishSuccess(JournalRecord{Warnings: []string{"existing"}})
	if err != nil {
		t.Fatal(err)
	}

	if len(completion.Warnings) != 2 {
		t.Fatalf("expected existing warning plus journal warning, got %#v", completion.Warnings)
	}
	if completion.Warnings[0] != "existing" {
		t.Fatalf("expected existing warning first, got %#v", completion.Warnings)
	}
	if !strings.Contains(completion.Warnings[1], "failed to write journal entry") {
		t.Fatalf("expected journal write warning, got %#v", completion.Warnings)
	}
}
