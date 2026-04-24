package cli

import (
	"encoding/json"
	"path/filepath"
	"testing"
)

func TestBackupVerifyCLIJSONSuccess(t *testing.T) {
	tmp := t.TempDir()
	journalDir := filepath.Join(tmp, "journal")
	backupRoot := filepath.Join(tmp, "backups")
	set := writeBackupSet(t, backupRoot, "espocrm-prod", "2026-04-24_12-00-00", "prod", map[string]string{
		"storage/a.txt": "hello",
	})

	outcome := executeCLI(
		"--journal-dir", journalDir,
		"--json",
		"backup",
		"verify",
		"--manifest", set.ManifestJSON,
	)

	if outcome.ExitCode != 0 {
		t.Fatalf("expected exit code 0, got %d stdout=%s stderr=%s", outcome.ExitCode, outcome.Stdout, outcome.Stderr)
	}

	var obj map[string]any
	if err := json.Unmarshal([]byte(outcome.Stdout), &obj); err != nil {
		t.Fatal(err)
	}
	if command := requireJSONString(t, obj, "command"); command != "backup verify" {
		t.Fatalf("unexpected command: %s", command)
	}
	if !requireJSONBool(t, obj, "ok") {
		t.Fatal("expected ok=true")
	}
	if message := requireJSONString(t, obj, "message"); message != "backup verified" {
		t.Fatalf("unexpected message: %s", message)
	}
	if errValue, exists := obj["error"]; !exists || errValue != nil {
		t.Fatalf("expected error=null, got %#v", errValue)
	}
	if manifest := requireJSONString(t, obj, "result", "manifest"); manifest != set.ManifestJSON {
		t.Fatalf("unexpected manifest: %s", manifest)
	}
	if dbBackup := requireJSONString(t, obj, "result", "db_backup"); dbBackup != set.DBBackup {
		t.Fatalf("unexpected db_backup: %s", dbBackup)
	}
	if filesBackup := requireJSONString(t, obj, "result", "files_backup"); filesBackup != set.FilesBackup {
		t.Fatalf("unexpected files_backup: %s", filesBackup)
	}
}

func TestBackupVerifyCLIJSONFailure(t *testing.T) {
	tmp := t.TempDir()
	journalDir := filepath.Join(tmp, "journal")

	outcome := executeCLI(
		"--journal-dir", journalDir,
		"--json",
		"backup",
		"verify",
	)

	if outcome.ExitCode != 2 {
		t.Fatalf("expected exit code 2, got %d stdout=%s stderr=%s", outcome.ExitCode, outcome.Stdout, outcome.Stderr)
	}

	var obj map[string]any
	if err := json.Unmarshal([]byte(outcome.Stdout), &obj); err != nil {
		t.Fatal(err)
	}
	if command := requireJSONString(t, obj, "command"); command != "espops" {
		t.Fatalf("unexpected command: %s", command)
	}
	if requireJSONBool(t, obj, "ok") {
		t.Fatal("expected ok=false")
	}
	if _, exists := obj["message"]; exists {
		t.Fatalf("unexpected top-level message: %#v", obj["message"])
	}
	if kind := requireJSONString(t, obj, "error", "kind"); kind != "usage" {
		t.Fatalf("unexpected error kind: %s", kind)
	}
	if code := requireJSONString(t, obj, "error", "code"); code != "usage_error" {
		t.Fatalf("unexpected error code: %s", code)
	}
	if errMessage := requireJSONString(t, obj, "error", "message"); errMessage != "--manifest is required" {
		t.Fatalf("unexpected error message: %s", errMessage)
	}
}
