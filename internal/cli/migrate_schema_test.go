package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/lazuale/espocrm-ops/internal/contract/exitcode"
)

func TestSchema_Migrate_JSON_Success(t *testing.T) {
	lockOpt := isolateRecoveryLocks(t)

	fixture := prepareMigrateCommandFixture(t)
	fixture.docker.SetRunningServices(t, "espocrm", "espocrm-daemon", "espocrm-websocket")
	fixture.docker.SetServiceHealth(t, "db", "healthy")
	fixture.docker.EnableLog(t)

	outcome := executeCLIWithOptions(
		[]testAppOption{
			lockOpt,
			withFixedTestRuntime(fixture.fixedNow, "op-migrate-1"),
		},
		"--journal-dir", fixture.journalDir,
		"--json",
		"migrate",
		"--from", "dev",
		"--to", "prod",
		"--project-dir", fixture.projectDir,
		"--force",
		"--confirm-prod", "prod",
	)
	if outcome.ExitCode != 0 {
		t.Fatalf("command failed\nstdout=%s\nstderr=%s", outcome.Stdout, outcome.Stderr)
	}

	obj := decodeCLIJSON(t, outcome.Stdout)
	if command := requireJSONString(t, obj, "command"); command != "migrate" {
		t.Fatalf("unexpected command: %v", command)
	}
	if !requireJSONBool(t, obj, "ok") {
		t.Fatalf("expected ok=true")
	}
	if message := requireJSONString(t, obj, "message"); message != "backup migration completed" {
		t.Fatalf("unexpected message: %v", message)
	}
	if sourceScope := requireJSONString(t, obj, "details", "source_scope"); sourceScope != "dev" {
		t.Fatalf("expected source_scope=dev, got %v", sourceScope)
	}
	if targetScope := requireJSONString(t, obj, "details", "target_scope"); targetScope != "prod" {
		t.Fatalf("expected target_scope=prod, got %v", targetScope)
	}
	if selectionMode := requireJSONString(t, obj, "details", "selection_mode"); selectionMode != "auto_latest_complete" {
		t.Fatalf("expected auto_latest_complete selection, got %v", selectionMode)
	}
	if completed := requireJSONInt(t, obj, "details", "completed"); completed != 8 {
		t.Fatalf("unexpected completed count: %v", completed)
	}
	if !requireJSONBool(t, obj, "details", "started_db_temporarily") {
		t.Fatalf("expected started_db_temporarily=true")
	}

	requireArtifactPathsExist(t, obj, "manifest_json", "db_backup", "files_backup")
	if items := requireJSONArray(t, obj, "items"); len(items) != 8 {
		t.Fatalf("expected eight migrate items, got %#v", obj["items"])
	}

	restoredContent, err := os.ReadFile(filepath.Join(fixture.storageDir, "test.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(restoredContent) != "hello" {
		t.Fatalf("unexpected restored file content: %q", string(restoredContent))
	}
	if _, err := os.Stat(filepath.Join(fixture.storageDir, "before.txt")); !os.IsNotExist(err) {
		t.Fatalf("expected old storage tree to be replaced, stat err=%v", err)
	}

	log := fixture.docker.ReadLog(t)
	if !containsAll(log,
		"compose --project-directory "+fixture.projectDir+" -f "+filepath.Join(fixture.projectDir, "compose.yaml")+" --env-file "+filepath.Join(fixture.projectDir, ".env.prod")+" up -d db",
		"compose --project-directory "+fixture.projectDir+" -f "+filepath.Join(fixture.projectDir, "compose.yaml")+" --env-file "+filepath.Join(fixture.projectDir, ".env.prod")+" stop espocrm espocrm-daemon espocrm-websocket",
		"exec mock-db mariadb --version",
		"exec -i -e MYSQL_PWD mock-db mariadb -u root",
		"compose --project-directory "+fixture.projectDir+" -f "+filepath.Join(fixture.projectDir, "compose.yaml")+" --env-file "+filepath.Join(fixture.projectDir, ".env.prod")+" up -d",
	) {
		t.Fatalf("unexpected docker log:\n%s", log)
	}
}

func TestSchema_Migrate_JSON_Success_SkipDBNoStart(t *testing.T) {
	lockOpt := isolateRecoveryLocks(t)

	fixture := prepareMigrateCommandFixture(t)
	fixture.docker.SetRunningServices(t, "espocrm", "espocrm-daemon", "espocrm-websocket")
	fixture.docker.SetServiceHealth(t, "db", "healthy")
	fixture.docker.EnableLog(t)

	outcome := executeCLIWithOptions(
		[]testAppOption{lockOpt},
		"--journal-dir", fixture.journalDir,
		"--json",
		"migrate",
		"--from", "dev",
		"--to", "prod",
		"--project-dir", fixture.projectDir,
		"--force",
		"--confirm-prod", "prod",
		"--skip-db",
		"--no-start",
	)
	if outcome.ExitCode != 0 {
		t.Fatalf("command failed\nstdout=%s\nstderr=%s", outcome.Stdout, outcome.Stderr)
	}

	obj := decodeCLIJSON(t, outcome.Stdout)
	if selectionMode := requireJSONString(t, obj, "details", "selection_mode"); selectionMode != "auto_latest_files" {
		t.Fatalf("expected auto_latest_files selection, got %v", selectionMode)
	}
	if skipped := requireJSONInt(t, obj, "details", "skipped"); skipped != 2 {
		t.Fatalf("expected two skipped steps, got %v", skipped)
	}

	log := fixture.docker.ReadLog(t)
	if strings.Contains(log, "exec -i -e MYSQL_PWD mock-db mariadb -u root") {
		t.Fatalf("did not expect database restore commands in docker log:\n%s", log)
	}
	if strings.Contains(log, "compose --project-directory "+fixture.projectDir+" -f "+filepath.Join(fixture.projectDir, "compose.yaml")+" --env-file "+filepath.Join(fixture.projectDir, ".env.prod")+" up -d\n") {
		t.Fatalf("did not expect final contour start in docker log:\n%s", log)
	}
}

func TestSchema_Migrate_JSON_Failure_InvalidMatchingManifestBlocked(t *testing.T) {
	lockOpt := isolateRecoveryLocks(t)

	fixture := prepareMigrateCommandFixture(t)
	otherSet := writeBackupSet(t, fixture.sourceRoot, "espocrm-dev", "2026-04-19_07-00-00", "dev", nil)
	writeJSON(t, fixture.sourceBackup.ManifestJSON, map[string]any{
		"version":    1,
		"scope":      "dev",
		"created_at": "2026-04-19T08:00:00Z",
		"artifacts": map[string]any{
			"db_backup":    filepath.Base(fixture.sourceBackup.DBBackup),
			"files_backup": filepath.Base(otherSet.FilesBackup),
		},
		"checksums": map[string]any{
			"db_backup":    sha256OfFile(t, fixture.sourceBackup.DBBackup),
			"files_backup": sha256OfFile(t, otherSet.FilesBackup),
		},
	})

	outcome := executeCLIWithOptions(
		[]testAppOption{lockOpt},
		"--journal-dir", fixture.journalDir,
		"--json",
		"migrate",
		"--from", "dev",
		"--to", "prod",
		"--project-dir", fixture.projectDir,
		"--force",
		"--confirm-prod", "prod",
	)

	assertCLIErrorOutput(t, outcome, exitcode.ValidationError, "migrate_failed", "manifest backup set is inconsistent")
}

func TestSchema_Migrate_JSON_Failure_TargetHealthValidation(t *testing.T) {
	lockOpt := isolateRecoveryLocks(t)

	fixture := prepareMigrateCommandFixture(t)
	fixture.docker.SetRunningServices(t, "espocrm", "espocrm-daemon", "espocrm-websocket")
	fixture.docker.SetServiceHealth(t, "db", "healthy")
	fixture.docker.SetServiceHealth(t, "espocrm", "unhealthy")
	fixture.docker.SetHealthFailureMessage(t, "target health failed")

	outcome := executeCLIWithOptions(
		[]testAppOption{
			lockOpt,
			withFixedTestRuntime(fixture.fixedNow, "op-migrate-health-fail"),
		},
		"--journal-dir", fixture.journalDir,
		"--json",
		"migrate",
		"--from", "dev",
		"--to", "prod",
		"--project-dir", fixture.projectDir,
		"--force",
		"--confirm-prod", "prod",
	)

	assertCLIErrorOutput(t, outcome, exitcode.RestoreError, "migrate_failed", "target health failed")
}

func TestSchema_Migrate_JSON_RepeatedDeterministicState(t *testing.T) {
	lockOpt := isolateRecoveryLocks(t)

	fixture := prepareMigrateCommandFixture(t)
	fixture.docker.SetRunningServices(t, "espocrm", "espocrm-daemon", "espocrm-websocket")
	fixture.docker.SetServiceHealth(t, "db", "healthy")

	first := executeCLIWithOptions(
		[]testAppOption{
			lockOpt,
			withFixedTestRuntime(fixture.fixedNow, "op-migrate-repeat-1"),
		},
		"--journal-dir", fixture.journalDir,
		"--json",
		"migrate",
		"--from", "dev",
		"--to", "prod",
		"--project-dir", fixture.projectDir,
		"--force",
		"--confirm-prod", "prod",
	)
	if first.ExitCode != 0 {
		t.Fatalf("first migrate failed\nstdout=%s\nstderr=%s", first.Stdout, first.Stderr)
	}

	mustWriteFile(t, filepath.Join(fixture.storageDir, "test.txt"), "drift\n")
	mustWriteFile(t, filepath.Join(fixture.storageDir, "stale.txt"), "stale\n")

	second := executeCLIWithOptions(
		[]testAppOption{
			lockOpt,
			withFixedTestRuntime(fixture.fixedNow.Add(time.Minute), "op-migrate-repeat-2"),
		},
		"--journal-dir", fixture.journalDir,
		"--json",
		"migrate",
		"--from", "dev",
		"--to", "prod",
		"--project-dir", fixture.projectDir,
		"--force",
		"--confirm-prod", "prod",
	)
	if second.ExitCode != 0 {
		t.Fatalf("second migrate failed\nstdout=%s\nstderr=%s", second.Stdout, second.Stderr)
	}

	firstObj := decodeCLIJSON(t, first.Stdout)
	secondObj := decodeCLIJSON(t, second.Stdout)
	requireSameJSONValue(t, firstObj, secondObj, "details", "selection_mode")
	requireSameJSONValue(t, firstObj, secondObj, "details", "completed")

	if restoredContent, err := os.ReadFile(filepath.Join(fixture.storageDir, "test.txt")); err != nil {
		t.Fatal(err)
	} else if string(restoredContent) != "hello" {
		t.Fatalf("unexpected restored file content after repeat migrate: %q", string(restoredContent))
	}
	if _, err := os.Stat(filepath.Join(fixture.storageDir, "stale.txt")); !os.IsNotExist(err) {
		t.Fatalf("expected stale file removed after repeated migrate, stat err=%v", err)
	}
}

func TestSchema_Migrate_JSON_Failure_CompatibilityDrift(t *testing.T) {
	lockOpt := isolateRecoveryLocks(t)

	fixture := prepareMigrateCommandFixture(t)
	writeDoctorEnvFile(t, fixture.projectDir, "dev", map[string]string{
		"ESPOCRM_IMAGE": "espocrm/espocrm:9.3.4-apache",
	})
	writeDoctorEnvFile(t, fixture.projectDir, "prod", map[string]string{
		"ESPOCRM_IMAGE": "espocrm/espocrm:9.4.0-apache",
	})

	outcome := executeCLIWithOptions(
		[]testAppOption{lockOpt},
		"--journal-dir", fixture.journalDir,
		"--json",
		"migrate",
		"--from", "dev",
		"--to", "prod",
		"--project-dir", fixture.projectDir,
		"--force",
		"--confirm-prod", "prod",
	)

	assertCLIErrorOutput(t, outcome, exitcode.ValidationError, "migrate_failed", "conflict with the migration compatibility contract")

	obj := decodeCLIJSON(t, outcome.Stdout)
	if message := requireJSONString(t, obj, "message"); message != "backup migration failed" {
		t.Fatalf("unexpected message: %v", message)
	}
}
