package cli

import (
	"strings"
	"testing"

	"github.com/lazuale/espocrm-ops/internal/contract/result"
)

func TestJournalRecordFromResultSerializesPayload(t *testing.T) {
	res := result.Result{
		Message: "ok",
		DryRun:  true,
		Artifacts: struct {
			Manifest string `json:"manifest"`
		}{Manifest: "/tmp/manifest.json"},
		Details: map[string]any{
			"scope": "dev",
		},
		Items: []any{
			struct {
				Code string `json:"code"`
			}{Code: "backup"},
		},
		Warnings: []string{"existing"},
	}

	record := journalRecordFromResult(&res)

	if record.Message != "ok" || !record.DryRun {
		t.Fatalf("unexpected journal record identity: %#v", record)
	}
	if record.Payload.Artifacts["manifest"] != "/tmp/manifest.json" {
		t.Fatalf("unexpected artifacts: %#v", record.Payload.Artifacts)
	}
	if record.Payload.Details["scope"] != "dev" {
		t.Fatalf("unexpected details: %#v", record.Payload.Details)
	}
	if len(record.Payload.Items) != 1 {
		t.Fatalf("unexpected items: %#v", record.Payload.Items)
	}
	item, ok := record.Payload.Items[0].(map[string]any)
	if !ok || item["code"] != "backup" {
		t.Fatalf("unexpected item payload: %#v", record.Payload.Items)
	}
	if len(record.Warnings) != 1 || record.Warnings[0] != "existing" {
		t.Fatalf("unexpected warnings: %#v", record.Warnings)
	}
}

func TestJournalRecordFromResultAppendsSerializationWarnings(t *testing.T) {
	res := result.Result{
		Artifacts: func() {},
	}

	record := journalRecordFromResult(&res)

	if len(res.Warnings) != 1 || !strings.Contains(res.Warnings[0], "failed to serialize journal artifacts") {
		t.Fatalf("expected serialization warning on result, got %#v", res.Warnings)
	}
	if len(record.Warnings) != 1 || !strings.Contains(record.Warnings[0], "failed to serialize journal artifacts") {
		t.Fatalf("expected serialization warning on journal record, got %#v", record.Warnings)
	}
}
