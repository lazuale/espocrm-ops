package cli

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/lazuale/espocrm-ops/internal/contract/exitcode"
)

func TestSchema_VerifyBackup_JSON_Error_MissingManifest_NoJournal(t *testing.T) {
	tmp := t.TempDir()
	journalDir := filepath.Join(tmp, "journal")

	outcome := executeCLI(
		"--journal-dir", journalDir,
		"--json",
		"verify-backup",
	)

	assertUsageErrorOutput(t, outcome, "--manifest or --backup-root is required")
	assertNoJournalFiles(t, journalDir)
}

func TestSchema_VerifyBackup_JSON_Error_BlankManifest_NoJournal(t *testing.T) {
	tmp := t.TempDir()
	journalDir := filepath.Join(tmp, "journal")

	outcome := executeCLI(
		"--journal-dir", journalDir,
		"--json",
		"verify-backup",
		"--manifest", "   ",
	)

	assertUsageErrorOutput(t, outcome, "--manifest is required")
	assertNoJournalFiles(t, journalDir)
}

func TestSchema_VerifyBackup_JSON_Error_ManifestBackupRootConflict_NoJournal(t *testing.T) {
	tmp := t.TempDir()
	journalDir := filepath.Join(tmp, "journal")

	outcome := executeCLI(
		"--journal-dir", journalDir,
		"--json",
		"verify-backup",
		"--manifest", filepath.Join(tmp, "manifest.json"),
		"--backup-root", filepath.Join(tmp, "backups"),
	)

	assertUsageErrorOutput(t, outcome, "use either --manifest or --backup-root")
	assertNoJournalFiles(t, journalDir)
}

func TestSchema_VerifyBackup_JSON_Error_BlankBackupRoot_NoJournal(t *testing.T) {
	tmp := t.TempDir()
	journalDir := filepath.Join(tmp, "journal")

	outcome := executeCLI(
		"--journal-dir", journalDir,
		"--json",
		"verify-backup",
		"--backup-root", "   ",
	)

	assertUsageErrorOutput(t, outcome, "--backup-root is required")
	assertNoJournalFiles(t, journalDir)
}

func TestSchema_VerifyBackup_JSON_Error_InvalidManifestContract(t *testing.T) {
	tmp := t.TempDir()
	journalDir := filepath.Join(tmp, "journal")
	manifestPath := filepath.Join(tmp, "manifest.json")
	writeJSON(t, manifestPath, map[string]any{
		"version": 2,
	})

	outcome := executeCLI(
		"--journal-dir", journalDir,
		"--json",
		"verify-backup",
		"--manifest", manifestPath,
	)

	assertCLIErrorOutput(t, outcome, exitcode.ManifestError, "manifest_invalid", "validate manifest")
}

func TestSchema_VerifyBackup_JSON_Error_ChecksumMismatchContract(t *testing.T) {
	tmp := t.TempDir()
	journalDir := filepath.Join(tmp, "journal")

	dbName := "db.sql.gz"
	filesName := "files.tar.gz"
	dbPath := filepath.Join(tmp, dbName)
	filesPath := filepath.Join(tmp, filesName)
	manifestPath := filepath.Join(tmp, "manifest.json")

	writeGzipFile(t, dbPath, []byte("select 1;"))
	writeTarGzFile(t, filesPath, map[string]string{
		"storage/a.txt": "hello",
	})
	writeJSON(t, manifestPath, map[string]any{
		"version":    1,
		"scope":      "prod",
		"created_at": "2026-04-15T11:00:00Z",
		"artifacts": map[string]any{
			"db_backup":    dbName,
			"files_backup": filesName,
		},
		"checksums": map[string]any{
			"db_backup":    strings.Repeat("0", 64),
			"files_backup": sha256OfFile(t, filesPath),
		},
	})

	outcome := executeCLI(
		"--journal-dir", journalDir,
		"--json",
		"verify-backup",
		"--manifest", manifestPath,
	)

	assertCLIErrorOutput(t, outcome, exitcode.ValidationError, "backup_verification_failed", "checksum verification failed")
}
