package cli

import (
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/lazuale/espocrm-ops/internal/contract/result"
	domainjournal "github.com/lazuale/espocrm-ops/internal/domain/journal"
	"github.com/spf13/cobra"
)

type failingWriter struct{}

func (failingWriter) Write(entry domainjournal.Entry) error {
	return errors.New("journal write failed")
}

type captureWriter struct {
	entries []domainjournal.Entry
}

func (c *captureWriter) Write(entry domainjournal.Entry) error {
	c.entries = append(c.entries, entry)
	return nil
}

func TestRunCommand_JournalWriteFailure_AddsWarning(t *testing.T) {
	opts := []testAppOption{
		withFixedTestRuntime(time.Date(2026, 4, 15, 12, 0, 0, 0, time.UTC), "op-test"),
		withJournalWriterFactory(func(dir string) JournalWriter {
			return failingWriter{}
		}),
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
			Message: "ok message",
		}, nil
	})
	if err != nil {
		t.Fatalf("RunCommand returned error: %v", err)
	}

	got := out.String()
	if !strings.Contains(got, "ok message") {
		t.Fatalf("expected output to contain ok message, got: %s", got)
	}
	if !strings.Contains(got, "failed to write journal entry") {
		t.Fatalf("expected warning about journal write failure, got: %s", got)
	}
}

func TestRunCommand_Error_WritesJournalEntry(t *testing.T) {
	cw := &captureWriter{}
	opts := []testAppOption{
		withFixedTestRuntime(time.Date(2026, 4, 15, 12, 0, 0, 0, time.UTC), "op-test-error"),
		withJournalWriterFactory(func(dir string) JournalWriter {
			return cw
		}),
	}

	cmd := &cobra.Command{}
	bindTestApp(cmd, opts...)
	cmd.SetOut(&strings.Builder{})

	err := RunCommand(cmd, CommandSpec{
		Name:      "restore-db",
		ErrorCode: "restore_db_failed",
		ExitCode:  5,
	}, func() (result.Result, error) {
		return result.Result{}, errors.New("boom")
	})
	if err == nil {
		t.Fatal("expected error")
	}

	if len(cw.entries) != 1 {
		t.Fatalf("expected 1 journal entry, got %d", len(cw.entries))
	}

	entry := cw.entries[0]
	if entry.OK {
		t.Fatalf("expected failed journal entry")
	}
	if entry.OperationID != "op-test-error" {
		t.Fatalf("unexpected operation id: %s", entry.OperationID)
	}
	if entry.Command != "restore-db" {
		t.Fatalf("unexpected command: %s", entry.Command)
	}
	if entry.StartedAt != "2026-04-15T12:00:00Z" || entry.FinishedAt != "2026-04-15T12:00:00Z" {
		t.Fatalf("unexpected timing: started=%s finished=%s", entry.StartedAt, entry.FinishedAt)
	}
	if entry.ErrorCode != "restore_db_failed" {
		t.Fatalf("unexpected error code: %s", entry.ErrorCode)
	}
	if entry.ErrorMessage != "boom" {
		t.Fatalf("unexpected error message: %s", entry.ErrorMessage)
	}
}

