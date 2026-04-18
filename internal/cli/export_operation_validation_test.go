package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/lazuale/espocrm-ops/internal/contract/exitcode"
)

func TestSchema_ExportOperation_JSON_Error_BlankID(t *testing.T) {
	tmp := t.TempDir()
	journalDir := filepath.Join(tmp, "journal")
	outputPath := filepath.Join(tmp, "bundle.json")

	outcome := executeCLI(
		"--journal-dir", journalDir,
		"--json",
		"export-operation",
		"--id", "   ",
		"--output", outputPath,
	)

	assertUsageErrorOutput(t, outcome, "--id is required")
	assertPathMissing(t, outputPath)
	assertNoJournalFiles(t, journalDir)
}

func TestSchema_ExportOperation_JSON_Error_OutputRequired(t *testing.T) {
	tmp := t.TempDir()
	journalDir := filepath.Join(tmp, "journal")

	outcome := executeCLI(
		"--journal-dir", journalDir,
		"--json",
		"export-operation",
		"--id", "op-1",
	)

	assertUsageErrorOutput(t, outcome, "--output is required")
	assertNoJournalFiles(t, journalDir)
}

func TestSchema_ExportOperation_JSON_Error_InvalidOutputPath(t *testing.T) {
	for _, tc := range []struct {
		name        string
		outputPath  string
		messagePart string
	}{
		{
			name:        "current-dir",
			outputPath:  ".",
			messagePart: "--output must not be the current directory",
		},
		{
			name:        "root",
			outputPath:  string(os.PathSeparator),
			messagePart: "--output must not be the filesystem root",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tmp := t.TempDir()
			journalDir := filepath.Join(tmp, "journal")

			outcome := executeCLI(
				"--journal-dir", journalDir,
				"--json",
				"export-operation",
				"--id", "op-1",
				"--output", tc.outputPath,
			)

			assertUsageErrorOutput(t, outcome, tc.messagePart)
			assertNoJournalFiles(t, journalDir)
		})
	}
}

func TestSchema_ExportOperation_JSON_Error_NotFound(t *testing.T) {
	tmp := t.TempDir()
	journalDir := filepath.Join(tmp, "journal")
	outputPath := filepath.Join(tmp, "bundle.json")
	if err := os.MkdirAll(journalDir, 0o755); err != nil {
		t.Fatal(err)
	}

	outcome := executeCLI(
		"--journal-dir", journalDir,
		"--json",
		"export-operation",
		"--id", "missing-op",
		"--output", outputPath,
	)

	assertCLIErrorOutput(t, outcome, exitcode.ValidationError, "operation_not_found", "operation not found: missing-op")
	assertPathMissing(t, outputPath)
}

func TestSchema_ExportOperation_JSON_Error_OutputWriteFailure(t *testing.T) {
	tmp := t.TempDir()
	journalDir := filepath.Join(tmp, "journal")
	outputDir := filepath.Join(tmp, "bundle-dir")
	if err := os.MkdirAll(journalDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeJournalEntryFile(t, journalDir, "op.json", map[string]any{
		"operation_id": "op-export-write-fail",
		"command":      "verify-backup",
		"started_at":   "2026-04-19T10:00:00Z",
		"finished_at":  "2026-04-19T10:00:01Z",
		"ok":           true,
	})

	outcome := executeCLI(
		"--journal-dir", journalDir,
		"--json",
		"export-operation",
		"--id", "op-export-write-fail",
		"--output", outputDir,
	)

	assertCLIErrorOutput(t, outcome, exitcode.FilesystemError, "filesystem_error", "is a directory")
}
