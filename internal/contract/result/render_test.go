package result

import (
	"bytes"
	"encoding/json"
	"testing"
)

func TestRenderTextSuccess(t *testing.T) {
	var buf bytes.Buffer

	err := Render(&buf, Result{
		Command: "backup verify",
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
		Command:  "restore",
		OK:       true,
		Message:  "restore completed",
		Warnings: []string{"restore is destructive for the target contour"},
	}, false)
	if err != nil {
		t.Fatalf("Render failed: %v", err)
	}

	want := "restore completed\nWARNING: restore is destructive for the target contour\n"
	if got := buf.String(); got != want {
		t.Fatalf("unexpected render output: got %q want %q", got, want)
	}
}

func TestRenderJSON(t *testing.T) {
	var buf bytes.Buffer

	err := Render(&buf, Result{
		Command: "restore",
		OK:      true,
		Message: "restore dry-run passed",
		Details: RestoreDetails{
			Ready:     true,
			Scope:     "dev",
			SourceKind:"manifest",
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
	if decoded.Command != "restore" || !decoded.OK || decoded.Message != "restore dry-run passed" {
		t.Fatalf("unexpected decoded result: %+v", decoded)
	}
	if decoded.Timing == nil || decoded.Timing.DurationMS != 1000 {
		t.Fatalf("expected timing in decoded result, got: %+v", decoded.Timing)
	}
}
