package cli

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lazuale/espocrm-ops/internal/contract/exitcode"
	"github.com/lazuale/espocrm-ops/internal/platform/locks"
)

func TestSchema_RestoreFiles_JSON_Error_MissingManifest_NoJournal(t *testing.T) {
	tmp := t.TempDir()
	journalDir := filepath.Join(tmp, "journal")

	outcome := executeCLI(
		"--journal-dir", journalDir,
		"--json",
		"restore-files",
		"--target-dir", filepath.Join(tmp, "storage"),
		"--dry-run",
	)

	assertUsageErrorOutput(t, outcome, "--manifest or --files-backup is required")
	assertNoJournalFiles(t, journalDir)
}

func TestSchema_RestoreFiles_JSON_Error_RejectsRootTarget_NoJournal(t *testing.T) {
	tmp := t.TempDir()
	journalDir := filepath.Join(tmp, "journal")

	outcome := executeCLI(
		"--journal-dir", journalDir,
		"--json",
		"restore-files",
		"--manifest", filepath.Join(tmp, "manifest.json"),
		"--target-dir", string(os.PathSeparator),
		"--dry-run",
	)

	assertUsageErrorOutput(t, outcome, "--target-dir must not be the filesystem root")
	assertNoJournalFiles(t, journalDir)
}

func TestSchema_RestoreFiles_JSON_Error_BlankTargetDir_NoJournal(t *testing.T) {
	tmp := t.TempDir()
	journalDir := filepath.Join(tmp, "journal")

	outcome := executeCLI(
		"--journal-dir", journalDir,
		"--json",
		"restore-files",
		"--manifest", filepath.Join(tmp, "manifest.json"),
		"--target-dir", "   ",
		"--dry-run",
	)

	assertUsageErrorOutput(t, outcome, "--target-dir is required")
	assertNoJournalFiles(t, journalDir)
}

func TestSchema_RestoreFiles_JSON_Error_LockHeld(t *testing.T) {
	tmp := t.TempDir()
	journalDir := filepath.Join(tmp, "journal")
	lockDir := filepath.Join(tmp, "locks")
	restoreLockDir := locks.SetLockDirForTest(lockDir)
	t.Cleanup(restoreLockDir)
	if err := os.MkdirAll(lockDir, 0o755); err != nil {
		t.Fatal(err)
	}

	dbName := "db.sql.gz"
	filesName := "files.tar.gz"
	manifestPath := filepath.Join(tmp, "manifest.json")
	dbPath := filepath.Join(tmp, dbName)
	filesPath := filepath.Join(tmp, filesName)
	targetDir := filepath.Join(tmp, "storage")
	stalePath := filepath.Join(targetDir, "stale.txt")
	restoredPath := filepath.Join(targetDir, "storage", "a.txt")

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
			"db_backup":    sha256OfFile(t, dbPath),
			"files_backup": sha256OfFile(t, filesPath),
		},
	})
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(stalePath, []byte("stale"), 0o644); err != nil {
		t.Fatal(err)
	}
	lock, err := locks.AcquireRestoreFilesLock()
	if err != nil {
		t.Fatalf("acquire restore-files lock failed: %v", err)
	}
	defer lock.Release()

	outcome := executeCLI(
		"--journal-dir", journalDir,
		"--json",
		"restore-files",
		"--manifest", manifestPath,
		"--target-dir", targetDir,
		"--dry-run",
	)

	assertCLIErrorOutput(t, outcome, exitcode.RestoreError, "lock_acquire_failed", "files restore lock failed")
	assertPathExists(t, stalePath)
	assertPathMissing(t, restoredPath)
}

