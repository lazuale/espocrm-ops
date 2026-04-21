package cli

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/lazuale/espocrm-ops/internal/contract/exitcode"
)

func TestSchema_Restore_JSON_Success_FullManifest(t *testing.T) {
	lockOpt := isolateRecoveryLocks(t)

	fixture := prepareRestoreCommandFixture(t, "prod", map[string]string{
		"espo/data/nested/file.txt":      "hello",
		"espo/custom/modules/module.txt": "custom",
		"espo/client/custom/app.js":      "client",
		"espo/upload/blob.txt":           "upload",
	})
	fixture.docker.SetRunningServices(t, "db", "espocrm", "espocrm-daemon", "espocrm-websocket")
	fixture.docker.SetServiceHealth(t, "db", "healthy")
	fixture.docker.EnableLog(t)

	outcome := executeCLIWithOptions(
		[]testAppOption{
			lockOpt,
			withFixedTestRuntime(fixture.fixedNow, "op-restore-1"),
		},
		"--journal-dir", fixture.journalDir,
		"--json",
		"restore",
		"--scope", "prod",
		"--project-dir", fixture.projectDir,
		"--manifest", fixture.backupSet.ManifestJSON,
		"--force",
		"--confirm-prod", "prod",
	)
	if outcome.ExitCode != 0 {
		t.Fatalf("command failed\nstdout=%s\nstderr=%s", outcome.Stdout, outcome.Stderr)
	}

	obj := decodeCLIJSON(t, outcome.Stdout)
	if command := requireJSONString(t, obj, "command"); command != "restore" {
		t.Fatalf("unexpected command: %v", command)
	}
	if !requireJSONBool(t, obj, "ok") {
		t.Fatalf("expected ok=true")
	}
	if selectionMode := requireJSONString(t, obj, "details", "selection_mode"); selectionMode != "manifest_full" {
		t.Fatalf("expected manifest_full selection, got %v", selectionMode)
	}
	if sourceKind := requireJSONString(t, obj, "details", "source_kind"); sourceKind != "manifest" {
		t.Fatalf("expected manifest source kind, got %v", sourceKind)
	}
	if !requireJSONBool(t, obj, "details", "snapshot_enabled") {
		t.Fatalf("expected snapshot_enabled=true")
	}
	if completed := requireJSONInt(t, obj, "details", "completed"); completed != 7 {
		t.Fatalf("unexpected completed count: %v", completed)
	}

	requireArtifactPathsExist(t, obj, "manifest_json", "db_backup", "files_backup", "snapshot_manifest_json", "snapshot_db_backup", "snapshot_files_backup")

	if got, err := os.ReadFile(filepath.Join(fixture.storageDir, "data", "nested", "file.txt")); err != nil {
		t.Fatal(err)
	} else if string(got) != "hello" {
		t.Fatalf("unexpected restored file content: %q", string(got))
	}
	if _, err := os.Stat(filepath.Join(fixture.storageDir, "before.txt")); !os.IsNotExist(err) {
		t.Fatalf("expected old storage tree to be replaced, stat err=%v", err)
	}
	if mode := mustFileMode(t, filepath.Join(fixture.storageDir, "data")); mode != "775" {
		t.Fatalf("unexpected data dir mode: %s", mode)
	}
	if mode := mustFileMode(t, filepath.Join(fixture.storageDir, "custom")); mode != "775" {
		t.Fatalf("unexpected custom dir mode: %s", mode)
	}
	if mode := mustFileMode(t, filepath.Join(fixture.storageDir, "client", "custom", "app.js")); mode != "664" {
		t.Fatalf("unexpected client/custom file mode: %s", mode)
	}

	log := fixture.docker.ReadLog(t)
	if !containsAll(log,
		"compose --project-directory "+fixture.projectDir+" -f "+filepath.Join(fixture.projectDir, "compose.yaml")+" --env-file "+filepath.Join(fixture.projectDir, ".env.prod")+" stop espocrm espocrm-daemon espocrm-websocket",
		"exec -i -e MYSQL_PWD mock-db mariadb-dump -u espocrm espocrm --single-transaction --quick --routines --triggers --events",
		"exec -i -e MYSQL_PWD mock-db mariadb -u root",
		"image inspect espocrm/espocrm:9.3.4-apache",
		"-v "+fixture.storageDir+":/espo-storage",
		"compose --project-directory "+fixture.projectDir+" -f "+filepath.Join(fixture.projectDir, "compose.yaml")+" --env-file "+filepath.Join(fixture.projectDir, ".env.prod")+" up -d espocrm espocrm-daemon espocrm-websocket",
	) {
		t.Fatalf("unexpected docker log:\n%s", log)
	}
}

