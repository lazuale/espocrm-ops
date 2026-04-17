package journalstore

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	domainjournal "github.com/lazuale/espocrm-ops/internal/domain/journal"
)

func TestFSWriter_Write(t *testing.T) {
	tmp := t.TempDir()

	entry := domainjournal.Entry{
		OperationID: "op-20260415T120000Z-deadbeef",
		Command:     "verify-backup",
		StartedAt:   "2026-04-15T12:00:00Z",
		FinishedAt:  "2026-04-15T12:00:01Z",
		OK:          true,
		Artifacts: map[string]any{
			"manifest": "/backups/manifest.json",
		},
	}

	if err := (FSWriter{Dir: tmp}).Write(entry); err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	raw, err := os.ReadFile(filepath.Join(tmp, entry.OperationID+".json"))
	if err != nil {
		t.Fatal(err)
	}

	var got domainjournal.Entry
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("invalid journal JSON: %v", err)
	}
	if got.OperationID != entry.OperationID || got.Command != entry.Command || !got.OK {
		t.Fatalf("unexpected journal entry: %+v", got)
	}
}
