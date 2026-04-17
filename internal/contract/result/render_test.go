package result

import (
	"bytes"
	"encoding/json"
	"testing"
)

func TestRenderTextSuccess(t *testing.T) {
	var buf bytes.Buffer

	err := Render(&buf, Result{
		Command: "verify-backup",
		OK:      true,
		Message: "backup verification passed",
	}, false)
	if err != nil {
		t.Fatalf("Render failed: %v", err)
	}

	if got, want := buf.String(), "backup verification passed\n"; got != want {
		t.Fatalf("unexpected render output: got %q want %q", got, want)
	}
}

func TestRenderTextSuccessWithWarnings(t *testing.T) {
	var buf bytes.Buffer

	err := Render(&buf, Result{
		Command:  "restore-files",
		OK:       true,
		Message:  "files restore completed",
		Warnings: []string{"files restore is destructive for the target directory"},
	}, false)
	if err != nil {
		t.Fatalf("Render failed: %v", err)
	}

	want := "files restore completed\nWARNING: files restore is destructive for the target directory\n"
	if got := buf.String(); got != want {
		t.Fatalf("unexpected render output: got %q want %q", got, want)
	}
}

func TestRenderJSON(t *testing.T) {
	var buf bytes.Buffer

	err := Render(&buf, Result{
		Command: "restore-files",
		OK:      true,
		Message: "files restore dry-run passed",
		Details: RestoreFilesDetails{
			DryRun: true,
		},
		Timing: &TimingInfo{
			StartedAt:  "2026-04-15T10:00:00Z",
			FinishedAt: "2026-04-15T10:00:01Z",
			DurationMS: 1000,
		},
	}, true)
	if err != nil {
		t.Fatalf("Render failed: %v", err)
	}

	var decoded Result
	if err := json.Unmarshal(buf.Bytes(), &decoded); err != nil {
		t.Fatalf("invalid JSON output: %v", err)
	}
	if decoded.Command != "restore-files" || !decoded.OK || decoded.Message != "files restore dry-run passed" {
		t.Fatalf("unexpected decoded result: %+v", decoded)
	}
	if decoded.Timing == nil || decoded.Timing.DurationMS != 1000 {
		t.Fatalf("expected timing in decoded result, got: %+v", decoded.Timing)
	}
}
