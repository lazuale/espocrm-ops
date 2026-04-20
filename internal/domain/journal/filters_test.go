package journal

import (
	"testing"
	"time"
)

func TestApplyFilters(t *testing.T) {
	since := time.Date(2026, 4, 15, 12, 30, 0, 0, time.UTC)
	until := time.Date(2026, 4, 15, 13, 30, 0, 0, time.UTC)
	entries := []Entry{
		{
			OperationID: "op-old",
			Command:     "backup verify",
			StartedAt:   "2026-04-15T12:00:00Z",
			OK:          true,
		},
		{
			OperationID: "op-hit",
			Command:     "restore",
			StartedAt:   "2026-04-15T13:00:00Z",
			OK:          false,
		},
		{
			OperationID: "op-new",
			Command:     "restore",
			StartedAt:   "2026-04-15T14:00:00Z",
			OK:          false,
		},
	}

	got := ApplyFilters(entries, Filters{
		Command:    "restore",
		FailedOnly: true,
		Since:      &since,
		Until:      &until,
		Limit:      1,
	})

	if len(got) != 1 || got[0].OperationID != "op-hit" {
		t.Fatalf("unexpected filtered entries: %+v", got)
	}
}

func TestApplyFiltersDateRangeSkipsInvalidStartedAt(t *testing.T) {
	since := time.Date(2026, 4, 15, 12, 0, 0, 0, time.UTC)
	entries := []Entry{
		{
			OperationID: "op-invalid",
			Command:     "restore",
			StartedAt:   "not-a-time",
			OK:          false,
		},
	}

	got := ApplyFilters(entries, Filters{Since: &since})
	if len(got) != 0 {
		t.Fatalf("expected invalid started_at to be excluded by date filter, got: %+v", got)
	}
}
