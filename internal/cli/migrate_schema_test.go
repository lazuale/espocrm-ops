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
	fixture.docker.SetRunningServices(t, "db", "espocrm", "espocrm-daemon", "espocrm-websocket")
	fixture.docker.SetServiceHealth(t, "db", "healthy")
	fixture.docker.SetServiceHealth(t, "espocrm", "healthy")
	fixture.docker.SetServiceHealth(t, "espocrm-daemon", "healthy")
	fixture.docker.SetServiceHealth(t, "espocrm-websocket", "healthy")
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
	if message := requireJSONString(t, obj, "message"); message != "migrate завершён" {
		t.Fatalf("unexpected message: %v", message)
	}
	if selectionMode := requireJSONString(t, obj, "details", "selection_mode"); selectionMode != "auto_latest_complete" {
		t.Fatalf("expected auto_latest_complete selection, got %v", selectionMode)
	}
	if sourceKind := requireJSONString(t, obj, "details", "source_kind"); sourceKind != "backup_root" {
		t.Fatalf("expected backup_root source kind, got %v", sourceKind)
	}
	if !requireJSONBool(t, obj, "details", "snapshot_enabled") {
		t.Fatalf("expected snapshot_enabled=true")
	}
	if !requireJSONBool(t, obj, "details", "app_services_were_running") {
		t.Fatalf("expected app_services_were_running=true")
	}
	if requireJSONBool(t, obj, "details", "started_db_temporarily") {
		t.Fatalf("expected started_db_temporarily=false")
	}
	if completed := requireJSONInt(t, obj, "details", "completed"); completed != 9 {
		t.Fatalf("unexpected completed count: %v", completed)
	}

	requireArtifactPathsExist(t, obj,
		"manifest_json",
		"db_backup",
		"files_backup",
		"snapshot_manifest_json",
		"snapshot_db_backup",
		"snapshot_files_backup",
	)
	if items := requireJSONArray(t, obj, "items"); len(items) != 9 {
		t.Fatalf("expected nine migrate items, got %#v", obj["items"])
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
		"compose --project-directory "+fixture.projectDir+" -f "+filepath.Join(fixture.projectDir, "compose.yaml")+" --env-file "+filepath.Join(fixture.projectDir, ".env.prod")+" stop espocrm espocrm-daemon espocrm-websocket",
		"exec -i -e MYSQL_PWD mock-db mariadb-dump -u espocrm espocrm --single-transaction --quick --routines --triggers --events",
		"exec -i -e MYSQL_PWD mock-db mariadb -u root",
		"compose --project-directory "+fixture.projectDir+" -f "+filepath.Join(fixture.projectDir, "compose.yaml")+" --env-file "+filepath.Join(fixture.projectDir, ".env.prod")+" up -d espocrm espocrm-daemon espocrm-websocket",
	) {
		t.Fatalf("unexpected docker log:\n%s", log)
	}
}

func TestSchema_Migrate_JSON_Success_ExplicitFilesOnly_NoStart(t *testing.T) {
	lockOpt := isolateRecoveryLocks(t)

	fixture := prepareMigrateCommandFixture(t)
	fixture.docker.SetRunningServices(t, "db", "espocrm", "espocrm-daemon", "espocrm-websocket")
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
		"--files-backup", fixture.sourceBackup.FilesBackup,
		"--skip-db",
		"--no-start",
		"--force",
		"--confirm-prod", "prod",
	)
	if outcome.ExitCode != 0 {
		t.Fatalf("command failed\nstdout=%s\nstderr=%s", outcome.Stdout, outcome.Stderr)
	}

	obj := decodeCLIJSON(t, outcome.Stdout)
	if selectionMode := requireJSONString(t, obj, "details", "selection_mode"); selectionMode != "explicit_files_only" {
		t.Fatalf("expected explicit_files_only selection, got %v", selectionMode)
	}
	if sourceKind := requireJSONString(t, obj, "details", "source_kind"); sourceKind != "direct" {
		t.Fatalf("expected direct source kind, got %v", sourceKind)
	}
	if skipped := requireJSONInt(t, obj, "details", "skipped"); skipped != 2 {
		t.Fatalf("expected two skipped steps, got %v", skipped)
	}

	requireArtifactPathsExist(t, obj,
		"files_backup",
		"snapshot_files_backup",
	)

	log := fixture.docker.ReadLog(t)
	if strings.Contains(log, "exec -i -e MYSQL_PWD mock-db mariadb -u root") {
		t.Fatalf("did not expect database restore commands in docker log:\n%s", log)
	}
	if running := readRunningServicesSnapshot(t, fixture.docker.stateDir); len(running) != 1 || running[0] != "db" {
		t.Fatalf("expected final runtime to keep only db running, got %v", running)
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

	assertCLIErrorOutput(t, outcome, exitcode.ValidationError, "migrate_failed", "matching manifest backup-set неконсистентен")
}

