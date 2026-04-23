package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	errortransport "github.com/lazuale/espocrm-ops/internal/cli/errortransport"
	"github.com/lazuale/espocrm-ops/internal/contract/exitcode"
	"github.com/spf13/cobra"
)

func newBackupVerifyValidationCmd() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Flags().String("manifest", "", "")
	cmd.Flags().String("backup-root", "", "")
	return cmd
}

func TestValidateBackupVerifyInput_NormalizesManifestPath(t *testing.T) {
	cmd := newBackupVerifyValidationCmd()
	raw := "  " + filepath.Join(".", "fixtures", "..", "manifest.json") + "  "
	if err := cmd.Flags().Set("manifest", raw); err != nil {
		t.Fatal(err)
	}

	in := backupVerifyInput{manifestPath: raw}
	if err := validateBackupVerifyInput(cmd, &in); err != nil {
		t.Fatalf("validateBackupVerifyInput returned error: %v", err)
	}

	want, err := filepath.Abs(filepath.Clean(filepath.Join(".", "fixtures", "..", "manifest.json")))
	if err != nil {
		t.Fatal(err)
	}
	if in.manifestPath != want {
		t.Fatalf("unexpected manifest path: got %q want %q", in.manifestPath, want)
	}
	if in.backupRoot != "" {
		t.Fatalf("expected empty backup root, got %q", in.backupRoot)
	}
}

func TestValidateBackupVerifyInput_NormalizesBackupRoot(t *testing.T) {
	cmd := newBackupVerifyValidationCmd()
	raw := "  " + filepath.Join(".", "backups", "..", "backup-root") + "  "
	if err := cmd.Flags().Set("backup-root", raw); err != nil {
		t.Fatal(err)
	}

	in := backupVerifyInput{backupRoot: raw}
	if err := validateBackupVerifyInput(cmd, &in); err != nil {
		t.Fatalf("validateBackupVerifyInput returned error: %v", err)
	}

	want, err := filepath.Abs(filepath.Clean(filepath.Join(".", "backups", "..", "backup-root")))
	if err != nil {
		t.Fatal(err)
	}
	if in.backupRoot != want {
		t.Fatalf("unexpected backup root: got %q want %q", in.backupRoot, want)
	}
	if in.manifestPath != "" {
		t.Fatalf("expected empty manifest path, got %q", in.manifestPath)
	}
}

