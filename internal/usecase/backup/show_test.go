package backup

import (
	"path/filepath"
	"testing"
	"time"
)

func TestShow_ReturnsSelectedBackupByID(t *testing.T) {
	root := filepath.Join(t.TempDir(), "backups")
	journalDir := filepath.Join(t.TempDir(), "journal")

	_, _ = writeCatalogBackupSet(t, root, "espocrm-dev", "2026-04-07_01-00-00", "dev")
	writeCatalogJournalEntry(t, journalDir, "backup.json", map[string]any{
		"operation_id": "op-backup-1",
		"command":      "backup",
		"started_at":   "2026-04-07T01:00:00Z",
		"finished_at":  "2026-04-07T01:01:00Z",
		"ok":           true,
		"artifacts": map[string]any{
			"manifest": filepath.Join(root, "manifests", "espocrm-dev_2026-04-07_01-00-00.manifest.json"),
		},
	})

	info, err := Show(ShowRequest{
		BackupRoot:     root,
		ID:             "espocrm-dev_2026-04-07_01-00-00",
		JournalDir:     journalDir,
		VerifyChecksum: true,
		Now:            time.Date(2026, 4, 15, 12, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}

	if info.Item.ID != "espocrm-dev_2026-04-07_01-00-00" {
		t.Fatalf("unexpected item: %#v", info.Item)
	}
	if info.Item.Origin.Kind != BackupOriginNormalBackup {
		t.Fatalf("unexpected origin: %#v", info.Item.Origin)
	}
}

func TestShow_ReturnsNotFoundForUnknownID(t *testing.T) {
	root := filepath.Join(t.TempDir(), "backups")

	_, _ = writeCatalogBackupSet(t, root, "espocrm-dev", "2026-04-07_01-00-00", "dev")

	if _, err := Show(ShowRequest{
		BackupRoot: root,
		ID:         "missing",
		Now:        time.Date(2026, 4, 15, 12, 0, 0, 0, time.UTC),
	}); err == nil {
		t.Fatal("expected not found error")
	}
}
