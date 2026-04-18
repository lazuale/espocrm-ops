package journalstore

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	domainjournal "github.com/lazuale/espocrm-ops/internal/domain/journal"
	platformclock "github.com/lazuale/espocrm-ops/internal/platform/clock"
	"github.com/lazuale/espocrm-ops/internal/platform/locks"
)

type pruneTestClock struct {
	now time.Time
}

func (c pruneTestClock) Now() time.Time {
	return c.now
}

func TestPrune_KeepMostRecentByStartedAt(t *testing.T) {
	tmp := t.TempDir()
	writer := FSWriter{Dir: tmp}
	entries := []domainjournal.Entry{
		{
			OperationID: "op-20260415T120000Z-11111111",
			Command:     "verify-backup",
			StartedAt:   "2026-04-15T12:00:00Z",
			OK:          true,
		},
		{
			OperationID: "op-20260415T130000Z-22222222",
			Command:     "restore-files",
			StartedAt:   "2026-04-15T13:00:00Z",
			OK:          true,
		},
		{
			OperationID: "op-20260415T140000Z-33333333",
			Command:     "restore-db",
			StartedAt:   "2026-04-15T14:00:00Z",
			OK:          false,
		},
	}

	for _, entry := range entries {
		if err := writer.Write(entry); err != nil {
			t.Fatal(err)
		}
	}

	res, err := Prune(tmp, PruneRequest{KeepLast: 2, DryRun: true})
	if err != nil {
		t.Fatalf("Prune dry-run failed: %v", err)
	}
	if res.Checked != 3 || res.Retained != 2 || res.Protected != 1 || res.Deleted != 1 {
		t.Fatalf("unexpected dry-run result: %+v", res)
	}
	if res.ReadStats.TotalFilesSeen != 3 || res.ReadStats.LoadedEntries != 3 || res.ReadStats.SkippedCorrupt != 0 {
		t.Fatalf("unexpected read stats: %+v", res.ReadStats)
	}
	if res.LatestOperationID != entries[2].OperationID {
		t.Fatalf("expected latest operation id %s, got %s", entries[2].OperationID, res.LatestOperationID)
	}
	if len(res.Paths) != 1 || filepath.Base(res.Paths[0]) != entries[0].OperationID+".json" {
		t.Fatalf("expected oldest entry to be selected, got: %+v", res.Paths)
	}
	if len(res.Decisions) != 3 {
		t.Fatalf("expected a decision for each entry, got %+v", res.Decisions)
	}
	if res.Decisions[0].Decision != PruneDecisionProtect || !hasPruneReason(res.Decisions[0].Reasons, PruneReasonLatestOperation) {
		t.Fatalf("expected newest entry to be protected, got %+v", res.Decisions[0])
	}
	if res.Decisions[1].Decision != PruneDecisionKeep {
		t.Fatalf("expected middle entry to be kept, got %+v", res.Decisions[1])
	}
	if res.Decisions[2].Decision != PruneDecisionRemove || !hasPruneReason(res.Decisions[2].Reasons, PruneReasonOutsideKeepLast) {
		t.Fatalf("expected oldest entry to be removed outside keep-last, got %+v", res.Decisions[2])
	}

	if _, err := os.Stat(filepath.Join(tmp, entries[0].OperationID+".json")); err != nil {
		t.Fatalf("dry-run should not delete file: %v", err)
	}

	res, err = Prune(tmp, PruneRequest{KeepLast: 2})
	if err != nil {
		t.Fatalf("Prune failed: %v", err)
	}
	if res.Deleted != 1 {
		t.Fatalf("unexpected prune result: %+v", res)
	}
	if _, err := os.Stat(filepath.Join(tmp, entries[0].OperationID+".json")); !os.IsNotExist(err) {
		t.Fatalf("expected oldest entry to be deleted, got: %v", err)
	}
}

func TestPrune_RequiresRetentionPolicy(t *testing.T) {
	tmp := t.TempDir()
	emptyDir := filepath.Join(tmp, "2026-04-01")
	if err := os.MkdirAll(emptyDir, 0o755); err != nil {
		t.Fatal(err)
	}

	if _, err := Prune(tmp, PruneRequest{}); err == nil {
		t.Fatal("expected missing retention policy error")
	}
	if _, err := os.Stat(emptyDir); err != nil {
		t.Fatalf("invalid prune request should not remove directories: %v", err)
	}
}

