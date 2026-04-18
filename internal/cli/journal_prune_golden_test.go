package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestGolden_JournalPrune_JSON(t *testing.T) {
	tmp := t.TempDir()
	journalDir := filepath.Join(tmp, "journal")
	if err := os.MkdirAll(journalDir, 0o755); err != nil {
		t.Fatal(err)
	}

	writeJournalEntryFile(t, journalDir, "2026-04-15-a.json", map[string]any{
		"operation_id": "op-prune-golden-1",
		"command":      "verify-backup",
		"started_at":   "2026-04-15T10:00:00Z",
		"finished_at":  "2026-04-15T10:00:01Z",
		"ok":           true,
		"details": map[string]any{
			"scope": "prod",
		},
	})
	writeJournalEntryFile(t, journalDir, "2026-04-15-b.json", map[string]any{
		"operation_id": "op-prune-golden-2",
		"command":      "rollback",
		"started_at":   "2026-04-15T11:00:00Z",
		"finished_at":  "2026-04-15T11:00:03Z",
		"ok":           false,
		"error_code":   "rollback_failed",
		"details": map[string]any{
			"scope":          "prod",
			"selection_mode": "auto_latest_valid",
		},
		"artifacts": map[string]any{
			"selected_prefix": "espocrm-prod",
			"selected_stamp":  "2026-04-15_09-00-00",
		},
		"items": []any{
			map[string]any{
				"code":    "target_selection",
				"status":  "failed",
				"summary": "Automatic rollback target selection failed",
				"action":  "Inspect the manifest set and rerun rollback with an explicit target if needed.",
			},
		},
	})
	writeJournalEntryFile(t, journalDir, "2026-04-15-c.json", map[string]any{
		"operation_id": "op-prune-golden-3",
		"command":      "update",
		"started_at":   "2026-04-15T12:00:00Z",
		"finished_at":  "2026-04-15T12:00:02Z",
		"ok":           true,
		"message":      "update completed",
		"details": map[string]any{
			"scope": "stage",
		},
		"items": []any{
			map[string]any{
				"code":    "runtime_return",
				"status":  "completed",
				"summary": "Contour return completed",
			},
		},
	})

	out, err := runRootCommand(t,
		"--journal-dir", journalDir,
		"--json",
		"journal-prune",
		"--keep-last", "2",
		"--dry-run",
	)
	if err != nil {
		t.Fatalf("command failed: %v\noutput=%s", err, out)
	}

	normalized := normalizeJournalPruneJSON(t, []byte(out))
	assertGoldenJSON(t, normalized, filepath.Join("testdata", "journal_prune_ok.golden.json"))
}

func normalizeJournalPruneJSON(t *testing.T, raw []byte) []byte {
	t.Helper()

	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatalf("invalid json: %v", err)
	}

	items, _ := payload["items"].([]any)
	for _, rawItem := range items {
		item, ok := rawItem.(map[string]any)
		if !ok {
			continue
		}
		operation, _ := item["operation"].(map[string]any)
		operationID, _ := operation["operation_id"].(string)
		switch operationID {
		case "op-prune-golden-1":
			item["path"] = "REPLACE_PATH_OP_PRUNE_GOLDEN_1"
		case "op-prune-golden-2":
			item["path"] = "REPLACE_PATH_OP_PRUNE_GOLDEN_2"
		case "op-prune-golden-3":
			item["path"] = "REPLACE_PATH_OP_PRUNE_GOLDEN_3"
		}
	}

	normalized, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		t.Fatal(err)
	}

	return normalized
}
