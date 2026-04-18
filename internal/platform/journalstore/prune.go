package journalstore

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	domainjournal "github.com/lazuale/espocrm-ops/internal/domain/journal"
	platformclock "github.com/lazuale/espocrm-ops/internal/platform/clock"
	"github.com/lazuale/espocrm-ops/internal/platform/locks"
)

type PruneRequest struct {
	KeepDays int
	KeepLast int
	DryRun   bool
}

const (
	PruneDecisionRemove  = "remove"
	PruneDecisionKeep    = "keep"
	PruneDecisionProtect = "protect"

	PruneReasonOlderThanKeepDays = "older_than_keep_days"
	PruneReasonOutsideKeepLast   = "outside_keep_last"
	PruneReasonLatestOperation   = "latest_operation"
	PruneReasonUnfinished        = "unfinished_operation"
)

type PruneDecision struct {
	Entry    domainjournal.Entry
	Path     string
	Decision string
	Reasons  []string
}

type PruneResult struct {
	ReadStats         domainjournal.ReadStats `json:"read"`
	Checked           int                     `json:"checked"`
	Retained          int                     `json:"retained"`
	Protected         int                     `json:"protected"`
	Deleted           int                     `json:"deleted"`
	RemovedDirs       int                     `json:"removed_dirs"`
	LatestOperationID string                  `json:"latest_operation_id,omitempty"`
	Decisions         []PruneDecision         `json:"decisions,omitempty"`
	Paths             []string                `json:"paths,omitempty"`
	RemovedPaths      []string                `json:"removed_paths,omitempty"`
	FailedPath        string                  `json:"failed_path,omitempty"`
}

type PruneRemovalError struct {
	Path string
	Err  error
}

func (e PruneRemovalError) Error() string {
	return fmt.Sprintf("remove journal file %s: %v", e.Path, e.Err)
}

func (e PruneRemovalError) Unwrap() error {
	return e.Err
}

var removeJournalFile = os.Remove
var removeJournalDir = os.Remove

func Prune(dir string, req PruneRequest) (result PruneResult, err error) {
	if dir == "" {
		return PruneResult{}, fmt.Errorf("journal dir is required")
	}
	if req.KeepDays < 0 {
		return PruneResult{}, fmt.Errorf("keep-days must be non-negative")
	}
	if req.KeepLast < 0 {
		return PruneResult{}, fmt.Errorf("keep-last must be non-negative")
	}
	if req.KeepDays == 0 && req.KeepLast == 0 {
		return PruneResult{}, fmt.Errorf("keep-days or keep-last is required")
	}

	lock, err := locks.AcquireJournalPruneLock(dir)
	if err != nil {
		return PruneResult{}, err
	}
	defer func() {
		if releaseErr := lock.Release(); releaseErr != nil {
			wrapped := fmt.Errorf("release journal prune lock: %w", releaseErr)
			if err == nil {
				err = wrapped
			} else {
				err = errors.Join(err, wrapped)
			}
		}
	}()

	items, stats, err := (Reader{Dir: dir}).ReadAllWithPaths()
	if err != nil {
		return PruneResult{}, err
	}

	result = PruneResult{
		ReadStats: stats,
		Checked:   len(items),
		Decisions: planPrune(items, req),
	}
	if len(items) > 0 {
		result.LatestOperationID = items[0].Entry.OperationID
	}

	for _, item := range result.Decisions {
		switch item.Decision {
		case PruneDecisionProtect:
			result.Protected++
			result.Retained++
			continue
		case PruneDecisionKeep:
			result.Retained++
			continue
		}

		if req.DryRun {
			result.Paths = append(result.Paths, item.Path)
			result.Deleted++
			continue
		}

		if err := removeJournalFile(item.Path); err != nil {
			result.FailedPath = item.Path
			return result, PruneRemovalError{Path: item.Path, Err: err}
		}
		result.Paths = append(result.Paths, item.Path)
		result.Deleted++
	}

	if !req.DryRun {
		removedDirs, removedPaths, err := removeEmptyDirs(dir)
		if err != nil {
			return result, err
		}
		result.RemovedDirs = removedDirs
		result.RemovedPaths = removedPaths
	}

	return result, nil
}

func planPrune(items []EntryWithPath, req PruneRequest) []PruneDecision {
	decisions := make([]PruneDecision, 0, len(items))

	cutoff := platformclock.Now().AddDate(0, 0, -req.KeepDays)
	for idx, item := range items {
		reasons := []string{}
		protect := false
		remove := false

		if idx == 0 {
			protect = true
			reasons = appendUniquePruneReason(reasons, PruneReasonLatestOperation)
			if strings.TrimSpace(item.Entry.FinishedAt) == "" {
				reasons = appendUniquePruneReason(reasons, PruneReasonUnfinished)
			}
		}
		if req.KeepDays > 0 {
			startedAt := parseStartedAt(item.Entry.StartedAt)
			if !startedAt.IsZero() && startedAt.Before(cutoff) {
				remove = true
				reasons = appendUniquePruneReason(reasons, PruneReasonOlderThanKeepDays)
			}
		}
		if req.KeepLast > 0 && idx >= req.KeepLast {
			remove = true
			reasons = appendUniquePruneReason(reasons, PruneReasonOutsideKeepLast)
		}

		decision := PruneDecisionKeep
		if remove {
			decision = PruneDecisionRemove
		}
		if protect {
			decision = PruneDecisionProtect
		}

		decisions = append(decisions, PruneDecision{
			Entry:    item.Entry,
			Path:     item.Path,
			Decision: decision,
			Reasons:  reasons,
		})
	}

	return decisions
}

func appendUniquePruneReason(reasons []string, value string) []string {
	for _, current := range reasons {
		if current == value {
			return reasons
		}
	}

	return append(reasons, value)
}

func removeEmptyDirs(root string) (int, []string, error) {
	if _, err := os.Stat(root); err != nil {
		if os.IsNotExist(err) {
			return 0, []string{}, nil
		}
		return 0, nil, fmt.Errorf("stat journal dir: %w", err)
	}

	var dirs []string
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() && path != root {
			dirs = append(dirs, path)
		}
		return nil
	})
	if err != nil {
		return 0, nil, fmt.Errorf("walk journal dirs: %w", err)
	}

	for i, j := 0, len(dirs)-1; i < j; i, j = i+1, j-1 {
		dirs[i], dirs[j] = dirs[j], dirs[i]
	}

	removed := 0
	removedPaths := []string{}
	for _, dir := range dirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			return removed, removedPaths, fmt.Errorf("read dir %s: %w", dir, err)
		}
		if len(entries) != 0 {
			continue
		}
		if err := removeJournalDir(dir); err != nil {
			return removed, removedPaths, fmt.Errorf("remove empty dir %s: %w", dir, err)
		}
		removed++
		removedPaths = append(removedPaths, dir)
	}

	return removed, removedPaths, nil
}