func TestSchema_Restore_JSON_Failure_InconsistentManifest(t *testing.T) {
	lockOpt := isolateRecoveryLocks(t)

	fixture := prepareRestoreCommandFixture(t, "prod", map[string]string{
		"espo/data/nested/file.txt": "hello",
	})

	otherSet := writeBackupSet(t, fixture.backupRoot, "espocrm-prod", "2026-04-19_07-00-00", "prod", map[string]string{
		"espo/data/nested/file.txt": "other",
	})
	writeJSON(t, fixture.backupSet.ManifestJSON, map[string]any{
		"version":    1,
		"scope":      "prod",
		"created_at": "2026-04-19T08:00:00Z",
		"artifacts": map[string]any{
			"db_backup":    filepath.Base(fixture.backupSet.DBBackup),
			"files_backup": filepath.Base(otherSet.FilesBackup),
		},
		"checksums": map[string]any{
			"db_backup":    sha256OfFile(t, fixture.backupSet.DBBackup),
			"files_backup": sha256OfFile(t, otherSet.FilesBackup),
		},
	})

	outcome := executeCLIWithOptions(
		[]testAppOption{lockOpt},
		"--journal-dir", fixture.journalDir,
		"--json",
		"restore",
		"--scope", "prod",
		"--project-dir", fixture.projectDir,
		"--manifest", fixture.backupSet.ManifestJSON,
		"--force",
		"--confirm-prod", "prod",
	)

	assertCLIErrorOutput(t, outcome, exitcode.ValidationError, "restore_failed", "manifest backup set is inconsistent")
}

func TestSchema_Restore_JSON_Failure_PostRestoreHealthValidation(t *testing.T) {
	lockOpt := isolateRecoveryLocks(t)

	fixture := prepareRestoreCommandFixture(t, "prod", map[string]string{
		"espo/data/nested/file.txt": "hello",
	})
	fixture.docker.SetRunningServices(t, "db", "espocrm", "espocrm-daemon", "espocrm-websocket")
	fixture.docker.SetServiceHealth(t, "db", "healthy")
	fixture.docker.SetServiceHealth(t, "espocrm", "unhealthy")
	fixture.docker.EnableLog(t)
	fixture.docker.SetHealthFailureMessage(t, "app health failed")

	outcome := executeCLIWithOptions(
		[]testAppOption{
			lockOpt,
			withFixedTestRuntime(fixture.fixedNow, "op-restore-health-fail"),
		},
		"--journal-dir", fixture.journalDir,
		"--json",
		"restore",
		"--scope", "prod",
		"--project-dir", fixture.projectDir,
		"--manifest", fixture.backupSet.ManifestJSON,
		"--force",
		"--confirm-prod", "prod",
	)

	assertCLIErrorOutput(t, outcome, exitcode.RestoreError, "restore_failed", "app health failed")
}

func TestSchema_Restore_JSON_RepeatedManifestRestore_DeterministicState(t *testing.T) {
	lockOpt := isolateRecoveryLocks(t)

	fixture := prepareRestoreCommandFixture(t, "dev", map[string]string{
		"espo/data/nested/file.txt": "hello",
		"espo/upload/blob.txt":      "blob",
	})
	fixture.docker.SetRunningServices(t, "db", "espocrm", "espocrm-daemon", "espocrm-websocket")
	fixture.docker.SetServiceHealth(t, "db", "healthy")

	first := executeCLIWithOptions(
		[]testAppOption{
			lockOpt,
			withFixedTestRuntime(fixture.fixedNow, "op-restore-repeat-1"),
		},
		"--journal-dir", fixture.journalDir,
		"--json",
		"restore",
		"--scope", "dev",
		"--project-dir", fixture.projectDir,
		"--manifest", fixture.backupSet.ManifestJSON,
		"--force",
	)
	if first.ExitCode != 0 {
		t.Fatalf("first restore failed\nstdout=%s\nstderr=%s", first.Stdout, first.Stderr)
	}

	mustWriteFile(t, filepath.Join(fixture.storageDir, "data", "nested", "file.txt"), "drift\n")
	mustWriteFile(t, filepath.Join(fixture.storageDir, "stale.txt"), "stale\n")

	second := executeCLIWithOptions(
		[]testAppOption{
			lockOpt,
			withFixedTestRuntime(fixture.fixedNow.Add(time.Minute), "op-restore-repeat-2"),
		},
		"--journal-dir", fixture.journalDir,
		"--json",
		"restore",
		"--scope", "dev",
		"--project-dir", fixture.projectDir,
		"--manifest", fixture.backupSet.ManifestJSON,
		"--force",
	)
	if second.ExitCode != 0 {
		t.Fatalf("second restore failed\nstdout=%s\nstderr=%s", second.Stdout, second.Stderr)
	}

	firstObj := decodeCLIJSON(t, first.Stdout)
	secondObj := decodeCLIJSON(t, second.Stdout)
	requireSameJSONValue(t, firstObj, secondObj, "details", "selection_mode")
	requireSameJSONValue(t, firstObj, secondObj, "details", "completed")

	if got, err := os.ReadFile(filepath.Join(fixture.storageDir, "data", "nested", "file.txt")); err != nil {
		t.Fatal(err)
	} else if string(got) != "hello" {
		t.Fatalf("unexpected restored file content after repeat: %q", string(got))
	}
	if _, err := os.Stat(filepath.Join(fixture.storageDir, "stale.txt")); !os.IsNotExist(err) {
		t.Fatalf("expected stale file removed after repeated restore, stat err=%v", err)
	}
}