func TestRunCommand_SerializesTypedArtifactsAndDetailsIntoJournal(t *testing.T) {
	cw := &captureWriter{}
	opts := []testAppOption{
		withFixedTestRuntime(time.Date(2026, 4, 15, 12, 0, 0, 0, time.UTC), "op-typed-payload"),
		withJournalWriterFactory(func(dir string) JournalWriter {
			return cw
		}),
	}

	cmd := &cobra.Command{}
	bindTestApp(cmd, opts...)
	cmd.SetOut(&strings.Builder{})

	err := RunCommand(cmd, CommandSpec{
		Name:      "verify-backup",
		ErrorCode: "backup_verification_failed",
		ExitCode:  4,
	}, func() (result.Result, error) {
		return result.Result{
			Message: "backup verification passed",
			Artifacts: result.VerifyBackupArtifacts{
				Manifest:    "/tmp/manifest.json",
				DBBackup:    "/tmp/db.sql.gz",
				FilesBackup: "/tmp/files.tar.gz",
			},
			Details: result.VerifyBackupDetails{
				Scope:     "prod",
				CreatedAt: "2026-04-15T11:00:00Z",
			},
			Items: []any{
				result.UpdateItem{
					SectionItem: result.SectionItem{
						Code:    "doctor",
						Status:  "completed",
						Summary: "Doctor completed",
					},
				},
			},
		}, nil
	})
	if err != nil {
		t.Fatalf("RunCommand returned error: %v", err)
	}

	if len(cw.entries) != 1 {
		t.Fatalf("expected 1 journal entry, got %d", len(cw.entries))
	}

	entry := cw.entries[0]
	if entry.Message != "backup verification passed" {
		t.Fatalf("unexpected message: %q", entry.Message)
	}
	if entry.Artifacts["manifest"] != "/tmp/manifest.json" {
		t.Fatalf("unexpected artifacts.manifest: %v", entry.Artifacts["manifest"])
	}
	if entry.Artifacts["db_backup"] != "/tmp/db.sql.gz" {
		t.Fatalf("unexpected artifacts.db_backup: %v", entry.Artifacts["db_backup"])
	}
	if entry.Artifacts["files_backup"] != "/tmp/files.tar.gz" {
		t.Fatalf("unexpected artifacts.files_backup: %v", entry.Artifacts["files_backup"])
	}
	if entry.Details["scope"] != "prod" {
		t.Fatalf("unexpected details.scope: %v", entry.Details["scope"])
	}
	if entry.Details["created_at"] != "2026-04-15T11:00:00Z" {
		t.Fatalf("unexpected details.created_at: %v", entry.Details["created_at"])
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

func TestRunCommand_PayloadSerializationFailure_AddsWarning(t *testing.T) {
	cw := &captureWriter{}
	opts := []testAppOption{
		withFixedTestRuntime(time.Date(2026, 4, 15, 12, 0, 0, 0, time.UTC), "op-bad-payload"),
		withJournalWriterFactory(func(dir string) JournalWriter {
			return cw
		}),
	}

	cmd := &cobra.Command{}
	bindTestApp(cmd, opts...)
	out := &strings.Builder{}
	cmd.SetOut(out)

	err := RunCommand(cmd, CommandSpec{
		Name:      "weird-command",
		ErrorCode: "weird_failed",
		ExitCode:  5,
	}, func() (result.Result, error) {
		ch := make(chan int)
		return result.Result{
			Message: "ok",
			Details: struct {
				Bad any `json:"bad"`
			}{
				Bad: ch,
			},
		}, nil
	})
	if err != nil {
		t.Fatalf("RunCommand returned error: %v", err)
	}

	got := out.String()
	if !strings.Contains(got, "failed to serialize journal details") {
		t.Fatalf("expected serialization warning, got: %s", got)
	}
	if len(cw.entries) != 1 {
		t.Fatalf("expected 1 journal entry, got %d", len(cw.entries))
	}
	if len(cw.entries[0].Warnings) != 1 || !strings.Contains(cw.entries[0].Warnings[0], "failed to serialize journal details") {
		t.Fatalf("expected journal serialization warning, got: %+v", cw.entries[0].Warnings)
	}
	if cw.entries[0].Details != nil {
		t.Fatalf("expected failed details payload to be omitted from journal, got: %+v", cw.entries[0].Details)
	}
}

func TestRunResultCommand_DoesNotWriteJournalEntries(t *testing.T) {
	cw := &captureWriter{}
	opts := []testAppOption{
		withFixedTestRuntime(time.Date(2026, 4, 15, 12, 0, 0, 0, time.UTC), "op-read-only"),
		withJournalWriterFactory(func(dir string) JournalWriter {
			return cw
		}),
	}

	cmd := &cobra.Command{}
	bindTestApp(cmd, opts...)
	cmd.SetOut(&strings.Builder{})

	if err := RunResultCommand(cmd, CommandSpec{
		Name:      "history",
		ErrorCode: "history_failed",
		ExitCode:  1,
	}, func() (result.Result, error) {
		return result.Result{
			OK:      true,
			Message: "history loaded",
		}, nil
	}); err != nil {
		t.Fatalf("RunResultCommand returned success error: %v", err)
	}

	if len(cw.entries) != 0 {
		t.Fatalf("expected no journal entries for non-journaling success, got %d", len(cw.entries))
	}

	err := RunResultCommand(cmd, CommandSpec{
		Name:      "show-operation",
		ErrorCode: "show_operation_failed",
		ExitCode:  1,
	}, func() (result.Result, error) {
		return result.Result{}, errors.New("boom")
	})
	if err == nil {
		t.Fatal("expected error")
	}

	if len(cw.entries) != 0 {
		t.Fatalf("expected no journal entries for non-journaling failure, got %d", len(cw.entries))
	}
}

func TestRunCommand_CustomRenderText_AppendsWarnings(t *testing.T) {
	opts := []testAppOption{
		withFixedTestRuntime(time.Date(2026, 4, 15, 12, 0, 0, 0, time.UTC), "op-custom-render"),
		withJournalWriterFactory(func(dir string) JournalWriter {
			return failingWriter{}
		}),
	}

	cmd := &cobra.Command{}
	bindTestApp(cmd, opts...)
	out := &strings.Builder{}
	cmd.SetOut(out)

	err := RunCommand(cmd, CommandSpec{
		Name:      "backup-audit",
		ErrorCode: "backup_audit_failed",
		ExitCode:  1,
		RenderText: func(w io.Writer, res result.Result) error {
			_, err := fmt.Fprintln(w, "audit summary")
			return err
		},
	}, func() (result.Result, error) {
		return result.Result{
			OK:      true,
			Message: "ignored by custom renderer",
		}, nil
	})
	if err != nil {
		t.Fatalf("RunCommand returned error: %v", err)
	}

	got := out.String()
	if !strings.Contains(got, "audit summary") {
		t.Fatalf("expected custom text output, got: %s", got)
	}
	if !strings.Contains(got, "WARNING: failed to write journal entry") {
		t.Fatalf("expected warning after custom render, got: %s", got)
	}
}
