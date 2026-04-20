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
		"doctor",
		"--totally-unknown-flag",
	)

	assertUsageErrorOutput(t, outcome, "unknown flag: --totally-unknown-flag")
	assertNoJournalFiles(t, journalDir)
}

func TestRootExposesOnlyBackupRecoveryCommands(t *testing.T) {
	root := newTestRootCmd()

	commands := map[string]struct{}{}
	for _, cmd := range root.Commands() {
		commands[cmd.Name()] = struct{}{}
	}

	for _, expected := range []string{"doctor", "backup", "restore", "migrate"} {
		if _, ok := commands[expected]; !ok {
			t.Fatalf("expected command %q to be present", expected)
		}
	}
	for _, forbidden := range []string{
		"overview",
		"status-report",
		"health-summary",
		"operation-gate",
		"history",
		"show-operation",
		"last-operation",
		"export-operation",
		"support-bundle",
		"maintenance",
		"backup-health",
		"backup-catalog",
		"show-backup",
		"restore-drill",
		"rollback",
		"update",
		"verify-backup",
		"migrate-backup",
	} {
		if _, ok := commands[forbidden]; ok {
			t.Fatalf("expected removed command %q to be absent", forbidden)
		}
	}
}

func TestBackupCommandExposesOnlyVerifySubcommand(t *testing.T) {
	root := newTestRootCmd()
	backup, _, err := root.Find([]string{"backup"})
	if err != nil {
		t.Fatalf("find backup command: %v", err)
	}
	subcommands := map[string]struct{}{}
	for _, cmd := range backup.Commands() {
		subcommands[cmd.Name()] = struct{}{}
	}
	if _, ok := subcommands["verify"]; !ok {
		t.Fatalf("expected backup verify subcommand to be present")
	}
	for _, forbidden := range []string{"audit", "catalog", "show"} {
		if _, ok := subcommands[forbidden]; ok {
			t.Fatalf("expected removed backup subcommand %q to be absent", forbidden)
		}
	}
}
