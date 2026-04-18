package journalstore

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	domainjournal "github.com/lazuale/espocrm-ops/internal/domain/journal"
	platformclock "github.com/lazuale/espocrm-ops/internal/platform/clock"
	"github.com/lazuale/espocrm-ops/internal/platform/locks"
)

type PruneRequest struct {
	KeepDays int
	Keep     int
	DryRun   bool
}

type PruneResult struct {
	ReadStats    domainjournal.ReadStats `json:"read"`
	Checked      int                     `json:"checked"`
	Deleted      int                     `json:"deleted"`
	RemovedDirs  int                     `json:"removed_dirs"`
	Paths        []string                `json:"paths,omitempty"`
	RemovedPaths []string                `json:"removed_paths,omitempty"`
	FailedPath   string                  `json:"failed_path,omitempty"`
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
	if req.Keep < 0 {
		return PruneResult{}, fmt.Errorf("keep must be non-negative")
	}
	if req.KeepDays == 0 && req.Keep == 0 {
		return PruneResult{}, fmt.Errorf("keep-days or keep is required")
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
		Paths:     []string{},
	}
	toDelete := make(map[string]struct{})

	if req.KeepDays > 0 {
		cutoff := platformclock.Now().AddDate(0, 0, -req.KeepDays)
		for _, item := range items {
			startedAt := parseStartedAt(item.Entry.StartedAt)
			if startedAt.IsZero() {
				continue
			}
			if startedAt.Before(cutoff) {
				toDelete[item.Path] = struct{}{}
			}
		}
	}

	if req.Keep > 0 && len(items) > req.Keep {
		for _, item := range items[req.Keep:] {
			toDelete[item.Path] = struct{}{}
		}
	}

	for _, item := range items {
		if _, ok := toDelete[item.Path]; !ok {
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
