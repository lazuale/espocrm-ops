package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/lazuale/espocrm-ops/internal/contract/exitcode"
	platformfs "github.com/lazuale/espocrm-ops/internal/platform/fs"
)

func TestSchema_BackupExecute_JSON_FilesOnlyNoStop(t *testing.T) {
	tmp := t.TempDir()
	journalDir := filepath.Join(tmp, "journal")
	projectDir := filepath.Join(tmp, "project")
	composeFile := filepath.Join(projectDir, "compose.yaml")
	storageDir := filepath.Join(projectDir, "runtime", "dev", "espo")
	mockBinDir := filepath.Join(tmp, "bin")

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
	if err := os.WriteFile(filepath.Join(mockBinDir, "docker"), []byte("#!/usr/bin/env bash\nset -Eeuo pipefail\necho 'docker must not be called' >&2\nexit 97\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	envFile := writeDoctorEnvFile(t, projectDir, "dev", map[string]string{
		"COMPOSE_PROJECT_NAME": "espocrm-test",
		"ESPO_STORAGE_DIR":     "./runtime/dev/espo",
		"BACKUP_ROOT":          "./backups/dev",
		"BACKUP_NAME_PREFIX":   "espocrm-test-dev",
	})

	t.Setenv("PATH", mockBinDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	out, err := runRootCommandWithOptions(
		t,
		[]testAppOption{
			withFixedTestRuntime(time.Date(2026, 4, 15, 11, 0, 0, 0, time.UTC), "op-backup-1"),
		},
		"--journal-dir", journalDir,
		"--json",
		"backup",
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

	if obj["command"] != "backup" {
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

	normalized := normalizeBackupJSON(t, []byte(out))
	assertGoldenJSON(t, normalized, filepath.Join("testdata", "backup_ok.golden.json"))
}

func TestSchema_BackupExecute_JSON_Error_MissingBackupRoot_NoJournal(t *testing.T) {
	tmp := t.TempDir()
	journalDir := filepath.Join(tmp, "journal")
	projectDir := filepath.Join(tmp, "project")
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, "compose.yaml"), []byte("services: {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	envFile := writeDoctorEnvFile(t, projectDir, "dev", map[string]string{
		"BACKUP_ROOT": "",
	})

	outcome := executeCLI(
		"--journal-dir", journalDir,
		"--json",
		"backup",
		"--scope", "dev",
		"--project-dir", projectDir,
		"--compose-file", filepath.Join(projectDir, "compose.yaml"),
		"--env-file", envFile,
		"--skip-db",
		"--no-stop",
	)

	assertCLIErrorOutput(t, outcome, exitcode.ValidationError, "backup_failed", "BACKUP_ROOT is not set")
}

func normalizeBackupJSON(t *testing.T, raw []byte) []byte {
	t.Helper()

	var obj map[string]any
	if err := json.Unmarshal(raw, &obj); err != nil {
		t.Fatalf("invalid json output: %v\n%s", err, string(raw))
	}

	replacements := map[string]string{}
	if artifacts, ok := obj["artifacts"].(map[string]any); ok {
		for key, placeholder := range map[string]string{
			"project_dir":    "REPLACE_PROJECT_DIR",
			"compose_file":   "REPLACE_COMPOSE_FILE",
			"env_file":       "REPLACE_ENV_FILE",
			"backup_root":    "REPLACE_BACKUP_ROOT",
			"manifest_txt":   "REPLACE_MANIFEST_TXT",
			"manifest_json":  "REPLACE_MANIFEST_JSON",
			"files_backup":   "REPLACE_FILES_BACKUP",
			"files_checksum": "REPLACE_FILES_CHECKSUM",
		} {
			value, ok := artifacts[key].(string)
			if !ok || value == "" {
				continue
			}
			replacements[value] = placeholder
			artifacts[key] = placeholder
		}
	}

	if items, ok := obj["items"].([]any); ok {
		for _, rawItem := range items {
			item, ok := rawItem.(map[string]any)
			if !ok {
				continue
			}
			if value, ok := item["details"].(string); ok {
				item["details"] = replaceKnownPaths(value, replacements)
			}
		}
	}

	out, err := json.Marshal(obj)
	if err != nil {
		t.Fatal(err)
	}

	return out
}
