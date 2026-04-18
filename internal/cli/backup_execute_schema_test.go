package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	platformfs "github.com/lazuale/espocrm-ops/internal/platform/fs"
)

func TestSchema_BackupExecute_JSON_FilesOnlyNoStop(t *testing.T) {
	tmp := t.TempDir()
	journalDir := filepath.Join(tmp, "journal")
	projectDir := filepath.Join(tmp, "project")
	composeFile := filepath.Join(projectDir, "compose.yaml")
	envFile := filepath.Join(projectDir, ".env.dev")
	backupRoot := filepath.Join(tmp, "backups")
	storageDir := filepath.Join(tmp, "storage", "espo")
	mockBinDir := filepath.Join(tmp, "bin")

	useJournalClockForTest(t, time.Date(2026, 4, 15, 11, 0, 0, 0, time.UTC))

	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(storageDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(mockBinDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(storageDir, "file.txt"), []byte("hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(composeFile, []byte("services: {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(envFile, []byte("COMPOSE_PROJECT_NAME=espocrm-test\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(mockBinDir, "docker"), []byte("#!/usr/bin/env bash\nset -Eeuo pipefail\necho 'docker must not be called' >&2\nexit 97\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	t.Setenv("PATH", mockBinDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("COMPOSE_PROJECT_NAME", "espocrm-test")
	t.Setenv("BACKUP_ROOT", backupRoot)
	t.Setenv("ESPO_STORAGE_DIR", storageDir)
	t.Setenv("BACKUP_NAME_PREFIX", "espocrm-test-dev")
	t.Setenv("BACKUP_RETENTION_DAYS", "7")

	out, err := runRootCommand(
		t,
		"--journal-dir", journalDir,
		"--json",
		"backup-exec",
		"--scope", "dev",
		"--project-dir", projectDir,
		"--compose-file", composeFile,
		"--env-file", envFile,
		"--skip-db",
		"--no-stop",
	)
	if err != nil {
		t.Fatalf("command failed: %v\noutput=%s", err, out)
	}

	var obj map[string]any
	if err := json.Unmarshal([]byte(out), &obj); err != nil {
		t.Fatal(err)
	}

	requireJSONPath(t, obj, "command")
	requireJSONPath(t, obj, "ok")
	requireJSONPath(t, obj, "message")
	requireJSONPath(t, obj, "details", "scope")
	requireJSONPath(t, obj, "details", "created_at")
	requireJSONPath(t, obj, "details", "skip_db")
	requireJSONPath(t, obj, "details", "skip_files")
	requireJSONPath(t, obj, "details", "no_stop")
	requireJSONPath(t, obj, "details", "consistent_snapshot")
	requireJSONPath(t, obj, "artifacts", "manifest_txt")
	requireJSONPath(t, obj, "artifacts", "manifest_json")
	requireJSONPath(t, obj, "artifacts", "files_backup")
	requireJSONPath(t, obj, "artifacts", "files_checksum")

	if obj["command"] != "backup-exec" {
		t.Fatalf("unexpected command: %v", obj["command"])
	}
	if message := obj["message"]; message != "backup completed" {
		t.Fatalf("unexpected message: %v", message)
	}
	if createdAt := requireJSONPath(t, obj, "details", "created_at"); createdAt != "2026-04-15T11:00:00Z" {
		t.Fatalf("unexpected created_at: %v", createdAt)
	}
	if consistent, _ := requireJSONPath(t, obj, "details", "consistent_snapshot").(bool); consistent {
		t.Fatalf("expected consistent_snapshot=false, got %v", consistent)
	}

	filesBackup, _ := requireJSONPath(t, obj, "artifacts", "files_backup").(string)
	if err := platformfs.VerifyTarGzReadable(filesBackup, nil); err != nil {
		t.Fatalf("expected readable files backup: %v", err)
	}
	for _, key := range []string{"manifest_txt", "manifest_json", "files_backup", "files_checksum"} {
		path, _ := requireJSONPath(t, obj, "artifacts", key).(string)
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected artifact %s at %s: %v", key, path, err)
		}
	}

	assertNoJournalFiles(t, journalDir)
}

func TestSchema_BackupExecute_JSON_Error_MissingBackupRoot_NoJournal(t *testing.T) {
	tmp := t.TempDir()
	journalDir := filepath.Join(tmp, "journal")
	t.Setenv("COMPOSE_PROJECT_NAME", "espocrm-test")
	t.Setenv("ESPO_STORAGE_DIR", filepath.Join(tmp, "storage", "espo"))

	outcome := executeCLI(
		"--journal-dir", journalDir,
		"--json",
		"backup-exec",
		"--scope", "dev",
		"--project-dir", tmp,
		"--compose-file", filepath.Join(tmp, "compose.yaml"),
		"--env-file", filepath.Join(tmp, ".env"),
		"--skip-db",
		"--no-stop",
	)

	assertUsageErrorOutput(t, outcome, "BACKUP_ROOT is required")
	assertNoJournalFiles(t, journalDir)
}
