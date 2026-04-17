package cli

import (
	"path/filepath"
	"testing"
)

func TestSchema_History_JSON_Error_InvalidInput(t *testing.T) {
	for _, tc := range []struct {
		name        string
		args        []string
		messagePart string
	}{
		{
			name:        "negative-limit",
			args:        []string{"--limit", "-1"},
			messagePart: "--limit must be non-negative",
		},
		{
			name:        "conflicting-status-filters",
			args:        []string{"--ok-only", "--failed-only"},
			messagePart: "use either --ok-only or --failed-only",
		},
		{
			name:        "blank-command-filter",
			args:        []string{"--command", "   "},
			messagePart: "--command must not be blank",
		},
		{
			name:        "invalid-since",
			args:        []string{"--since", "not-rfc3339"},
			messagePart: "invalid --since value",
		},
		{
			name:        "since-after-until",
			args:        []string{"--since", "2026-04-16T00:00:00Z", "--until", "2026-04-15T00:00:00Z"},
			messagePart: "--since must be before or equal to --until",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tmp := t.TempDir()
			journalDir := filepath.Join(tmp, "journal")

			args := append([]string{
				"--journal-dir", journalDir,
				"--json",
				"history",
			}, tc.args...)

			outcome := executeCLI(args...)

			assertUsageErrorOutput(t, outcome, tc.messagePart)
		})
	}
}
