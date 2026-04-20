package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/lazuale/espocrm-ops/internal/contract/exitcode"
)

func TestSchema_Restore_JSON_Success_FullManifest(t *testing.T) {
	isolateRollbackPlanLocks(t)

	fixture := prepareRestoreCommandFixture(t, "prod", map[string]string{
		"espo/data/nested/file.txt":      "hello",
		"espo/custom/modules/module.txt": "custom",
		"espo/client/custom/app.js":      "client",
		"espo/upload/blob.txt":           "upload",
	})
	useJournalClockForTest(t, fixture.fixedNow)

	if err := os.WriteFile(filepath.Join(fixture.stateDir, "running-services"), []byte("db\nespocrm\nespocrm-daemon\nespocrm-websocket\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeUpdateRuntimeStatusFile(t, fixture.stateDir, "db", "healthy")

	prependFakeDockerForRollbackCLITest(t)
	t.Setenv("DOCKER_MOCK_ROLLBACK_STATE_DIR", fixture.stateDir)
	t.Setenv("DOCKER_MOCK_ROLLBACK_LOG", fixture.logPath)
	t.Setenv("DOCKER_MOCK_RESTORE_RUNTIME_UID", strconv.Itoa(os.Getuid()))
	t.Setenv("DOCKER_MOCK_RESTORE_RUNTIME_GID", strconv.Itoa(os.Getgid()))

	outcome := executeCLIWithOptions(
		[]testAppOption{withFixedTestRuntime(fixture.fixedNow, "op-restore-1")},
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

	var obj map[string]any
	if err := json.Unmarshal([]byte(outcome.Stdout), &obj); err != nil {
		t.Fatal(err)
	}

	requireJSONPath(t, obj, "command")
	requireJSONPath(t, obj, "ok")
	requireJSONPath(t, obj, "message")
	requireJSONPath(t, obj, "details", "scope")
	requireJSONPath(t, obj, "details", "selection_mode")
	requireJSONPath(t, obj, "details", "source_kind")
	requireJSONPath(t, obj, "details", "snapshot_enabled")
	requireJSONPath(t, obj, "details", "completed")
	requireJSONPath(t, obj, "artifacts", "manifest_json")
	requireJSONPath(t, obj, "artifacts", "snapshot_manifest_json")
	requireJSONPath(t, obj, "artifacts", "snapshot_db_backup")
	requireJSONPath(t, obj, "artifacts", "snapshot_files_backup")
	requireJSONPath(t, obj, "items")

	if obj["command"] != "restore" {
		t.Fatalf("unexpected command: %v", obj["command"])
	}
	if ok, _ := obj["ok"].(bool); !ok {
		t.Fatalf("expected ok=true, got %v", obj["ok"])
	}
	if selectionMode := requireJSONPath(t, obj, "details", "selection_mode"); selectionMode != "manifest_full" {
		t.Fatalf("expected manifest_full selection, got %v", selectionMode)
	}
	if sourceKind := requireJSONPath(t, obj, "details", "source_kind"); sourceKind != "manifest" {
		t.Fatalf("expected manifest source kind, got %v", sourceKind)
	}
	if snapshotEnabled, _ := requireJSONPath(t, obj, "details", "snapshot_enabled").(bool); !snapshotEnabled {
		t.Fatalf("expected snapshot_enabled=true")
	}
	if completed, _ := requireJSONPath(t, obj, "details", "completed").(float64); int(completed) != 7 {
		t.Fatalf("unexpected completed count: %v", completed)
	}

	for _, key := range []string{"manifest_json", "db_backup", "files_backup", "snapshot_manifest_json", "snapshot_db_backup", "snapshot_files_backup"} {
		path, _ := requireJSONPath(t, obj, "artifacts", key).(string)
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected artifact %s at %s: %v", key, path, err)
		}
	}

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

	rawLog, err := os.ReadFile(fixture.logPath)
	if err != nil {
		t.Fatal(err)
	}
	log := string(rawLog)
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
	isolateRollbackPlanLocks(t)

	fixture := prepareRestoreCommandFixture(t, "prod", map[string]string{
		"espo/data/nested/file.txt": "hello",
	})

	otherSet := writeRestoreBackupSet(t, fixture.backupRoot, "espocrm-prod", "2026-04-19_07-00-00", "prod", map[string]string{
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

	outcome := executeCLI(
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
	isolateRollbackPlanLocks(t)

	fixture := prepareRestoreCommandFixture(t, "prod", map[string]string{
		"espo/data/nested/file.txt": "hello",
	})
	useJournalClockForTest(t, fixture.fixedNow)

	if err := os.WriteFile(filepath.Join(fixture.stateDir, "running-services"), []byte("db\nespocrm\nespocrm-daemon\nespocrm-websocket\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeUpdateRuntimeStatusFile(t, fixture.stateDir, "db", "healthy")
	writeUpdateRuntimeStatusFile(t, fixture.stateDir, "espocrm", "unhealthy")

	prependFakeDockerForRollbackCLITest(t)
	t.Setenv("DOCKER_MOCK_ROLLBACK_STATE_DIR", fixture.stateDir)
	t.Setenv("DOCKER_MOCK_ROLLBACK_LOG", fixture.logPath)
	t.Setenv("DOCKER_MOCK_ROLLBACK_HEALTH_MESSAGE", "app health failed")
	t.Setenv("DOCKER_MOCK_RESTORE_RUNTIME_UID", strconv.Itoa(os.Getuid()))
	t.Setenv("DOCKER_MOCK_RESTORE_RUNTIME_GID", strconv.Itoa(os.Getgid()))

	outcome := executeCLIWithOptions(
		[]testAppOption{withFixedTestRuntime(fixture.fixedNow, "op-restore-health-fail")},
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
	isolateRollbackPlanLocks(t)

	fixture := prepareRestoreCommandFixture(t, "dev", map[string]string{
		"espo/data/nested/file.txt": "hello",
		"espo/upload/blob.txt":      "blob",
	})
	useJournalClockForTest(t, fixture.fixedNow)

	if err := os.WriteFile(filepath.Join(fixture.stateDir, "running-services"), []byte("db\nespocrm\nespocrm-daemon\nespocrm-websocket\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeUpdateRuntimeStatusFile(t, fixture.stateDir, "db", "healthy")

	prependFakeDockerForRollbackCLITest(t)
	t.Setenv("DOCKER_MOCK_ROLLBACK_STATE_DIR", fixture.stateDir)
	t.Setenv("DOCKER_MOCK_RESTORE_RUNTIME_UID", strconv.Itoa(os.Getuid()))
	t.Setenv("DOCKER_MOCK_RESTORE_RUNTIME_GID", strconv.Itoa(os.Getgid()))

	first := executeCLIWithOptions(
		[]testAppOption{withFixedTestRuntime(fixture.fixedNow, "op-restore-repeat-1")},
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

	if err := os.WriteFile(filepath.Join(fixture.storageDir, "data", "nested", "file.txt"), []byte("drift\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(fixture.storageDir, "stale.txt"), []byte("stale\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	second := executeCLIWithOptions(
		[]testAppOption{withFixedTestRuntime(fixture.fixedNow.Add(time.Minute), "op-restore-repeat-2")},
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

	var firstObj map[string]any
	if err := json.Unmarshal([]byte(first.Stdout), &firstObj); err != nil {
		t.Fatal(err)
	}
	var secondObj map[string]any
	if err := json.Unmarshal([]byte(second.Stdout), &secondObj); err != nil {
		t.Fatal(err)
	}

	if got := requireJSONPath(t, firstObj, "details", "selection_mode"); got != requireJSONPath(t, secondObj, "details", "selection_mode") {
		t.Fatalf("selection_mode drifted across restores: first=%v second=%v", requireJSONPath(t, firstObj, "details", "selection_mode"), got)
	}
	if got := requireJSONPath(t, firstObj, "details", "completed"); got != requireJSONPath(t, secondObj, "details", "completed") {
		t.Fatalf("completed count drifted across restores: first=%v second=%v", requireJSONPath(t, firstObj, "details", "completed"), got)
	}

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
	isolateRollbackPlanLocks(t)

	fixture := prepareRestoreCommandFixture(t, "dev", map[string]string{
		"espo/data/restored.txt": "files-only",
	})
	useJournalClockForTest(t, fixture.fixedNow)

	prependFakeDockerForRollbackCLITest(t)
	t.Setenv("DOCKER_MOCK_ROLLBACK_STATE_DIR", fixture.stateDir)
	t.Setenv("DOCKER_MOCK_ROLLBACK_LOG", fixture.logPath)
	t.Setenv("DOCKER_MOCK_RESTORE_RUNTIME_UID", strconv.Itoa(os.Getuid()))
	t.Setenv("DOCKER_MOCK_RESTORE_RUNTIME_GID", strconv.Itoa(os.Getgid()))

	outcome := executeCLIWithOptions(
		[]testAppOption{withFixedTestRuntime(fixture.fixedNow, "op-restore-files-1")},
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

	var obj map[string]any
	if err := json.Unmarshal([]byte(outcome.Stdout), &obj); err != nil {
		t.Fatal(err)
	}

	if selectionMode := requireJSONPath(t, obj, "details", "selection_mode"); selectionMode != "direct_files_only" {
		t.Fatalf("expected direct_files_only selection, got %v", selectionMode)
	}
	if skipped, _ := requireJSONPath(t, obj, "details", "skipped").(float64); int(skipped) != 3 {
		t.Fatalf("expected three skipped steps, got %v", skipped)
	}
	if startedTemporarily, _ := requireJSONPath(t, obj, "details", "started_db_temporarily").(bool); startedTemporarily {
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

	rawLog, err := os.ReadFile(fixture.logPath)
	if err != nil {
		t.Fatal(err)
	}
	log := string(rawLog)
	if stringsContainsAny(log, "mariadb-dump", "exec -i -e MYSQL_PWD mock-db mariadb -u root") {
		t.Fatalf("did not expect database snapshot or restore commands in docker log:\n%s", log)
	}
	if !containsAll(log, "image inspect espocrm/espocrm:9.3.4-apache", "-v "+fixture.storageDir+":/espo-storage") {
		t.Fatalf("expected permission reconcile docker calls in log:\n%s", log)
	}
}

func TestSchema_Restore_JSON_DryRun(t *testing.T) {
	isolateRollbackPlanLocks(t)

	fixture := prepareRestoreCommandFixture(t, "dev", map[string]string{
		"espo/data/dry-run.txt": "dry-run",
	})
	useJournalClockForTest(t, fixture.fixedNow)

	if err := os.WriteFile(filepath.Join(fixture.stateDir, "running-services"), []byte("db\nespocrm\nespocrm-daemon\nespocrm-websocket\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeUpdateRuntimeStatusFile(t, fixture.stateDir, "db", "healthy")

	prependFakeDockerForRollbackCLITest(t)
	t.Setenv("DOCKER_MOCK_ROLLBACK_STATE_DIR", fixture.stateDir)
	t.Setenv("DOCKER_MOCK_RESTORE_RUNTIME_UID", strconv.Itoa(os.Getuid()))
	t.Setenv("DOCKER_MOCK_RESTORE_RUNTIME_GID", strconv.Itoa(os.Getgid()))

	outcome := executeCLIWithOptions(
		[]testAppOption{withFixedTestRuntime(fixture.fixedNow, "op-restore-dryrun-1")},
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

	var obj map[string]any
	if err := json.Unmarshal([]byte(outcome.Stdout), &obj); err != nil {
		t.Fatal(err)
	}

	if dryRun, _ := obj["dry_run"].(bool); !dryRun {
		t.Fatalf("expected dry_run=true")
	}
	if wouldRun, _ := requireJSONPath(t, obj, "details", "would_run").(float64); int(wouldRun) != 5 {
		t.Fatalf("expected five would_run steps, got %v", wouldRun)
	}
	if completed, _ := requireJSONPath(t, obj, "details", "completed").(float64); int(completed) != 2 {
		t.Fatalf("expected two completed steps, got %v", completed)
	}
	if items, ok := obj["items"].([]any); !ok || len(items) != 7 {
		t.Fatalf("expected seven restore items, got %#v", obj["items"])
	}
	if _, err := os.Stat(filepath.Join(fixture.storageDir, "before.txt")); err != nil {
		t.Fatalf("expected dry-run to avoid mutating storage: %v", err)
	}
}

func mustFileMode(t *testing.T, path string) string {
	t.Helper()

	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	return strconv.FormatUint(uint64(fi.Mode().Perm()), 8)
}

func stringsContainsAny(text string, parts ...string) bool {
	for _, part := range parts {
		if strings.Contains(text, part) {
			return true
		}
	}
	return false
}
