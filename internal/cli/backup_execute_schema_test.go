package cli

import (
	"path/filepath"
	"strings"
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
	requireArtifactPathsExist(t, obj, "files_backup", "files_checksum")
	if manifest := requireJSONString(t, obj, "artifacts", "manifest_txt"); manifest != "" {
		t.Fatalf("partial backup must not expose text manifest, got %s", manifest)
	}
	if manifest := requireJSONString(t, obj, "artifacts", "manifest_json"); manifest != "" {
		t.Fatalf("partial backup must not expose json manifest, got %s", manifest)
	}
}

func TestSchema_BackupExecute_JSON_HelperFallback_FilesOnlyNoStop(t *testing.T) {
	fixture := prepareBackupCommandFixture(t)
	fixture.docker.EnableLog(t)
	prependFailingTar(t, "local tar failed")

	out, err := runRootCommandWithOptions(
		t,
		[]testAppOption{
			withFixedTestRuntime(fixture.fixedNow, "op-backup-helper"),
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
	if !requireJSONBool(t, obj, "ok") {
		t.Fatalf("expected ok=true")
	}
	if warnings := requireJSONInt(t, obj, "details", "warnings"); warnings != 1 {
		t.Fatalf("expected one helper warning, got %d", warnings)
	}
	rawWarnings := requireJSONArray(t, obj, "warnings")
	if len(rawWarnings) != 1 {
		t.Fatalf("expected one warning, got %v", rawWarnings)
	}
	warning, _ := rawWarnings[0].(string)
	if !strings.Contains(warning, "helper fallback") {
		t.Fatalf("unexpected warning: %v", rawWarnings)
	}

	filesBackup := requireJSONString(t, obj, "artifacts", "files_backup")
	if err := platformfs.VerifyTarGzReadable(filesBackup, nil); err != nil {
		t.Fatalf("expected readable files backup: %v", err)
	}
	if manifest := requireJSONString(t, obj, "artifacts", "manifest_txt"); manifest != "" {
		t.Fatalf("partial backup must not expose text manifest, got %s", manifest)
	}
	if manifest := requireJSONString(t, obj, "artifacts", "manifest_json"); manifest != "" {
		t.Fatalf("partial backup must not expose json manifest, got %s", manifest)
	}

	log := fixture.docker.ReadLog(t)
	if !containsAll(log, "image inspect alpine:3.20", "run --pull=never --rm --entrypoint tar") {
		t.Fatalf("expected explicit helper archive path, got log:\n%s", log)
	}
}

func TestSchema_BackupExecute_JSON_Error_HelperContractMissing_FailClosed(t *testing.T) {
	fixture := prepareBackupCommandFixture(t)
	fixture.docker.SetFailOnAnyCall(t)
	envFile := writeDoctorEnvFile(t, fixture.projectDir, "dev", map[string]string{
		"ESPO_HELPER_IMAGE": "",
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

	assertCLIErrorOutput(t, outcome, exitcode.ValidationError, "backup_failed", "ESPO_HELPER_IMAGE is not set")
}

func TestSchema_BackupExecute_JSON_Error_HelperFallbackExecutionFailure_NoSuccess(t *testing.T) {
	fixture := prepareBackupCommandFixture(t)
	fixture.docker.SetFailOnMatch(t, "run --pull=never --rm --entrypoint tar")
	prependFailingTar(t, "local tar failed")

	outcome := executeCLI(
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

	assertCLIErrorOutput(t, outcome, exitcode.RestoreError, "backup_failed", "helper fallback")
	matches, err := filepath.Glob(filepath.Join(fixture.projectDir, "backups", "dev", "files", "*.tar.gz"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 0 {
		t.Fatalf("helper execution failure must not leave files artifacts: %v", matches)
	}
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