func TestSchema_Migrate_JSON_Failure_TargetHealthValidation(t *testing.T) {
	lockOpt := isolateRecoveryLocks(t)

	fixture := prepareMigrateCommandFixture(t)
	fixture.docker.SetRunningServices(t, "db", "espocrm", "espocrm-daemon", "espocrm-websocket")
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

	obj := decodeCLIJSON(t, outcome.Stdout)
	if message := requireJSONString(t, obj, "message"); message != "migrate завершился ошибкой" {
		t.Fatalf("unexpected message: %v", message)
	}
}

func TestSchema_Migrate_JSON_RepeatedDeterministicState(t *testing.T) {
	lockOpt := isolateRecoveryLocks(t)

	fixture := prepareMigrateCommandFixture(t)
	fixture.docker.SetRunningServices(t, "db", "espocrm", "espocrm-daemon", "espocrm-websocket")
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

	assertCLIErrorOutput(t, outcome, exitcode.ValidationError, "migrate_failed", "migration compatibility contract")

	obj := decodeCLIJSON(t, outcome.Stdout)
	if message := requireJSONString(t, obj, "message"); message != "migrate завершился ошибкой" {
		t.Fatalf("unexpected message: %v", message)
	}
}

func TestSchema_Migrate_JSON_Success_ReturnsToStoppedRuntimeWhenTargetWasStopped(t *testing.T) {
	lockOpt := isolateRecoveryLocks(t)

	fixture := prepareMigrateCommandFixture(t)
	fixture.docker.EnableLog(t)

	outcome := executeCLIWithOptions(
		[]testAppOption{
			lockOpt,
			withFixedTestRuntime(fixture.fixedNow, "op-migrate-stopped-target"),
		},
		"--journal-dir", fixture.journalDir,
		"--json",
		"migrate",
		"--from", "dev",
		"--to", "prod",
		"--project-dir", fixture.projectDir,
		"--files-backup", fixture.sourceBackup.FilesBackup,
		"--skip-db",
		"--force",
		"--confirm-prod", "prod",
	)
	if outcome.ExitCode != 0 {
		t.Fatalf("command failed\nstdout=%s\nstderr=%s", outcome.Stdout, outcome.Stderr)
	}

	obj := decodeCLIJSON(t, outcome.Stdout)
	if requireJSONBool(t, obj, "details", "app_services_were_running") {
		t.Fatalf("expected app_services_were_running=false")
	}
	if !requireJSONBool(t, obj, "details", "started_db_temporarily") {
		t.Fatalf("expected started_db_temporarily=true")
	}

	if running := readRunningServicesSnapshot(t, fixture.docker.stateDir); len(running) != 0 {
		t.Fatalf("expected target runtime to return to stopped state, got %v", running)
	}

	log := fixture.docker.ReadLog(t)
	if strings.Contains(log, "up -d espocrm espocrm-daemon espocrm-websocket") {
		t.Fatalf("did not expect application services start for initially stopped target:\n%s", log)
	}
}
