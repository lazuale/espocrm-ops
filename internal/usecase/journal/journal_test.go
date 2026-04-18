package journal

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/lazuale/espocrm-ops/internal/contract/apperr"
	domainjournal "github.com/lazuale/espocrm-ops/internal/domain/journal"
	"github.com/lazuale/espocrm-ops/internal/platform/journalstore"
)

func TestHistoryAppliesFiltersAndReturnsReadStats(t *testing.T) {
	dir := t.TempDir()
	writeEntry(t, dir, Entry{
		OperationID: "op-new-ok",
		Command:     "backup",
		StartedAt:   "2026-04-15T12:00:00Z",
		FinishedAt:  "2026-04-15T12:00:01Z",
		OK:          true,
	})
	writeEntry(t, dir, Entry{
		OperationID: "op-old-fail",
		Command:     "restore-db",
		StartedAt:   "2026-04-15T11:00:00Z",
		FinishedAt:  "2026-04-15T11:00:01Z",
		OK:          false,
	})
	writeRaw(t, filepath.Join(dir, "corrupt.json"), "{not-json")

	out, err := History(HistoryInput{
		JournalDir: dir,
		Filters: Filters{
			FailedOnly: true,
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	if out.Stats.TotalFilesSeen != 3 || out.Stats.LoadedEntries != 2 || out.Stats.SkippedCorrupt != 1 {
		t.Fatalf("unexpected stats: %#v", out.Stats)
	}
	if len(out.Operations) != 1 || out.Operations[0].OperationID != "op-old-fail" {
		t.Fatalf("unexpected filtered operations: %#v", out.Operations)
	}
	if out.Operations[0].Status != OperationStatusFailed {
		t.Fatalf("expected failed status, got %#v", out.Operations[0])
	}
}

func TestHistoryProjectsCanonicalSummariesAndAppliesDerivedFilters(t *testing.T) {
	dir := t.TempDir()
	writeEntry(t, dir, Entry{
		OperationID: "op-rollback-recovery",
		Command:     "rollback",
		StartedAt:   "2026-04-15T12:00:00Z",
		FinishedAt:  "2026-04-15T12:00:05Z",
		OK:          true,
		Message:     "rollback recovery completed",
		Artifacts: map[string]any{
			"selected_prefix": "espocrm-prod",
			"selected_stamp":  "2026-04-15_11-00-00",
		},
		Details: map[string]any{
			"scope":          "prod",
			"selection_mode": "auto_latest_valid",
			"recovery": map[string]any{
				"source_operation_id": "op-source",
				"requested_mode":      RecoveryModeAuto,
				"applied_mode":        RecoveryModeResume,
				"decision":            RecoveryDecisionResumeFromCheckpoint,
				"resume_step":         "runtime_prepare",
			},
		},
		Items: []any{
			map[string]any{
				"code":    "runtime_return",
				"status":  "completed",
				"summary": "Contour return completed",
			},
		},
		Warnings: []string{"final probe retried once"},
	})
	writeEntry(t, dir, Entry{
		OperationID: "op-update-plan",
		Command:     "update",
		StartedAt:   "2026-04-15T11:00:00Z",
		FinishedAt:  "2026-04-15T11:00:01Z",
		OK:          false,
		DryRun:      true,
		Message:     "update plan blocked",
		Details: map[string]any{
			"scope": "stage",
		},
		Items: []any{
			map[string]any{
				"code":    "doctor",
				"status":  "blocked",
				"summary": "Doctor is blocked",
			},
		},
	})

	out, err := History(HistoryInput{
		JournalDir: dir,
		Filters: Filters{
			Status:       OperationStatusCompleted,
			Scope:        "prod",
			RecoveryOnly: true,
			TargetPrefix: "espocrm-prod",
			Limit:        1,
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	if len(out.Operations) != 1 {
		t.Fatalf("expected one filtered operation, got %#v", out.Operations)
	}
	operation := out.Operations[0]
	if operation.OperationID != "op-rollback-recovery" {
		t.Fatalf("unexpected operation: %#v", operation)
	}
	if !operation.RecoveryRun || operation.RecoveryAttempt == nil {
		t.Fatalf("expected recovery metadata, got %#v", operation)
	}
	if operation.RecoveryAttempt.SourceOperationID != "op-source" {
		t.Fatalf("unexpected recovery source: %#v", operation.RecoveryAttempt)
	}
	if operation.Target == nil || operation.Target.Prefix != "espocrm-prod" {
		t.Fatalf("unexpected target summary: %#v", operation.Target)
	}
	if operation.WarningCount != 1 {
		t.Fatalf("expected warning count, got %#v", operation)
	}
}

func TestLastOperationReturnsNewestMatchingCommand(t *testing.T) {
	dir := t.TempDir()
	writeEntry(t, dir, Entry{
		OperationID: "op-restore-old",
		Command:     "restore-db",
		StartedAt:   "2026-04-15T10:00:00Z",
		FinishedAt:  "2026-04-15T10:00:01Z",
		OK:          true,
	})
	writeEntry(t, dir, Entry{
		OperationID: "op-backup-new",
		Command:     "backup",
		StartedAt:   "2026-04-15T12:00:00Z",
		FinishedAt:  "2026-04-15T12:00:01Z",
		OK:          true,
	})
	writeEntry(t, dir, Entry{
		OperationID: "op-restore-new",
		Command:     "restore-db",
		StartedAt:   "2026-04-15T11:00:00Z",
		FinishedAt:  "2026-04-15T11:00:01Z",
		OK:          true,
	})

	out, err := LastOperation(LastOperationInput{
		JournalDir: dir,
		Command:    "restore-db",
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.Entry == nil || out.Entry.OperationID != "op-restore-new" {
		t.Fatalf("unexpected last operation: %#v", out.Entry)
	}
}

func TestShowOperationFindsByID(t *testing.T) {
	dir := t.TempDir()
	writeEntry(t, dir, Entry{
		OperationID: "op-target",
		Command:     "verify-backup",
		StartedAt:   "2026-04-15T12:00:00Z",
		FinishedAt:  "2026-04-15T12:00:01Z",
		OK:          true,
	})

	out, err := ShowOperation(ShowOperationInput{
		JournalDir: dir,
		ID:         "op-target",
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.Entry.Command != "verify-backup" {
		t.Fatalf("unexpected entry: %#v", out.Entry)
	}

	if _, err := ShowOperation(ShowOperationInput{JournalDir: dir, ID: "missing"}); err == nil {
		t.Fatal("expected missing operation error")
	} else {
		var notFound NotFoundError
		if !errors.As(err, &notFound) {
			t.Fatalf("expected NotFoundError, got %T: %v", err, err)
		}
		if kind, ok := apperr.KindOf(err); !ok || kind != apperr.KindNotFound {
			t.Fatalf("expected not_found kind, got %s ok=%t", kind, ok)
		}
	}
}

func TestPruneDryRunMapsResultWithoutDeletingFiles(t *testing.T) {
	dir := t.TempDir()
	writeEntry(t, dir, Entry{
		OperationID: "op-1",
		Command:     "backup",
		StartedAt:   "2026-04-15T10:00:00Z",
		FinishedAt:  "2026-04-15T10:00:01Z",
		OK:          true,
	})
	writeEntry(t, dir, Entry{
		OperationID: "op-2",
		Command:     "backup",
		StartedAt:   "2026-04-15T11:00:00Z",
		FinishedAt:  "2026-04-15T11:00:01Z",
		OK:          true,
	})
	writeEntry(t, dir, Entry{
		OperationID: "op-3",
		Command:     "backup",
		StartedAt:   "2026-04-15T12:00:00Z",
		FinishedAt:  "2026-04-15T12:00:01Z",
		OK:          true,
	})

	out, err := Prune(PruneInput{
		JournalDir: dir,
		Keep:       1,
		DryRun:     true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.Checked != 3 || out.Deleted != 2 || len(out.Paths) != 2 {
		t.Fatalf("unexpected prune output: %#v", out)
	}
	for _, id := range []string{"op-1", "op-2", "op-3"} {
		if _, err := os.Stat(filepath.Join(dir, id+".json")); err != nil {
			t.Fatalf("dry-run should not delete %s: %v", id, err)
		}
	}
}

func writeEntry(t *testing.T, dir string, entry Entry) {
	t.Helper()

	if err := (journalstore.FSWriter{Dir: dir}).Write(domainjournal.Entry(entry)); err != nil {
		t.Fatal(err)
	}
}

func writeRaw(t *testing.T, path, raw string) {
	t.Helper()

	if err := os.WriteFile(path, []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}
}
