package cli

import "testing"

func TestSchema_GlobalOptions_JSON_Error_InvalidJournalDir(t *testing.T) {
	for _, tc := range []struct {
		name        string
		journalDir  string
		messagePart string
	}{
		{
			name:        "blank",
			journalDir:  "   ",
			messagePart: "--journal-dir is required",
		},
		{
			name:        "current-dir",
			journalDir:  ".",
			messagePart: "--journal-dir must not be the current directory",
		},
		{
			name:        "root",
			journalDir:  "/",
			messagePart: "--journal-dir must not be the filesystem root",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			outcome := executeCLI(
				"--journal-dir", tc.journalDir,
				"--json",
				"doctor",
			)

			assertUsageErrorOutput(t, outcome, tc.messagePart)
		})
	}
}
