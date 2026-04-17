package journal

import "time"

type Filters struct {
	Command    string
	OKOnly     bool
	FailedOnly bool
	Since      *time.Time
	Until      *time.Time
	Limit      int
}

func ApplyFilters(entries []Entry, filters Filters) []Entry {
	out := make([]Entry, 0, len(entries))

	for _, entry := range entries {
		if filters.Command != "" && entry.Command != filters.Command {
			continue
		}
		if filters.OKOnly && !entry.OK {
			continue
		}
		if filters.FailedOnly && entry.OK {
			continue
		}

		if filters.Since != nil || filters.Until != nil {
			startedAt, ok := entryStartedAt(entry)
			if !ok {
				continue
			}
			if filters.Since != nil && startedAt.Before(*filters.Since) {
				continue
			}
			if filters.Until != nil && startedAt.After(*filters.Until) {
				continue
			}
		}

		out = append(out, entry)
		if filters.Limit > 0 && len(out) >= filters.Limit {
			break
		}
	}

	return out
}

func entryStartedAt(entry Entry) (time.Time, bool) {
	startedAt, err := time.Parse(time.RFC3339, entry.StartedAt)
	return startedAt, err == nil
}
