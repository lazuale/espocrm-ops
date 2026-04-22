package journalbridge

import (
	"testing"

	"github.com/lazuale/espocrm-ops/internal/contract/result"
)

func TestRecordFromResultSerializesPayload(t *testing.T) {
	res := result.Result{
		Message: "ok",
		DryRun:  true,
		Artifacts: result.BackupVerifyArtifacts{
			Manifest: "/tmp/manifest.json",
		},
		Details: result.BackupVerifyDetails{
			Scope: "dev",
		},
		Items: []result.ItemPayload{
			result.BackupItem{
				SectionItem: result.SectionItem{
					Code: "backup",
				},
			},
		},
		Warnings: []string{"existing"},
	}

	record := RecordFromResult(&res)

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
