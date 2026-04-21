package cli

import (
	"testing"

	"github.com/lazuale/espocrm-ops/internal/contract/exitcode"
	platformfs "github.com/lazuale/espocrm-ops/internal/platform/fs"
)

func TestSchema_BackupExecute_JSON_FilesOnlyNoStop(t *testing.T) {
	fixture := prepareBackupCommandFixture(t)
	fixture.docker.SetFailOnAnyCall(t)

	out, err := runRootCommandWithOptions(
		t,
		[]testAppOption{
			withFixedTestRuntime(fixture.fixedNow, "op-backup-1"),
		},
		"--journal-dir", fixture.journalDir,
		"--json",
		"backup",
		"--scope", "dev",
		"--project-dir", fixture.projectDir,
		"--compose-file", fixture.composeFile,
		"--env-file", fixture.envFile,
		"--skip-db",
		"--no-stop",
	)
	if err != nil {
		t.Fatalf("command failed: %v\noutput=%s", err, out)
	}

	obj := decodeCLIJSON(t, out)

	if command := requireJSONString(t, obj, "command"); command != "backup" {
		t.Fatalf("unexpected command: %v", command)
	}
	if !requireJSONBool(t, obj, "ok") {
		t.Fatalf("expected ok=true")
	}
	if message := requireJSONString(t, obj, "message"); message != "backup completed" {
		t.Fatalf("unexpected message: %v", message)
	}
	if createdAt := requireJSONString(t, obj, "details", "created_at"); createdAt != "2026-04-15T11:00:00Z" {
		t.Fatalf("unexpected created_at: %v", createdAt)
	}
	if requireJSONBool(t, obj, "details", "consistent_snapshot") {
		t.Fatalf("expected consistent_snapshot=false")
	}

	filesBackup := requireJSONString(t, obj, "artifacts", "files_backup")
	if err := platformfs.VerifyTarGzReadable(filesBackup, nil); err != nil {
		t.Fatalf("expected readable files backup: %v", err)
	}
	requireArtifactPathsExist(t, obj, "manifest_txt", "manifest_json", "files_backup", "files_checksum")
}

func TestSchema_BackupExecute_JSON_Error_MissingBackupRoot_NoJournal(t *testing.T) {
	fixture := prepareBackupCommandFixture(t)
	envFile := writeDoctorEnvFile(t, fixture.projectDir, "dev", map[string]string{
		"BACKUP_ROOT": "",
	})

	outcome := executeCLI(
		"--journal-dir", fixture.journalDir,
		"--json",
		"backup",
		"--scope", "dev",
		"--project-dir", fixture.projectDir,
		"--compose-file", fixture.composeFile,
		"--env-file", envFile,
		"--skip-db",
		"--no-stop",
	)

	assertCLIErrorOutput(t, outcome, exitcode.ValidationError, "backup_failed", "BACKUP_ROOT is not set")
}
