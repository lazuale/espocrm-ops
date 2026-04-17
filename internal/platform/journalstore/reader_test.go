package journalstore

import (
	"os"
	"path/filepath"
	"testing"

	domainjournal "github.com/lazuale/espocrm-ops/internal/domain/journal"
)

func TestReader_ReadAllSortsNewestFirstByStartedAtAndSkipsInvalidJSON(t *testing.T) {
	tmp := t.TempDir()
	writer := FSWriter{Dir: tmp}

	oldEntry := domainjournal.Entry{
		OperationID: "op-20260415T120000Z-11111111",
		Command:     "verify-backup",
		StartedAt:   "2026-04-15T12:00:00Z",
		OK:          true,
	}
	newEntry := domainjournal.Entry{
		OperationID: "op-20260415T130000Z-22222222",
		Command:     "restore-files",
		StartedAt:   "2026-04-15T13:00:00Z",
		OK:          false,
		ErrorCode:   "restore_files_failed",
	}

	if err := writer.Write(oldEntry); err != nil {
		t.Fatal(err)
	}
	if err := writer.Write(newEntry); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmp, "op-20260415T140000Z-bad.json"), []byte("{bad"), 0o644); err != nil {
		t.Fatal(err)
	}

	entries, stats, err := (Reader{Dir: tmp}).ReadAll()
	if err != nil {
		t.Fatalf("ReadAll failed: %v", err)
	}
	if stats.TotalFilesSeen != 3 || stats.LoadedEntries != 2 || stats.SkippedCorrupt != 1 {
		t.Fatalf("unexpected read stats: %+v", stats)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 readable entries, got %d", len(entries))
	}
	if entries[0].OperationID != newEntry.OperationID || entries[1].OperationID != oldEntry.OperationID {
		t.Fatalf("entries are not newest-first: %+v", entries)
	}
}

func TestReader_ReadAllWithPaths(t *testing.T) {
	tmp := t.TempDir()
	entry := domainjournal.Entry{
		OperationID: "op-20260415T120000Z-11111111",
		Command:     "verify-backup",
		StartedAt:   "2026-04-15T12:00:00Z",
		OK:          true,
	}
	if err := (FSWriter{Dir: tmp}).Write(entry); err != nil {
		t.Fatal(err)
	}

	items, stats, err := (Reader{Dir: tmp}).ReadAllWithPaths()
	if err != nil {
		t.Fatalf("ReadAllWithPaths failed: %v", err)
	}
	if stats.TotalFilesSeen != 1 || stats.LoadedEntries != 1 || stats.SkippedCorrupt != 0 {
		t.Fatalf("unexpected read stats: %+v", stats)
	}
	if len(items) != 1 {
		t.Fatalf("expected one item, got: %+v", items)
	}
	if items[0].Entry.OperationID != entry.OperationID {
		t.Fatalf("unexpected entry: %+v", items[0].Entry)
	}
	if filepath.Base(items[0].Path) != entry.OperationID+".json" {
		t.Fatalf("unexpected path: %s", items[0].Path)
	}
}

func TestReader_ReadByID(t *testing.T) {
	tmp := t.TempDir()
	entry := domainjournal.Entry{
		OperationID: "op-20260415T120000Z-11111111",
		Command:     "verify-backup",
		StartedAt:   "2026-04-15T12:00:00Z",
		OK:          true,
	}

	if err := (FSWriter{Dir: tmp}).Write(entry); err != nil {
		t.Fatal(err)
	}

	got, err := (Reader{Dir: tmp}).ReadByID(entry.OperationID)
	if err != nil {
		t.Fatalf("ReadByID failed: %v", err)
	}
	if got.OperationID != entry.OperationID {
		t.Fatalf("unexpected entry: %+v", got)
	}
}

func TestReader_ReadAllMissingDir(t *testing.T) {
	entries, stats, err := (Reader{Dir: filepath.Join(t.TempDir(), "missing")}).ReadAll()
	if err != nil {
		t.Fatalf("ReadAll failed: %v", err)
	}
	if stats.TotalFilesSeen != 0 || stats.LoadedEntries != 0 || stats.SkippedCorrupt != 0 {
		t.Fatalf("unexpected read stats: %+v", stats)
	}
	if len(entries) != 0 {
		t.Fatalf("expected no entries, got: %+v", entries)
	}
}
