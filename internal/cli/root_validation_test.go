package cli

import (
	"path/filepath"
	"testing"
)

func TestSchema_Root_JSON_Error_UnknownCommand_NoJournal(t *testing.T) {
	tmp := t.TempDir()
	journalDir := filepath.Join(tmp, "journal")

	outcome := executeCLI(
		"--journal-dir", journalDir,
		"--json",
		"totally-unknown-command",
	)

	assertUsageErrorOutput(t, outcome, `unknown command "totally-unknown-command" for "espops"`)
	assertNoJournalFiles(t, journalDir)
}

func TestSchema_Root_JSON_Error_UnknownFlag_NoJournal(t *testing.T) {
	tmp := t.TempDir()
	journalDir := filepath.Join(tmp, "journal")

	outcome := executeCLI(
		"--journal-dir", journalDir,
		"--json",
		"history",
		"--totally-unknown-flag",
	)

	assertUsageErrorOutput(t, outcome, "unknown flag: --totally-unknown-flag")
	assertNoJournalFiles(t, journalDir)
}

func TestRootExposesSingleDashboardCommand(t *testing.T) {
	root := newTestRootCmd()

	commands := map[string]struct{}{}
	for _, cmd := range root.Commands() {
		commands[cmd.Name()] = struct{}{}
	}

	if _, ok := commands["overview"]; !ok {
		t.Fatalf("expected overview command to be present")
	}
	for _, forbidden := range []string{"summary", "dashboard"} {
		if _, ok := commands[forbidden]; ok {
			t.Fatalf("expected no duplicate dashboard command %q", forbidden)
		}
	}
}