func TestPrune_RejectsConcurrentHolder(t *testing.T) {
	tmp := t.TempDir()
	writer := FSWriter{Dir: tmp}

	oldEntry := domainjournal.Entry{
		OperationID: "op-old",
		Command:     "verify-backup",
		StartedAt:   time.Now().UTC().AddDate(0, 0, -10).Format(time.RFC3339),
		OK:          true,
	}
	newEntry := domainjournal.Entry{
		OperationID: "op-new",
		Command:     "verify-backup",
		StartedAt:   time.Now().UTC().Format(time.RFC3339),
		OK:          true,
	}
	if err := writer.Write(oldEntry); err != nil {
		t.Fatal(err)
	}
	if err := writer.Write(newEntry); err != nil {
		t.Fatal(err)
	}

	lock, err := locks.AcquireJournalPruneLock(tmp)
	if err != nil {
		t.Fatalf("acquire prune lock failed: %v", err)
	}
	defer func() {
		if releaseErr := lock.Release(); releaseErr != nil {
			t.Fatalf("release prune lock failed: %v", releaseErr)
		}
	}()

	if _, err := Prune(tmp, PruneRequest{KeepLast: 1}); err == nil {
		t.Fatal("expected concurrent prune to fail")
	} else {
		var lockErr locks.LockError
		if !errors.As(err, &lockErr) {
			t.Fatalf("expected LockError, got %T: %v", err, err)
		}
	}

	if _, err := os.Stat(filepath.Join(tmp, oldEntry.OperationID+".json")); err != nil {
		t.Fatalf("locked prune should not delete old entry: %v", err)
	}
	if _, err := os.Stat(filepath.Join(tmp, newEntry.OperationID+".json")); err != nil {
		t.Fatalf("locked prune should not delete new entry: %v", err)
	}
}

func TestPrune_ReleasesLockAfterRun(t *testing.T) {
	tmp := t.TempDir()

	if _, err := Prune(tmp, PruneRequest{KeepLast: 10}); err != nil {
		t.Fatalf("prune failed: %v", err)
	}

	lock, err := locks.AcquireJournalPruneLock(tmp)
	if err != nil {
		t.Fatalf("expected prune lock to be released: %v", err)
	}
	_ = lock.Release()
}

func TestPrune_PartialFileRemovalFailureIsRepeatable(t *testing.T) {
	tmp := t.TempDir()
	writer := FSWriter{Dir: tmp}

	entries := []domainjournal.Entry{
		{
			OperationID: "op-new",
			Command:     "verify-backup",
			StartedAt:   "2026-04-15T14:00:00Z",
			OK:          true,
		},
		{
			OperationID: "op-middle",
			Command:     "verify-backup",
			StartedAt:   "2026-04-15T13:00:00Z",
			OK:          true,
		},
		{
			OperationID: "op-old",
			Command:     "verify-backup",
			StartedAt:   "2026-04-15T12:00:00Z",
			OK:          true,
		},
	}
	for _, entry := range entries {
		if err := writer.Write(entry); err != nil {
			t.Fatal(err)
		}
	}

	newPath := filepath.Join(tmp, "op-new.json")
	middlePath := filepath.Join(tmp, "op-middle.json")
	oldPath := filepath.Join(tmp, "op-old.json")

	oldRemove := removeJournalFile
	removeCalls := 0
	removeJournalFile = func(path string) error {
		removeCalls++
		if removeCalls == 2 {
			return errors.New("injected remove failure")
		}
		return os.Remove(path)
	}
	t.Cleanup(func() {
		removeJournalFile = oldRemove
	})

	res, err := Prune(tmp, PruneRequest{KeepLast: 1})
	if err == nil {
		t.Fatal("expected partial prune failure")
	}

	var removeErr PruneRemovalError
	if !errors.As(err, &removeErr) {
		t.Fatalf("expected PruneRemovalError, got %T: %v", err, err)
	}
	if removeErr.Path != oldPath {
		t.Fatalf("expected failed path %s, got %s", oldPath, removeErr.Path)
	}
	if res.Deleted != 1 {
		t.Fatalf("expected exactly one successful deletion before failure, got %+v", res)
	}
	if len(res.Paths) != 1 || res.Paths[0] != middlePath {
		t.Fatalf("expected only successfully deleted path, got %+v", res.Paths)
	}
	if res.FailedPath != oldPath {
		t.Fatalf("expected failed path in result, got %s", res.FailedPath)
	}

	if _, err := os.Stat(newPath); err != nil {
		t.Fatalf("new entry should remain: %v", err)
	}
	if _, err := os.Stat(middlePath); !os.IsNotExist(err) {
		t.Fatalf("middle entry should have been deleted before failure, got %v", err)
	}
	if _, err := os.Stat(oldPath); err != nil {
		t.Fatalf("old entry should remain after injected failure: %v", err)
	}

	removeJournalFile = oldRemove
	retry, err := Prune(tmp, PruneRequest{KeepLast: 1})
	if err != nil {
		t.Fatalf("retry prune failed: %v", err)
	}
	if retry.Deleted != 1 {
		t.Fatalf("expected retry to delete remaining old entry, got %+v", retry)
	}
	if _, err := os.Stat(oldPath); !os.IsNotExist(err) {
		t.Fatalf("old entry should be deleted on retry, got %v", err)
	}
	if _, err := os.Stat(newPath); err != nil {
		t.Fatalf("new entry should remain after retry: %v", err)
	}
}

