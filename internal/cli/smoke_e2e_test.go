package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/lazuale/espocrm-ops/internal/contract/exitcode"
	"github.com/lazuale/espocrm-ops/internal/domain/operation"
)

type sequenceRuntime struct {
	nextTime time.Time
	nextID   int
}

func (s *sequenceRuntime) Now() time.Time {
	now := s.nextTime
	s.nextTime = s.nextTime.Add(time.Second)
	return now
}

func (s *sequenceRuntime) NewOperationID() string {
	s.nextID++
	return fmt.Sprintf("op-smoke-%02d", s.nextID)
}

var _ operation.Runtime = (*sequenceRuntime)(nil)

func TestSmoke_JournalPrune_EndToEnd(t *testing.T) {
	useJournalClockForTest(t, time.Date(2026, 4, 15, 12, 0, 0, 0, time.UTC))

	tmp := t.TempDir()
	journalDir := filepath.Join(tmp, "journal")

	empty := runCLIJSON(t,
		"--journal-dir", journalDir,
		"--json",
		"journal-prune",
		"--keep-last", "10",
	)
	if deleted := requireJSONPath(t, empty, "details", "deleted"); deleted != float64(0) {
		t.Fatalf("expected empty journal prune to delete 0 entries, got %v", deleted)
	}

	oldDir := filepath.Join(journalDir, "2026-03-01")
	newDir := filepath.Join(journalDir, "2026-04-15")
	if err := os.MkdirAll(oldDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(newDir, 0o755); err != nil {
		t.Fatal(err)
	}

	oldFile := filepath.Join(oldDir, "old.json")
	newFile := filepath.Join(newDir, "new.json")
	writeJournalEntryFile(t, oldDir, "old.json", map[string]any{
		"operation_id": "op-old",
		"command":      "verify-backup",
		"started_at":   "2026-03-01T10:00:00Z",
		"finished_at":  "2026-03-01T10:00:01Z",
		"ok":           true,
	})
	writeJournalEntryFile(t, newDir, "new.json", map[string]any{
		"operation_id": "op-new",
		"command":      "verify-backup",
		"started_at":   "2026-04-15T10:00:00Z",
		"finished_at":  "2026-04-15T10:00:01Z",
		"ok":           true,
	})

	dryRun := runCLIJSON(t,
		"--journal-dir", journalDir,
		"--json",
		"journal-prune",
		"--keep-days", "30",
		"--dry-run",
	)
	if dry := requireJSONPath(t, dryRun, "details", "dry_run"); dry != true {
		t.Fatalf("expected dry-run details, got %v", dry)
	}
	if deleted := requireJSONPath(t, dryRun, "details", "deleted"); deleted != float64(1) {
		t.Fatalf("expected dry-run to plan 1 deletion, got %v", deleted)
	}
	assertPathExists(t, oldFile)
	assertPathExists(t, newFile)

	realRun := runCLIJSON(t,
		"--journal-dir", journalDir,
		"--json",
		"journal-prune",
		"--keep-days", "30",
	)
	if deleted := requireJSONPath(t, realRun, "details", "deleted"); deleted != float64(1) {
		t.Fatalf("expected real prune to delete 1 entry, got %v", deleted)
	}
	assertPathMissing(t, oldFile)
	assertPathMissing(t, oldDir)
	assertPathExists(t, newFile)

	secondRun := runCLIJSON(t,
		"--journal-dir", journalDir,
		"--json",
		"journal-prune",
		"--keep-days", "30",
	)
	if deleted := requireJSONPath(t, secondRun, "details", "deleted"); deleted != float64(0) {
		t.Fatalf("expected idempotent second prune to delete 0 entries, got %v", deleted)
	}
	assertPathExists(t, newFile)
}

func TestSmoke_VerifyBackupRestoreFilesDryRunAndJournal(t *testing.T) {
	seq := &sequenceRuntime{
		nextTime: time.Date(2026, 4, 15, 12, 0, 0, 0, time.UTC),
	}
	opts := []testAppOption{withTestRuntime(seq)}

	tmp := t.TempDir()
	journalDir := filepath.Join(tmp, "journal")
	targetDir := filepath.Join(tmp, "storage")

	dbName := "db.sql.gz"
	filesName := "files.tar.gz"
	manifestPath := filepath.Join(tmp, "manifest.json")
	dbPath := filepath.Join(tmp, dbName)
	filesPath := filepath.Join(tmp, filesName)
	restoredPath := filepath.Join(targetDir, "storage", "new.txt")
	stalePath := filepath.Join(targetDir, "stale.txt")

	writeGzipFile(t, dbPath, []byte("select 1;"))
	writeTarGzFile(t, filesPath, map[string]string{
		"storage/new.txt": "fresh",
	})
	writeJSON(t, manifestPath, map[string]any{
		"version":    1,
		"scope":      "prod",
		"created_at": "2026-04-15T11:00:00Z",
		"artifacts": map[string]any{
			"db_backup":    dbName,
			"files_backup": filesName,
		},
		"checksums": map[string]any{
			"db_backup":    sha256OfFile(t, dbPath),
			"files_backup": sha256OfFile(t, filesPath),
		},
	})

	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(stalePath, []byte("stale"), 0o644); err != nil {
		t.Fatal(err)
	}

	verify := runCLIJSONWithOptions(t, opts,
		"--journal-dir", journalDir,
		"--json",
		"verify-backup",
		"--manifest", manifestPath,
	)
	if command := requireJSONPath(t, verify, "command"); command != "verify-backup" {
		t.Fatalf("unexpected verify command: %v", command)
	}

	restoreDryRun := runCLIJSONWithOptions(t, opts,
		"--journal-dir", journalDir,
		"--json",
		"restore-files",
		"--manifest", manifestPath,
		"--target-dir", targetDir,
		"--dry-run",
	)
	if command := requireJSONPath(t, restoreDryRun, "command"); command != "restore-files" {
		t.Fatalf("unexpected restore command: %v", command)
	}
	if dry := requireJSONPath(t, restoreDryRun, "details", "dry_run"); dry != true {
		t.Fatalf("expected restore dry-run details, got %v", dry)
	}
	assertPathExists(t, stalePath)
	assertPathMissing(t, restoredPath)

	last := runCLIJSONWithOptions(t, opts,
		"--journal-dir", journalDir,
		"--json",
		"last-operation",
	)
	lastItems := requireJSONPath(t, last, "items").([]any)
	if len(lastItems) != 1 {
		t.Fatalf("expected one last-operation item, got %d", len(lastItems))
	}
	lastItem := lastItems[0].(map[string]any)
	if lastItem["command"] != "restore-files" {
		t.Fatalf("expected restore-files to be last journal entry, got %v", lastItem["command"])
	}

	history := runCLIJSONWithOptions(t, opts,
		"--journal-dir", journalDir,
		"--json",
		"history",
		"--limit", "10",
	)
	items := requireJSONPath(t, history, "items").([]any)
	if len(items) != 2 {
		t.Fatalf("expected 2 journal entries, got %d", len(items))
	}
}

func runCLIJSON(t *testing.T, args ...string) map[string]any {
	return runCLIJSONWithOptions(t, nil, args...)
}

func runCLIJSONWithOptions(t *testing.T, opts []testAppOption, args ...string) map[string]any {
	t.Helper()

	outcome := executeCLIWithOptions(opts, args...)
	if outcome.ExitCode != exitcode.OK {
		t.Fatalf("expected command to succeed, exit=%d stdout=%s stderr=%s", outcome.ExitCode, outcome.Stdout, outcome.Stderr)
	}

	var obj map[string]any
	if err := json.Unmarshal([]byte(outcome.Stdout), &obj); err != nil {
		t.Fatalf("invalid json output: %v\n%s", err, outcome.Stdout)
	}

	if ok, _ := obj["ok"].(bool); !ok {
		t.Fatalf("expected ok=true, got %#v", obj)
	}

	return obj
}

func assertPathExists(t *testing.T, path string) {
	t.Helper()

	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected path to exist %s: %v", path, err)
	}
}

func assertPathMissing(t *testing.T, path string) {
	t.Helper()

	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("expected path to be missing %s, got %v", path, err)
	}
}