func TestSchema_RestoreFiles_JSON_Error_TargetParentFilesystemFailure(t *testing.T) {
	tmp := t.TempDir()
	journalDir := filepath.Join(tmp, "journal")
	filesPath := filepath.Join(tmp, "files.tar.gz")
	blocker := filepath.Join(tmp, "blocker")
	targetDir := filepath.Join(blocker, "storage")
	sidecarPath := filesPath + ".sha256"

	writeTarGzFile(t, filesPath, map[string]string{
		"storage/a.txt": "hello",
	})
	body := sha256OfFile(t, filesPath) + "  " + filepath.Base(filesPath) + "\n"
	if err := os.WriteFile(sidecarPath, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(blocker, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	outcome := executeCLI(
		"--journal-dir", journalDir,
		"--json",
		"restore-files",
		"--files-backup", filesPath,
		"--target-dir", targetDir,
		"--dry-run",
	)

	assertCLIErrorOutput(t, outcome, exitcode.FilesystemError, "filesystem_error", "ensure target parent dir")

	var obj map[string]any
	if err := json.Unmarshal([]byte(outcome.Stdout), &obj); err != nil {
		t.Fatal(err)
	}
	if kind := requireJSONPath(t, obj, "error", "kind"); kind != "io" {
		t.Fatalf("expected error.kind=io, got %v", kind)
	}
}

func TestSchema_RestoreFiles_JSON_Error_RuntimeArchiveRootMismatch(t *testing.T) {
	tmp := t.TempDir()
	journalDir := filepath.Join(tmp, "journal")
	filesPath := filepath.Join(tmp, "files.tar.gz")
	targetDir := filepath.Join(tmp, "storage")
	sidecarPath := filesPath + ".sha256"

	writeTarGzFile(t, filesPath, map[string]string{
		"wrong/a.txt": "hello",
	})
	body := sha256OfFile(t, filesPath) + "  " + filepath.Base(filesPath) + "\n"
	if err := os.WriteFile(sidecarPath, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	outcome := executeCLI(
		"--journal-dir", journalDir,
		"--json",
		"restore-files",
		"--files-backup", filesPath,
		"--target-dir", targetDir,
	)

	assertCLIErrorOutput(t, outcome, exitcode.RestoreError, "restore_files_failed", "archive root must be exactly")

	var obj map[string]any
	if err := json.Unmarshal([]byte(outcome.Stdout), &obj); err != nil {
		t.Fatal(err)
	}
	if kind := requireJSONPath(t, obj, "error", "kind"); kind != "restore" {
		t.Fatalf("expected error.kind=restore, got %v", kind)
	}
}

func TestSchema_RestoreFiles_JSON_Error_RuntimeArchiveEntryConflict(t *testing.T) {
	tmp := t.TempDir()
	journalDir := filepath.Join(tmp, "journal")
	filesPath := filepath.Join(tmp, "files.tar.gz")
	targetDir := filepath.Join(tmp, "storage")
	sidecarPath := filesPath + ".sha256"

	writeOrderedTarGzFile(t, filesPath,
		tar.Header{
			Name: "storage/a.txt",
			Mode: 0o644,
			Size: int64(len("hello")),
		}, []byte("hello"),
		tar.Header{
			Name: "storage/a.txt/b.txt",
			Mode: 0o644,
			Size: int64(len("world")),
		}, []byte("world"),
	)
	body := sha256OfFile(t, filesPath) + "  " + filepath.Base(filesPath) + "\n"
	if err := os.WriteFile(sidecarPath, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	outcome := executeCLI(
		"--journal-dir", journalDir,
		"--json",
		"restore-files",
		"--files-backup", filesPath,
		"--target-dir", targetDir,
	)

	assertCLIErrorOutput(t, outcome, exitcode.RestoreError, "restore_files_failed", "archive entry conflicts with existing path")

	var obj map[string]any
	if err := json.Unmarshal([]byte(outcome.Stdout), &obj); err != nil {
		t.Fatal(err)
	}
	if kind := requireJSONPath(t, obj, "error", "kind"); kind != "restore" {
		t.Fatalf("expected error.kind=restore, got %v", kind)
	}
}

func TestSchema_RestoreFiles_JSON_Error_DirectArchiveMissingSidecarIsBackupVerificationFailure(t *testing.T) {
	tmp := t.TempDir()
	journalDir := filepath.Join(tmp, "journal")
	filesPath := filepath.Join(tmp, "files.tar.gz")
	targetDir := filepath.Join(tmp, "storage")

	writeTarGzFile(t, filesPath, map[string]string{
		"storage/a.txt": "hello",
	})

	outcome := executeCLI(
		"--journal-dir", journalDir,
		"--json",
		"restore-files",
		"--files-backup", filesPath,
		"--target-dir", targetDir,
		"--dry-run",
	)

	assertCLIErrorOutput(t, outcome, exitcode.ValidationError, "backup_verification_failed", "files backup checksum")

	var obj map[string]any
	if err := json.Unmarshal([]byte(outcome.Stdout), &obj); err != nil {
		t.Fatal(err)
	}
	if kind := requireJSONPath(t, obj, "error", "kind"); kind != "validation" {
		t.Fatalf("expected error.kind=validation, got %v", kind)
	}
}

func TestSchema_RestoreDB_JSON_Error_MissingPasswordSource_NoJournal(t *testing.T) {
	t.Setenv("ESPOPS_DB_PASSWORD", "")
	t.Setenv("DB_PASSWORD", "")
	t.Setenv("ESPOPS_DB_PASSWORD_FILE", "")

	tmp := t.TempDir()
	journalDir := filepath.Join(tmp, "journal")

	outcome := executeCLI(
		"--journal-dir", journalDir,
		"--json",
		"restore-db",
		"--manifest", filepath.Join(tmp, "manifest.json"),
		"--db-container", "espocrm-db",
		"--db-name", "espocrm",
		"--db-user", "espocrm",
		"--dry-run",
	)

	assertUsageErrorOutput(t, outcome, "database password source is required")
	assertNoJournalFiles(t, journalDir)
}

func TestSchema_RestoreDB_JSON_Error_EnvPasswordPassesCLIValidation(t *testing.T) {
	t.Setenv("ESPOPS_DB_PASSWORD", "env-secret")
	t.Setenv("DB_PASSWORD", "")
	t.Setenv("ESPOPS_DB_PASSWORD_FILE", "")

	tmp := t.TempDir()
	journalDir := filepath.Join(tmp, "journal")
	manifestPath := filepath.Join(tmp, "missing-manifest.json")

	outcome := executeCLI(
		"--journal-dir", journalDir,
		"--json",
		"restore-db",
		"--manifest", manifestPath,
		"--db-container", "espocrm-db",
		"--db-name", "espocrm",
		"--db-user", "espocrm",
		"--dry-run",
	)

	assertCLIErrorOutput(t, outcome, exitcode.ManifestError, "manifest_invalid", "read manifest")
}

func TestSchema_RestoreDB_JSON_Error_PasswordConflict_NoJournal(t *testing.T) {
	t.Setenv("ESPOPS_DB_PASSWORD", "")
	t.Setenv("DB_PASSWORD", "")
	t.Setenv("ESPOPS_DB_PASSWORD_FILE", "")

	tmp := t.TempDir()
	journalDir := filepath.Join(tmp, "journal")

	outcome := executeCLI(
		"--journal-dir", journalDir,
		"--json",
		"restore-db",
		"--manifest", filepath.Join(tmp, "manifest.json"),
		"--db-container", "espocrm-db",
		"--db-name", "espocrm",
		"--db-user", "espocrm",
		"--db-password", "secret",
		"--db-password-file", filepath.Join(tmp, "secret"),
		"--dry-run",
	)

	assertUsageErrorOutput(t, outcome, "use either --db-password or --db-password-file")
	assertNoJournalFiles(t, journalDir)
}

func TestSchema_RestoreDB_JSON_Error_PasswordFileReadIsFilesystemError(t *testing.T) {
	tmp := t.TempDir()
	journalDir := filepath.Join(tmp, "journal")
	missingPasswordPath := filepath.Join(tmp, "missing-secret")

	outcome := executeCLI(
		"--journal-dir", journalDir,
		"--json",
		"restore-db",
		"--manifest", filepath.Join(tmp, "manifest.json"),
		"--db-container", "espocrm-db",
		"--db-name", "espocrm",
		"--db-user", "espocrm",
		"--db-password-file", missingPasswordPath,
		"--dry-run",
	)

	assertCLIErrorOutput(t, outcome, exitcode.FilesystemError, "filesystem_error", "read password file")

	var obj map[string]any
	if err := json.Unmarshal([]byte(outcome.Stdout), &obj); err != nil {
		t.Fatal(err)
	}
	if kind := requireJSONPath(t, obj, "error", "kind"); kind != "io" {
		t.Fatalf("expected error.kind=io, got %v", kind)
	}
}

func TestSchema_RestoreDB_JSON_Error_BlankRequiredFlag_NoJournal(t *testing.T) {
	tmp := t.TempDir()
	journalDir := filepath.Join(tmp, "journal")

	outcome := executeCLI(
		"--journal-dir", journalDir,
		"--json",
		"restore-db",
		"--manifest", filepath.Join(tmp, "manifest.json"),
		"--db-container", "espocrm-db",
		"--db-name", "espocrm",
		"--db-user", "   ",
		"--db-password-file", filepath.Join(tmp, "secret"),
		"--dry-run",
	)

	assertUsageErrorOutput(t, outcome, "--db-user is required")
	assertNoJournalFiles(t, journalDir)
}

func TestSchema_RestoreDB_JSON_Error_LockHeld(t *testing.T) {
	tmp := t.TempDir()
	journalDir := filepath.Join(tmp, "journal")
	lockDir := filepath.Join(tmp, "locks")
	restoreLockDir := locks.SetLockDirForTest(lockDir)
	t.Cleanup(restoreLockDir)
	if err := os.MkdirAll(lockDir, 0o755); err != nil {
		t.Fatal(err)
	}

	dbName := "db.sql.gz"
	filesName := "files.tar.gz"
	manifestPath := filepath.Join(tmp, "manifest.json")
	dbPath := filepath.Join(tmp, dbName)
	filesPath := filepath.Join(tmp, filesName)
	passwordPath := filepath.Join(tmp, "secret")

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
			"db_backup":    sha256OfFile(t, dbPath),
			"files_backup": sha256OfFile(t, filesPath),
		},
	})
	if err := os.WriteFile(passwordPath, []byte("secret\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	prependFakeDockerForCLITest(t)
	lock, err := locks.AcquireRestoreDBLock()
	if err != nil {
		t.Fatalf("acquire restore-db lock failed: %v", err)
	}
	defer lock.Release()

	outcome := executeCLI(
		"--journal-dir", journalDir,
		"--json",
		"restore-db",
		"--manifest", manifestPath,
		"--db-container", "espocrm-db",
		"--db-name", "espocrm",
		"--db-user", "espocrm",
		"--db-password-file", passwordPath,
		"--dry-run",
	)

	assertCLIErrorOutput(t, outcome, exitcode.RestoreError, "lock_acquire_failed", "db restore lock failed")
}

func TestSchema_RestoreDB_JSON_Error_DirectBackupInvalidGzipIsBackupVerificationFailure(t *testing.T) {
	tmp := t.TempDir()
	journalDir := filepath.Join(tmp, "journal")
	dbPath := filepath.Join(tmp, "db.sql.gz")
	sidecarPath := dbPath + ".sha256"

	if err := os.WriteFile(dbPath, []byte("not gzip"), 0o644); err != nil {
		t.Fatal(err)
	}
	body := sha256OfFile(t, dbPath) + "  " + filepath.Base(dbPath) + "\n"
	if err := os.WriteFile(sidecarPath, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	outcome := executeCLI(
		"--journal-dir", journalDir,
		"--json",
		"restore-db",
		"--db-backup", dbPath,
		"--db-container", "espocrm-db",
		"--db-name", "espocrm",
		"--db-user", "espocrm",
		"--db-password", "secret",
		"--db-root-password", "root-secret",
		"--dry-run",
	)

	assertCLIErrorOutput(t, outcome, exitcode.ValidationError, "backup_verification_failed", "db backup gzip validation failed")

	var obj map[string]any
	if err := json.Unmarshal([]byte(outcome.Stdout), &obj); err != nil {
		t.Fatal(err)
	}
	if kind := requireJSONPath(t, obj, "error", "kind"); kind != "validation" {
		t.Fatalf("expected error.kind=validation, got %v", kind)
	}
}

func TestSchema_RestoreDB_JSON_Error_RuntimeDockerFailure(t *testing.T) {
	tmp := t.TempDir()
	journalDir := filepath.Join(tmp, "journal")
	dbPath := filepath.Join(tmp, "db.sql.gz")

	writeGzipFile(t, dbPath, []byte("select 1;"))
	body := sha256OfFile(t, dbPath) + "  " + filepath.Base(dbPath) + "\n"
	if err := os.WriteFile(dbPath+".sha256", []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	prependFakeDockerForCLITest(t)
	t.Setenv("DOCKER_EXEC_STDERR", "permission denied")
	t.Setenv("DOCKER_EXEC_EXIT_CODE", "23")

	outcome := executeCLI(
		"--journal-dir", journalDir,
		"--json",
		"restore-db",
		"--db-backup", dbPath,
		"--db-container", "espocrm-db",
		"--db-name", "espocrm",
		"--db-user", "espocrm",
		"--db-password", "secret",
		"--db-root-password", "root-secret",
	)

	assertCLIErrorOutput(t, outcome, exitcode.RestoreError, "restore_db_failed", "permission denied")

	var obj map[string]any
	if err := json.Unmarshal([]byte(outcome.Stdout), &obj); err != nil {
		t.Fatal(err)
	}
	if kind := requireJSONPath(t, obj, "error", "kind"); kind != "external" {
		t.Fatalf("expected error.kind=external, got %v", kind)
	}
}

func assertUsageErrorOutput(t *testing.T, outcome execOutcome, messagePart string) {
	t.Helper()

	assertCLIErrorOutput(t, outcome, exitcode.UsageError, "usage_error", messagePart)
}

func assertCLIErrorOutput(t *testing.T, outcome execOutcome, wantExitCode int, wantErrorCode, messagePart string) {
	t.Helper()

	if outcome.ExitCode != wantExitCode {
		t.Fatalf("expected exit code %d, got %d stdout=%s stderr=%s", wantExitCode, outcome.ExitCode, outcome.Stdout, outcome.Stderr)
	}

	var obj map[string]any
	if err := json.Unmarshal([]byte(outcome.Stdout), &obj); err != nil {
		t.Fatal(err)
	}

	requireJSONPath(t, obj, "command")
	requireJSONPath(t, obj, "ok")
	requireJSONPath(t, obj, "error", "code")
	requireJSONPath(t, obj, "error", "exit_code")
	requireJSONPath(t, obj, "error", "message")

	if ok, _ := obj["ok"].(bool); ok {
		t.Fatalf("expected ok=false")
	}
	if code := requireJSONPath(t, obj, "error", "code"); code != wantErrorCode {
		t.Fatalf("expected %s, got %v", wantErrorCode, code)
	}
	exitCode, _ := requireJSONPath(t, obj, "error", "exit_code").(float64)
	if int(exitCode) != wantExitCode {
		t.Fatalf("expected json error exit_code %d, got %v", wantExitCode, exitCode)
	}
	message, _ := requireJSONPath(t, obj, "error", "message").(string)
	if !strings.Contains(message, messagePart) {
		t.Fatalf("expected error message to contain %q, got %q", messagePart, message)
	}
}

func assertNoJournalFiles(t *testing.T, journalDir string) {
	t.Helper()

	var paths []string
	err := filepath.WalkDir(journalDir, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() && filepath.Ext(entry.Name()) == ".json" {
			paths = append(paths, path)
		}
		return nil
	})
	if os.IsNotExist(err) {
		return
	}
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) != 0 {
		t.Fatalf("expected no journal files, got %+v", paths)
	}
}

func writeOrderedTarGzFile(t *testing.T, path string, entries ...any) {
	t.Helper()

	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	gz := gzip.NewWriter(f)
	defer gz.Close()

	tw := tar.NewWriter(gz)
	defer tw.Close()

	for i := 0; i < len(entries); i += 2 {
		hdr, ok := entries[i].(tar.Header)
		if !ok {
			t.Fatalf("entry %d header has type %T", i, entries[i])
		}
		if err := tw.WriteHeader(&hdr); err != nil {
			t.Fatal(err)
		}
		if body, _ := entries[i+1].([]byte); len(body) != 0 {
			if _, err := tw.Write(body); err != nil {
				t.Fatal(err)
			}
		}
	}
}