func TestPrune_KeepDaysUsesStartedAtNotMtime(t *testing.T) {
	tmp := t.TempDir()
	writer := FSWriter{Dir: tmp}

	oldStartedAt := time.Now().UTC().AddDate(0, 0, -10).Format(time.RFC3339)
	newMtime := time.Now().UTC()
	oldEntry := domainjournal.Entry{
		OperationID: "op-old-started-at",
		Command:     "verify-backup",
		StartedAt:   oldStartedAt,
		OK:          true,
	}
	if err := writer.Write(oldEntry); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(filepath.Join(tmp, oldEntry.OperationID+".json"), newMtime, newMtime); err != nil {
		t.Fatal(err)
	}
	newEntry := domainjournal.Entry{
		OperationID: "op-new-started-at",
		Command:     "verify-backup",
		StartedAt:   time.Now().UTC().Format(time.RFC3339),
		FinishedAt:  time.Now().UTC().Add(time.Second).Format(time.RFC3339),
		OK:          true,
	}
	if err := writer.Write(newEntry); err != nil {
		t.Fatal(err)
	}

	badPath := filepath.Join(tmp, "op-invalid.json")
	if err := os.WriteFile(badPath, []byte("{bad"), 0o644); err != nil {
		t.Fatal(err)
	}
	newBadMtime := time.Now().UTC()
	if err := os.Chtimes(badPath, newBadMtime, newBadMtime); err != nil {
		t.Fatal(err)
	}

	res, err := Prune(tmp, PruneRequest{KeepDays: 5, DryRun: true})
	if err != nil {
		t.Fatalf("Prune failed: %v", err)
	}
	if res.Checked != 2 || res.Retained != 1 || res.Protected != 1 || res.Deleted != 1 {
		t.Fatalf("expected old StartedAt entry to be selected while keeping the newest entry, got: %+v", res)
	}
	if res.ReadStats.TotalFilesSeen != 3 || res.ReadStats.LoadedEntries != 2 || res.ReadStats.SkippedCorrupt != 1 {
		t.Fatalf("unexpected read stats: %+v", res.ReadStats)
	}
	if len(res.Paths) != 1 || filepath.Base(res.Paths[0]) != oldEntry.OperationID+".json" {
		t.Fatalf("expected old StartedAt entry path, got: %+v", res.Paths)
	}
}

