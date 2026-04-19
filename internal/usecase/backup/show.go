package backup

import (
	"fmt"
	"strings"
	"time"

	domainbackup "github.com/lazuale/espocrm-ops/internal/domain/backup"
)

type ShowRequest struct {
	BackupRoot     string
	ID             string
	JournalDir     string
	VerifyChecksum bool
	Now            time.Time
}

type ShowInfo struct {
	BackupRoot     string           `json:"backup_root"`
	ID             string           `json:"id"`
	VerifyChecksum bool             `json:"verify_checksum"`
	Item           CatalogItem      `json:"item"`
	JournalRead    JournalReadStats `json:"journal_read"`
	InspectedAt    string           `json:"inspected_at,omitempty"`
}

func Show(req ShowRequest) (ShowInfo, error) {
	backupRoot := strings.TrimSpace(req.BackupRoot)
	id := strings.TrimSpace(req.ID)
	if backupRoot == "" {
		return ShowInfo{}, fmt.Errorf("backup root is required")
	}
	if id == "" {
		return ShowInfo{}, fmt.Errorf("backup id is required")
	}

	now := req.Now
	if now.IsZero() {
		now = time.Now()
	}

	ctx, err := buildCatalogContext(backupRoot, strings.TrimSpace(req.JournalDir), req.VerifyChecksum, now)
	if err != nil {
		return ShowInfo{}, err
	}

	groups, err := catalogGroups(backupRoot)
	if err != nil {
		return ShowInfo{}, err
	}

	for _, group := range groups {
		if domainbackup.BackupSetID(group) != id {
			continue
		}

		item, err := catalogItem(ctx, group)
		if err != nil {
			return ShowInfo{}, err
		}

		return ShowInfo{
			BackupRoot:     backupRoot,
			ID:             id,
			VerifyChecksum: req.VerifyChecksum,
			Item:           item,
			JournalRead:    ctx.journalRead,
			InspectedAt:    now.UTC().Format(time.RFC3339),
		}, nil
	}

	return ShowInfo{
		BackupRoot:     backupRoot,
		ID:             id,
		VerifyChecksum: req.VerifyChecksum,
		JournalRead:    ctx.journalRead,
		InspectedAt:    now.UTC().Format(time.RFC3339),
	}, NotFoundError{ID: id}
}
