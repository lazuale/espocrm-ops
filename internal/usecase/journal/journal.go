package journal

import (
	"errors"
	"fmt"
	"time"

	"github.com/lazuale/espocrm-ops/internal/contract/apperr"
	domainjournal "github.com/lazuale/espocrm-ops/internal/domain/journal"
	"github.com/lazuale/espocrm-ops/internal/platform/journalstore"
	"github.com/lazuale/espocrm-ops/internal/platform/locks"
)

type Entry struct {
	OperationID  string         `json:"operation_id"`
	Command      string         `json:"command"`
	StartedAt    string         `json:"started_at"`
	FinishedAt   string         `json:"finished_at,omitempty"`
	OK           bool           `json:"ok"`
	DryRun       bool           `json:"dry_run,omitempty"`
	Message      string         `json:"message,omitempty"`
	ErrorCode    string         `json:"error_code,omitempty"`
	ErrorMessage string         `json:"error_message,omitempty"`
	Artifacts    map[string]any `json:"artifacts,omitempty"`
	Details      map[string]any `json:"details,omitempty"`
	Items        []any          `json:"items,omitempty"`
	Warnings     []string       `json:"warnings,omitempty"`
}

type Filters struct {
	Command      string
	OKOnly       bool
	FailedOnly   bool
	Status       string
	Scope        string
	RecoveryOnly bool
	TargetPrefix string
	Since        *time.Time
	Until        *time.Time
	Limit        int
}

type ReadStats struct {
	TotalFilesSeen int `json:"total_files_seen"`
	LoadedEntries  int `json:"loaded_entries"`
	SkippedCorrupt int `json:"skipped_corrupt"`
}

type HistoryInput struct {
	JournalDir string
	Filters    Filters
}

type HistoryOutput struct {
	Operations []OperationSummary
	Stats      ReadStats
}

type LastOperationInput struct {
	JournalDir string
	Command    string
}

type LastOperationOutput struct {
	Entry *Entry
	Stats ReadStats
}

type ShowOperationInput struct {
	JournalDir string
	ID         string
}

type ShowOperationOutput struct {
	Entry Entry
	Stats ReadStats
}

type PruneInput struct {
	JournalDir string
	KeepDays   int
	KeepLast   int
	DryRun     bool
}

type PruneOutput struct {
	ReadStats         ReadStats
	Checked           int
	Retained          int
	Protected         int
	Deleted           int
	RemovedDirs       int
	LatestOperationID string
	Items             []PruneItem
	Paths             []string
	RemovedPaths      []string
	FailedPath        string
}

type LockError struct {
	Err error
}

func (e LockError) Error() string {
	return e.Err.Error()
}

func (e LockError) Unwrap() error {
	return e.Err
}

func (e LockError) ErrorKind() apperr.Kind {
	return apperr.KindConflict
}

func (e LockError) ErrorCode() string {
	return "lock_acquire_failed"
}

type NotFoundError struct {
	ID string
}

func (e NotFoundError) Error() string {
	return fmt.Sprintf("operation not found: %s", e.ID)
}

func (e NotFoundError) ErrorKind() apperr.Kind {
	return apperr.KindNotFound
}

func (e NotFoundError) ErrorCode() string {
	return "operation_not_found"
}

func History(in HistoryInput) (HistoryOutput, error) {
	entries, stats, err := (journalstore.Reader{Dir: in.JournalDir}).ReadAll()
	if err != nil {
		return HistoryOutput{}, err
	}

	filteredEntries := entriesFromDomain(domainjournal.ApplyFilters(entries, domainFiltersForHistory(in.Filters)))
	return HistoryOutput{
		Operations: summarizeOperations(filteredEntries, in.Filters),
		Stats:      readStatsFromDomain(stats),
	}, nil
}

func LastOperation(in LastOperationInput) (LastOperationOutput, error) {
	entries, stats, err := (journalstore.Reader{Dir: in.JournalDir}).ReadAll()
	if err != nil {
		return LastOperationOutput{}, err
	}

	for _, entry := range entries {
		if in.Command != "" && entry.Command != in.Command {
			continue
		}
		found := entryFromDomain(entry)
		return LastOperationOutput{
			Entry: &found,
			Stats: readStatsFromDomain(stats),
		}, nil
	}

	return LastOperationOutput{Stats: readStatsFromDomain(stats)}, nil
}