func TestPrune_ProtectsLatestOperationWhenAgePolicyWouldDeleteIt(t *testing.T) {
	restoreClock := platformclock.SetForTest(pruneTestClock{now: time.Date(2026, 4, 15, 12, 0, 0, 0, time.UTC)})
	t.Cleanup(restoreClock)

	tmp := t.TempDir()
	writer := FSWriter{Dir: tmp}

	entries := []domainjournal.Entry{
		{
			OperationID: "op-oldest",
			Command:     "verify-backup",
			StartedAt:   "2026-02-01T12:00:00Z",
			FinishedAt:  "2026-02-01T12:00:01Z",
			OK:          true,
		},
		{
			OperationID: "op-latest-but-old",
			Command:     "verify-backup",
			StartedAt:   "2026-03-01T12:00:00Z",
			FinishedAt:  "2026-03-01T12:00:01Z",
			OK:          true,
		},
	}
	for _, entry := range entries {
		if err := writer.Write(entry); err != nil {
			t.Fatal(err)
		}
	}

	res, err := Prune(tmp, PruneRequest{KeepDays: 30, DryRun: true})
	if err != nil {
		t.Fatalf("Prune failed: %v", err)
	}
	if res.Checked != 2 || res.Retained != 1 || res.Protected != 1 || res.Deleted != 1 {
		t.Fatalf("unexpected prune result: %+v", res)
	}
	if res.LatestOperationID != "op-latest-but-old" {
		t.Fatalf("expected latest operation id op-latest-but-old, got %s", res.LatestOperationID)
	}
	if len(res.Decisions) != 2 {
		t.Fatalf("expected two prune decisions, got %+v", res.Decisions)
	}
	if res.Decisions[0].Decision != PruneDecisionProtect ||
		!hasPruneReason(res.Decisions[0].Reasons, PruneReasonLatestOperation) ||
		!hasPruneReason(res.Decisions[0].Reasons, PruneReasonOlderThanKeepDays) {
		t.Fatalf("expected latest old operation to be protected explicitly, got %+v", res.Decisions[0])
	}
	if res.Decisions[1].Decision != PruneDecisionRemove || !hasPruneReason(res.Decisions[1].Reasons, PruneReasonOlderThanKeepDays) {
		t.Fatalf("expected oldest operation to be removed by keep-days, got %+v", res.Decisions[1])
	}
}

func TestPrune_RemovesEmptyDirs(t *testing.T) {
	tmp := t.TempDir()
	oldDayDir := filepath.Join(tmp, "2026-04-01")
	newDayDir := filepath.Join(tmp, "2026-04-15")
	if err := os.MkdirAll(oldDayDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(newDayDir, 0o755); err != nil {
		t.Fatal(err)
	}

	oldEntry := domainjournal.Entry{
		OperationID: "op-20260401T120000Z-old",
		Command:     "verify-backup",
		StartedAt:   time.Now().UTC().AddDate(0, 0, -10).Format(time.RFC3339),
		FinishedAt:  time.Now().UTC().AddDate(0, 0, -10).Add(time.Second).Format(time.RFC3339),
		OK:          true,
	}
	newEntry := domainjournal.Entry{
		OperationID: "op-20260415T120000Z-new",
		Command:     "verify-backup",
		StartedAt:   time.Now().UTC().Format(time.RFC3339),
		FinishedAt:  time.Now().UTC().Add(time.Second).Format(time.RFC3339),
		OK:          true,
	}
	if err := (FSWriter{Dir: oldDayDir}).Write(oldEntry); err != nil {
		t.Fatal(err)
	}
	if err := (FSWriter{Dir: newDayDir}).Write(newEntry); err != nil {
		t.Fatal(err)
	}

	res, err := Prune(tmp, PruneRequest{KeepDays: 5})
	if err != nil {
		t.Fatalf("Prune failed: %v", err)
	}
	if res.Deleted != 1 || res.RemovedDirs != 1 {
		t.Fatalf("unexpected prune result: %+v", res)
	}
	if len(res.RemovedPaths) != 1 || res.RemovedPaths[0] != oldDayDir {
		t.Fatalf("unexpected removed paths: %+v", res.RemovedPaths)
	}
	if _, err := os.Stat(oldDayDir); !os.IsNotExist(err) {
		t.Fatalf("expected empty day dir to be removed, got: %v", err)
	}
	if _, err := os.Stat(newDayDir); err != nil {
		t.Fatalf("expected newest day dir to remain, got: %v", err)
	}
}

func hasPruneReason(reasons []string, want string) bool {
	for _, reason := range reasons {
		if reason == want {
			return true
		}
	}

	return false
}
