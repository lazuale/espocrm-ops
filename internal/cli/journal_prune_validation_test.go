package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/lazuale/espocrm-ops/internal/contract/exitcode"
	"github.com/lazuale/espocrm-ops/internal/platform/locks"
)

func TestSchema_JournalPrune_JSON_Error_MissingRetentionPolicy(t *testing.T) {
	tmp := t.TempDir()
	journalDir := filepath.Join(tmp, "journal")
	emptyDir := filepath.Join(journalDir, "2026-04-01")
	if err := os.MkdirAll(emptyDir, 0o755); err != nil {
		t.Fatal(err)
	}

	outcome := executeCLI(
		"--journal-dir", journalDir,
		"--json",
		"journal-prune",
	)

	assertUsageErrorOutput(t, outcome, "provide --keep-days or --keep")
	if _, err := os.Stat(emptyDir); err != nil {
		t.Fatalf("invalid prune request should not remove directories: %v", err)
	}
}

func TestSchema_JournalPrune_JSON_Error_NegativeRetention(t *testing.T) {
	for _, tc := range []struct {
		name        string
		args        []string
		messagePart string
	}{
		{
			name:        "keep-days",
			args:        []string{"--keep-days", "-1"},
			messagePart: "--keep-days must be non-negative",
		},
		{
			name:        "keep",
			args:        []string{"--keep", "-1"},
			messagePart: "--keep must be non-negative",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tmp := t.TempDir()
			journalDir := filepath.Join(tmp, "journal")
			emptyDir := filepath.Join(journalDir, "2026-04-01")
			if err := os.MkdirAll(emptyDir, 0o755); err != nil {
				t.Fatal(err)
			}

			args := append([]string{
				"--journal-dir", journalDir,
				"--json",
				"journal-prune",
			}, tc.args...)

			outcome := executeCLI(args...)

			assertUsageErrorOutput(t, outcome, tc.messagePart)
			if _, err := os.Stat(emptyDir); err != nil {
				t.Fatalf("invalid prune request should not remove directories: %v", err)
			}
		})
	}
}

func TestSchema_JournalPrune_JSON_Error_LockHeld(t *testing.T) {
	tmp := t.TempDir()
	journalDir := filepath.Join(tmp, "journal")
	if err := os.MkdirAll(journalDir, 0o755); err != nil {
		t.Fatal(err)
	}

	lock, err := locks.AcquireJournalPruneLock(journalDir)
	if err != nil {
		t.Fatalf("acquire prune lock failed: %v", err)
	}
	defer func() {
		if releaseErr := lock.Release(); releaseErr != nil {
			t.Fatalf("release prune lock failed: %v", releaseErr)
		}
	}()

	outcome := executeCLI(
		"--journal-dir", journalDir,
		"--json",
		"journal-prune",
		"--keep", "10",
	)

	assertCLIErrorOutput(t, outcome, exitcode.RestoreError, "lock_acquire_failed", "journal prune lock failed")
}