func ShowOperation(in ShowOperationInput) (ShowOperationOutput, error) {
	items, stats, err := (journalstore.Reader{Dir: in.JournalDir}).ReadAllWithPaths()
	if err != nil {
		return ShowOperationOutput{}, err
	}

	for _, item := range items {
		if item.Entry.OperationID == in.ID {
			return ShowOperationOutput{
				Entry: entryFromDomain(item.Entry),
				Stats: readStatsFromDomain(stats),
			}, nil
		}
	}

	return ShowOperationOutput{Stats: readStatsFromDomain(stats)}, NotFoundError{ID: in.ID}
}

func Prune(in PruneInput) (PruneOutput, error) {
	pruneResult, err := journalstore.Prune(in.JournalDir, journalstore.PruneRequest{
		KeepDays: in.KeepDays,
		KeepLast: in.KeepLast,
		DryRun:   in.DryRun,
	})
	if err != nil {
		var lockErr locks.LockError
		if errors.As(err, &lockErr) {
			err = LockError{Err: err}
		}

		return PruneOutput{
			ReadStats:         readStatsFromDomain(pruneResult.ReadStats),
			Checked:           pruneResult.Checked,
			Retained:          pruneResult.Retained,
			Protected:         pruneResult.Protected,
			Deleted:           pruneResult.Deleted,
			RemovedDirs:       pruneResult.RemovedDirs,
			LatestOperationID: pruneResult.LatestOperationID,
			Items:             pruneItemsFromStore(pruneResult),
			Paths:             pruneResult.Paths,
			RemovedPaths:      pruneResult.RemovedPaths,
			FailedPath:        pruneResult.FailedPath,
		}, err
	}

	return PruneOutput{
		ReadStats:         readStatsFromDomain(pruneResult.ReadStats),
		Checked:           pruneResult.Checked,
		Retained:          pruneResult.Retained,
		Protected:         pruneResult.Protected,
		Deleted:           pruneResult.Deleted,
		RemovedDirs:       pruneResult.RemovedDirs,
		LatestOperationID: pruneResult.LatestOperationID,
		Items:             pruneItemsFromStore(pruneResult),
		Paths:             pruneResult.Paths,
		RemovedPaths:      pruneResult.RemovedPaths,
		FailedPath:        pruneResult.FailedPath,
	}, nil
}

func domainFilters(filters Filters) domainjournal.Filters {
	return domainjournal.Filters{
		Command:    filters.Command,
		OKOnly:     filters.OKOnly,
		FailedOnly: filters.FailedOnly,
		Since:      filters.Since,
		Until:      filters.Until,
		Limit:      filters.Limit,
	}
}

func domainFiltersForHistory(filters Filters) domainjournal.Filters {
	filters.Limit = 0
	return domainFilters(filters)
}

func entriesFromDomain(entries []domainjournal.Entry) []Entry {
	out := make([]Entry, 0, len(entries))
	for _, entry := range entries {
		out = append(out, entryFromDomain(entry))
	}

	return out
}

func entryFromDomain(entry domainjournal.Entry) Entry {
	return Entry{
		OperationID:  entry.OperationID,
		Command:      entry.Command,
		StartedAt:    entry.StartedAt,
		FinishedAt:   entry.FinishedAt,
		OK:           entry.OK,
		DryRun:       entry.DryRun,
		Message:      entry.Message,
		ErrorCode:    entry.ErrorCode,
		ErrorMessage: entry.ErrorMessage,
		Artifacts:    entry.Artifacts,
		Details:      entry.Details,
		Items:        entry.Items,
		Warnings:     entry.Warnings,
	}
}

func readStatsFromDomain(stats domainjournal.ReadStats) ReadStats {
	return ReadStats{
		TotalFilesSeen: stats.TotalFilesSeen,
		LoadedEntries:  stats.LoadedEntries,
		SkippedCorrupt: stats.SkippedCorrupt,
	}
}