func TestSchema_Restore_JSON_Success_FilesOnly_Direct(t *testing.T) {
	lockOpt := isolateRecoveryLocks(t)

	fixture := prepareRestoreCommandFixture(t, "dev", map[string]string{
		"espo/data/restored.txt": "files-only",
	})
	fixture.docker.EnableLog(t)

	outcome := executeCLIWithOptions(
		[]testAppOption{
			lockOpt,
			withFixedTestRuntime(fixture.fixedNow, "op-restore-files-1"),
		},
		"--journal-dir", fixture.journalDir,
		"--json",
		"restore",
		"--scope", "dev",
		"--project-dir", fixture.projectDir,
		"--files-backup", fixture.backupSet.FilesBackup,
		"--skip-db",
		"--no-snapshot",
		"--no-start",
		"--force",
	)
	if outcome.ExitCode != 0 {
		t.Fatalf("command failed\nstdout=%s\nstderr=%s", outcome.Stdout, outcome.Stderr)
	}

	obj := decodeCLIJSON(t, outcome.Stdout)
	if selectionMode := requireJSONString(t, obj, "details", "selection_mode"); selectionMode != "direct_files_only" {
		t.Fatalf("expected direct_files_only selection, got %v", selectionMode)
	}
	if skipped := requireJSONInt(t, obj, "details", "skipped"); skipped != 3 {
		t.Fatalf("expected three skipped steps, got %v", skipped)
	}
	if requireJSONBool(t, obj, "details", "started_db_temporarily") {
		t.Fatalf("did not expect started_db_temporarily for files-only restore")
	}

	if got, err := os.ReadFile(filepath.Join(fixture.storageDir, "data", "restored.txt")); err != nil {
		t.Fatal(err)
	} else if string(got) != "files-only" {
		t.Fatalf("unexpected restored file content: %q", string(got))
	}
	if mode := mustFileMode(t, filepath.Join(fixture.storageDir, "data")); mode != "775" {
		t.Fatalf("unexpected data dir mode: %s", mode)
	}
	if mode := mustFileMode(t, filepath.Join(fixture.storageDir, "data", "restored.txt")); mode != "664" {
		t.Fatalf("unexpected restored file mode: %s", mode)
	}

	log := fixture.docker.ReadLog(t)
	if stringsContainsAny(log, "mariadb-dump", "exec -i -e MYSQL_PWD mock-db mariadb -u root") {
		t.Fatalf("did not expect database snapshot or restore commands in docker log:\n%s", log)
	}
	if !containsAll(log, "image inspect espocrm/espocrm:9.3.4-apache", "-v "+fixture.storageDir+":/espo-storage") {
		t.Fatalf("expected permission reconcile docker calls in log:\n%s", log)
	}
}

func TestSchema_Restore_JSON_DryRun(t *testing.T) {
	lockOpt := isolateRecoveryLocks(t)

	fixture := prepareRestoreCommandFixture(t, "dev", map[string]string{
		"espo/data/dry-run.txt": "dry-run",
	})
	fixture.docker.SetRunningServices(t, "db", "espocrm", "espocrm-daemon", "espocrm-websocket")
	fixture.docker.SetServiceHealth(t, "db", "healthy")

	outcome := executeCLIWithOptions(
		[]testAppOption{
			lockOpt,
			withFixedTestRuntime(fixture.fixedNow, "op-restore-dryrun-1"),
		},
		"--journal-dir", fixture.journalDir,
		"--json",
		"restore",
		"--scope", "dev",
		"--project-dir", fixture.projectDir,
		"--manifest", fixture.backupSet.ManifestJSON,
		"--dry-run",
	)
	if outcome.ExitCode != 0 {
		t.Fatalf("command failed\nstdout=%s\nstderr=%s", outcome.Stdout, outcome.Stderr)
	}

	obj := decodeCLIJSON(t, outcome.Stdout)
	assertNoLegacyWorkflowVocabularyInJSON(t, obj)

	if !requireJSONBool(t, obj, "dry_run") {
		t.Fatalf("expected dry_run=true")
	}
	if planned := requireJSONInt(t, obj, "details", "planned"); planned != 5 {
		t.Fatalf("expected five planned steps, got %v", planned)
	}
	details := requireJSONObject(t, obj, "details")
	if _, ok := details["would_run"]; ok {
		t.Fatalf("did not expect legacy would_run detail key in %#v", details)
	}
	if completed := requireJSONInt(t, obj, "details", "completed"); completed != 2 {
		t.Fatalf("expected two completed steps, got %v", completed)
	}
	if items := requireJSONArray(t, obj, "items"); len(items) != 7 {
		t.Fatalf("expected seven restore items, got %#v", obj["items"])
	}
	if _, err := os.Stat(filepath.Join(fixture.storageDir, "before.txt")); err != nil {
		t.Fatalf("expected dry-run to avoid mutating storage: %v", err)
	}
}
