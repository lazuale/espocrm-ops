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

func TestRootExposesSingleHousekeepingCommand(t *testing.T) {
	root := newTestRootCmd()

	commands := map[string]struct{}{}
	for _, cmd := range root.Commands() {
		commands[cmd.Name()] = struct{}{}
	}

	if _, ok := commands["maintenance"]; !ok {
		t.Fatalf("expected maintenance command to be present")
	}
	for _, forbidden := range []string{"cleanup", "housekeeping", "scheduled-maintenance", "maintenance-run"} {
		if _, ok := commands[forbidden]; ok {
			t.Fatalf("expected no duplicate housekeeping command %q", forbidden)
		}
	}
}

func TestRootExposesSingleHealthSummaryCommand(t *testing.T) {
	root := newTestRootCmd()

	commands := map[string]struct{}{}
	for _, cmd := range root.Commands() {
		commands[cmd.Name()] = struct{}{}
	}

	if _, ok := commands["health-summary"]; !ok {
		t.Fatalf("expected health-summary command to be present")
	}
	for _, forbidden := range []string{"health", "alerts", "health-alerts"} {
		if _, ok := commands[forbidden]; ok {
			t.Fatalf("expected no duplicate health command %q", forbidden)
		}
	}
}

func TestRootExposesSingleOperationGateCommand(t *testing.T) {
	root := newTestRootCmd()

	commands := map[string]struct{}{}
	for _, cmd := range root.Commands() {
		commands[cmd.Name()] = struct{}{}
	}

	if _, ok := commands["operation-gate"]; !ok {
		t.Fatalf("expected operation-gate command to be present")
	}
	for _, forbidden := range []string{"can-run", "readiness", "operation-readiness"} {
		if _, ok := commands[forbidden]; ok {
			t.Fatalf("expected no duplicate operation gate command %q", forbidden)
		}
	}
}

func TestMaintenanceCommandExposesUnattendedFlags(t *testing.T) {
	root := newTestRootCmd()
	maintenance, _, err := root.Find([]string{"maintenance"})
	if err != nil {
		t.Fatalf("find maintenance command: %v", err)
	}

	for _, name := range []string{"unattended", "allow-unattended-apply"} {
		if maintenance.Flags().Lookup(name) == nil {
			t.Fatalf("expected maintenance to expose flag %q", name)
		}
	}
}
