package journalstore

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	domainjournal "github.com/lazuale/espocrm-ops/internal/domain/journal"
)

type EntryWithPath struct {
	Entry domainjournal.Entry
	Path  string
}

type Reader struct {
	Dir string
}

func (r Reader) ReadAll() ([]domainjournal.Entry, domainjournal.ReadStats, error) {
	items, stats, err := r.ReadAllWithPaths()
	if err != nil {
		return nil, stats, err
	}

	entries := make([]domainjournal.Entry, 0, len(items))
	for _, item := range items {
		entries = append(entries, item.Entry)
	}

	return entries, stats, nil
}

func (r Reader) ReadAllWithPaths() ([]EntryWithPath, domainjournal.ReadStats, error) {
	paths, err := listJournalFiles(r.Dir)
	if err != nil {
		return nil, domainjournal.ReadStats{}, err
	}

	stats := domainjournal.ReadStats{
		TotalFilesSeen: len(paths),
	}

	items := make([]EntryWithPath, 0, len(paths))
	for _, path := range paths {
		entry, err := r.ReadByPath(path)
		if err != nil {
			stats.SkippedCorrupt++
			continue
		}
		items = append(items, EntryWithPath{
			Entry: entry,
			Path:  path,
		})
	}
	stats.LoadedEntries = len(items)

	sort.Slice(items, func(i, j int) bool {
		iTime := parseStartedAt(items[i].Entry.StartedAt)
		jTime := parseStartedAt(items[j].Entry.StartedAt)
		if iTime.Equal(jTime) {
			return items[i].Path > items[j].Path
		}
		return iTime.After(jTime)
	})

	return items, stats, nil
}

func (r Reader) ReadByID(id string) (domainjournal.Entry, error) {
	entries, _, err := r.ReadAll()
	if err != nil {
		return domainjournal.Entry{}, err
	}

	for _, entry := range entries {
		if entry.OperationID == id {
			return entry, nil
		}
	}

	return domainjournal.Entry{}, fmt.Errorf("operation not found: %s", id)
}

func (r Reader) ReadByPath(path string) (domainjournal.Entry, error) {
	var entry domainjournal.Entry

	raw, err := os.ReadFile(path)
	if err != nil {
		return entry, fmt.Errorf("read journal entry: %w", err)
	}

	if err := json.Unmarshal(raw, &entry); err != nil {
		return entry, fmt.Errorf("parse journal entry: %w", err)
	}

	return entry, nil
}

func listJournalFiles(dir string) ([]string, error) {
	if dir == "" {
		return nil, fmt.Errorf("journal dir is required")
	}

	if _, err := os.Stat(dir); err != nil {
		if os.IsNotExist(err) {
			return []string{}, nil
		}
		return nil, fmt.Errorf("stat journal dir: %w", err)
	}

	var paths []string
	err := filepath.WalkDir(dir, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		if strings.HasSuffix(entry.Name(), ".json") {
			paths = append(paths, path)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk journal dir: %w", err)
	}

	return paths, nil
}

func parseStartedAt(raw string) time.Time {
	startedAt, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return time.Time{}
	}
	return startedAt
}
