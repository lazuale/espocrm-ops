package cli

import (
	"path/filepath"
	"reflect"
	"slices"
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

	commands := make([]string, 0, len(root.Commands()))
	for _, cmd := range root.Commands() {
		commands = append(commands, cmd.Name())
	}
	slices.Sort(commands)

	expected := []string{"backup", "doctor", "migrate", "restore"}
	if !reflect.DeepEqual(commands, expected) {
		t.Fatalf("unexpected root commands: got %v want %v", commands, expected)
	}
}

func TestBackupCommandExposesOnlyVerifySubcommand(t *testing.T) {
	root := newTestRootCmd()
	backup, _, err := root.Find([]string{"backup"})
	if err != nil {
		t.Fatalf("find backup command: %v", err)
	}
	subcommands := make([]string, 0, len(backup.Commands()))
	for _, cmd := range backup.Commands() {
		subcommands = append(subcommands, cmd.Name())
	}
	slices.Sort(subcommands)
	if !reflect.DeepEqual(subcommands, []string{"verify"}) {
		t.Fatalf("unexpected backup subcommands: got %v want [verify]", subcommands)
	}
}