func TestValidateBackupVerifyInput_BlankAfterTrimIsUsageError(t *testing.T) {
	cmd := newBackupVerifyValidationCmd()
	if err := cmd.Flags().Set("manifest", "   "); err != nil {
		t.Fatal(err)
	}

	in := backupVerifyInput{manifestPath: "   "}
	err := validateBackupVerifyInput(cmd, &in)
	if err == nil {
		t.Fatal("expected error")
	}
	if !errortransport.IsUsageError(err) {
		t.Fatalf("expected usage error, got %v", err)
	}
	if !strings.Contains(err.Error(), "--manifest must not be blank") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateBackupVerifyInput_ManifestAndBackupRootConflictIsUsageError(t *testing.T) {
	cmd := newBackupVerifyValidationCmd()
	if err := cmd.Flags().Set("manifest", filepath.Join(".", "manifest.json")); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Flags().Set("backup-root", filepath.Join(".", "backups")); err != nil {
		t.Fatal(err)
	}

	in := backupVerifyInput{
		manifestPath: filepath.Join(".", "manifest.json"),
		backupRoot:   filepath.Join(".", "backups"),
	}
	err := validateBackupVerifyInput(cmd, &in)
	if err == nil {
		t.Fatal("expected error")
	}
	if !errortransport.IsUsageError(err) {
		t.Fatalf("expected usage error, got %v", err)
	}
	if !strings.Contains(err.Error(), "use either --manifest or --backup-root") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateBackupVerifyInput_RequiresManifestOrBackupRoot(t *testing.T) {
	cmd := newBackupVerifyValidationCmd()

	err := validateBackupVerifyInput(cmd, &backupVerifyInput{})
	if err == nil {
		t.Fatal("expected error")
	}
	if !errortransport.IsUsageError(err) {
		t.Fatalf("expected usage error, got %v", err)
	}
	if !strings.Contains(err.Error(), "--manifest or --backup-root is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSchema_BackupVerify_JSON_Error_MissingManifest_NoJournal(t *testing.T) {
	tmp := t.TempDir()
	journalDir := filepath.Join(tmp, "journal")

	outcome := executeCLI(
		"--journal-dir", journalDir,
		"--json",
		"backup",
		"verify",
	)

	assertUsageErrorOutput(t, outcome, "--manifest or --backup-root is required")
	assertNoJournalFiles(t, journalDir)
}

func TestSchema_BackupVerify_JSON_Error_BlankManifest_NoJournal(t *testing.T) {
	tmp := t.TempDir()
	journalDir := filepath.Join(tmp, "journal")

	outcome := executeCLI(
		"--journal-dir", journalDir,
		"--json",
		"backup",
		"verify",
		"--manifest", "   ",
	)

	assertUsageErrorOutput(t, outcome, "--manifest must not be blank")
	assertNoJournalFiles(t, journalDir)
}

func TestSchema_BackupVerify_JSON_Error_ManifestBackupRootConflict_NoJournal(t *testing.T) {
	tmp := t.TempDir()
	journalDir := filepath.Join(tmp, "journal")

	outcome := executeCLI(
		"--journal-dir", journalDir,
		"--json",
		"backup",
		"verify",
		"--manifest", filepath.Join(tmp, "manifest.json"),
		"--backup-root", filepath.Join(tmp, "backups"),
	)

	assertUsageErrorOutput(t, outcome, "use either --manifest or --backup-root")
	assertNoJournalFiles(t, journalDir)
}

func TestSchema_BackupVerify_JSON_Error_BlankBackupRoot_NoJournal(t *testing.T) {
	tmp := t.TempDir()
	journalDir := filepath.Join(tmp, "journal")

	outcome := executeCLI(
		"--journal-dir", journalDir,
		"--json",
		"backup",
		"verify",
		"--backup-root", "   ",
	)

	assertUsageErrorOutput(t, outcome, "--backup-root must not be blank")
	assertNoJournalFiles(t, journalDir)
}

func TestSchema_BackupVerify_JSON_Error_InvalidManifestContract(t *testing.T) {
	tmp := t.TempDir()
	journalDir := filepath.Join(tmp, "journal")
	manifestPath := filepath.Join(tmp, "manifest.json")
	writeJSON(t, manifestPath, map[string]any{
		"version": 2,
	})

	outcome := executeCLI(
		"--journal-dir", journalDir,
		"--json",
		"backup",
		"verify",
		"--manifest", manifestPath,
	)

	assertCLIErrorOutput(t, outcome, exitcode.ManifestError, "manifest_invalid", "manifest verify")
}

func TestSchema_BackupVerify_JSON_Error_ChecksumMismatchContract(t *testing.T) {
	tmp := t.TempDir()
	journalDir := filepath.Join(tmp, "journal")
	backupRoot := filepath.Join(tmp, "backups")

	set := writeBackupSet(t, backupRoot, "espocrm-prod", "2026-04-07_01-00-00", "prod", nil)
	if err := os.WriteFile(set.DBBackup+".sha256", []byte(strings.Repeat("0", 64)+"  "+filepath.Base(set.DBBackup)+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	outcome := executeCLI(
		"--journal-dir", journalDir,
		"--json",
		"backup",
		"verify",
		"--manifest", set.ManifestJSON,
	)

	assertCLIErrorOutput(t, outcome, exitcode.ValidationError, "backup_verification_failed", "artifact verify")
}

func TestSchema_BackupVerify_JSON_Error_BrokenArchiveContract(t *testing.T) {
	tmp := t.TempDir()
	journalDir := filepath.Join(tmp, "journal")
	backupRoot := filepath.Join(tmp, "backups")

	set := writeBackupSet(t, backupRoot, "espocrm-prod", "2026-04-07_01-00-00", "prod", nil)
	writeGzipFile(t, set.FilesBackup, []byte("not a tar stream"))
	writeBackupVerifyReferenceManifest(t, set, sha256OfFile(t, set.DBBackup), sha256OfFile(t, set.FilesBackup))
	if err := os.WriteFile(set.FilesBackup+".sha256", []byte(sha256OfFile(t, set.FilesBackup)+"  "+filepath.Base(set.FilesBackup)+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	outcome := executeCLI(
		"--journal-dir", journalDir,
		"--json",
		"backup",
		"verify",
		"--manifest", set.ManifestJSON,
	)

	assertCLIErrorOutput(t, outcome, exitcode.ValidationError, "backup_verification_failed", "artifact verify")
}

func TestSchema_BackupVerify_JSON_Error_MissingArtifactContract(t *testing.T) {
	tmp := t.TempDir()
	journalDir := filepath.Join(tmp, "journal")
	backupRoot := filepath.Join(tmp, "backups")

	set := writeBackupSet(t, backupRoot, "espocrm-prod", "2026-04-07_01-00-00", "prod", nil)
	if err := os.Remove(set.DBBackup); err != nil {
		t.Fatal(err)
	}

	outcome := executeCLI(
		"--journal-dir", journalDir,
		"--json",
		"backup",
		"verify",
		"--manifest", set.ManifestJSON,
	)

	assertCLIErrorOutput(t, outcome, exitcode.ValidationError, "backup_verification_failed", "artifact verify")
}

func TestSchema_BackupVerify_JSON_Error_BackupRootNoCompleteSet(t *testing.T) {
	tmp := t.TempDir()
	journalDir := filepath.Join(tmp, "journal")
	backupRoot := filepath.Join(tmp, "backups")
	writeBackupVerifyPartialManifest(t, backupRoot, "espocrm-prod", "2026-04-07_01-00-00")

	outcome := executeCLI(
		"--journal-dir", journalDir,
		"--json",
		"backup",
		"verify",
		"--backup-root", backupRoot,
	)

	assertCLIErrorOutput(t, outcome, exitcode.ValidationError, "backup_verification_failed", "complete backup-set")
}
