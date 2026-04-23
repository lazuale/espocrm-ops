package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"
)

const updateRestoreAcceptanceCLIEnv = "UPDATE_ACCEPTANCE_RESTORE"

func TestAcceptance_RestoreCLI_V2_JSONDiskAndRuntime(t *testing.T) {
	cases := []struct {
		id    string
		setup func(*testing.T, *restoreCommandFixture) []string
	}{
		{
			id: "RST-001",
			setup: func(t *testing.T, fixture *restoreCommandFixture) []string {
				restoreCLISetHealthyRuntime(t, fixture)
				return []string{"--manifest", fixture.backupSet.ManifestJSON}
			},
		},
		{
			id: "RST-002",
			setup: func(t *testing.T, fixture *restoreCommandFixture) []string {
				restoreCLISetHealthyRuntime(t, fixture)
				return []string{
					"--db-backup", fixture.backupSet.DBBackup,
					"--files-backup", fixture.backupSet.FilesBackup,
				}
			},
		},
		{
			id: "RST-101",
			setup: func(t *testing.T, fixture *restoreCommandFixture) []string {
				restoreCLISetHealthyRuntime(t, fixture)
				return []string{
					"--db-backup", fixture.backupSet.DBBackup,
					"--skip-files",
					"--no-snapshot",
				}
			},
		},
		{
			id: "RST-102",
			setup: func(t *testing.T, fixture *restoreCommandFixture) []string {
				restoreCLISetHealthyRuntime(t, fixture)
				return []string{
					"--files-backup", fixture.backupSet.FilesBackup,
					"--skip-db",
					"--no-snapshot",
					"--no-start",
				}
			},
		},
		{
			id: "RST-204",
			setup: func(t *testing.T, fixture *restoreCommandFixture) []string {
				otherSet := writeBackupSet(t, fixture.backupRoot, "espocrm-prod", "2026-04-19_07-00-00", "prod", map[string]string{
					"espo/data/nested/file.txt": "other",
				})
				return []string{
					"--db-backup", fixture.backupSet.DBBackup,
					"--files-backup", otherSet.FilesBackup,
					"--no-snapshot",
				}
			},
		},
		{
			id: "RST-301",
			setup: func(t *testing.T, fixture *restoreCommandFixture) []string {
				restoreCLISetHealthyRuntime(t, fixture)
				return []string{"--manifest", fixture.backupSet.ManifestJSON}
			},
		},
		{
			id: "RST-302",
			setup: func(t *testing.T, fixture *restoreCommandFixture) []string {
				restoreCLISetHealthyRuntime(t, fixture)
				return []string{
					"--manifest", fixture.backupSet.ManifestJSON,
					"--no-snapshot",
				}
			},
		},
		{
			id: "RST-303",
			setup: func(t *testing.T, fixture *restoreCommandFixture) []string {
				restoreCLISetHealthyRuntime(t, fixture)
				fixture.docker.SetFailOnMatch(t, "mariadb-dump -u espocrm espocrm --single-transaction --quick --routines --triggers --events")
				return []string{"--manifest", fixture.backupSet.ManifestJSON}
			},
		},
		{
			id: "RST-401",
			setup: func(t *testing.T, fixture *restoreCommandFixture) []string {
				restoreCLISetHealthyRuntime(t, fixture)
				return []string{
					"--manifest", fixture.backupSet.ManifestJSON,
					"--no-snapshot",
				}
			},
		},
		{
			id: "RST-402",
			setup: func(t *testing.T, fixture *restoreCommandFixture) []string {
				restoreCLISetHealthyRuntime(t, fixture)
				return []string{
					"--manifest", fixture.backupSet.ManifestJSON,
					"--no-snapshot",
					"--no-stop",
				}
			},
		},
		{
			id: "RST-403",
			setup: func(t *testing.T, fixture *restoreCommandFixture) []string {
				restoreCLISetHealthyRuntime(t, fixture)
				return []string{
					"--manifest", fixture.backupSet.ManifestJSON,
					"--no-snapshot",
					"--no-start",
				}
			},
		},
		{
			id: "RST-404",
			setup: func(t *testing.T, fixture *restoreCommandFixture) []string {
				restoreCLISetHealthyRuntime(t, fixture)
				fixture.docker.SetFailOnMatch(t, "up -d espocrm espocrm-daemon espocrm-websocket")
				return []string{
					"--manifest", fixture.backupSet.ManifestJSON,
					"--no-snapshot",
				}
			},
		},
		{
			id: "RST-501",
			setup: func(t *testing.T, fixture *restoreCommandFixture) []string {
				restoreCLISetHealthyRuntime(t, fixture)
				fixture.docker.SetFailOnMatch(t, "exec -i -e MYSQL_PWD mock-db mariadb -u root")
				return []string{
					"--manifest", fixture.backupSet.ManifestJSON,
					"--no-snapshot",
				}
			},
		},
		{
			id: "RST-502",
			setup: func(t *testing.T, fixture *restoreCommandFixture) []string {
				restoreCLISetHealthyRuntime(t, fixture)
				mustMkdirAll(t, filepath.Join(filepath.Dir(fixture.storageDir), ".espo.new"))
				return []string{
					"--manifest", fixture.backupSet.ManifestJSON,
					"--no-snapshot",
				}
			},
		},
		{
			id: "RST-503",
			setup: func(t *testing.T, fixture *restoreCommandFixture) []string {
				restoreCLISetHealthyRuntime(t, fixture)
				fixture.docker.SetFailOnMatch(t, ":/espo-storage")
				return []string{
					"--manifest", fixture.backupSet.ManifestJSON,
					"--no-snapshot",
				}
			},
		},
		{
			id: "RST-504",
			setup: func(t *testing.T, fixture *restoreCommandFixture) []string {
				mustWriteFile(t, fixture.backupSet.ManifestJSON, "{")
				return []string{
					"--manifest", fixture.backupSet.ManifestJSON,
					"--no-snapshot",
				}
			},
		},
		{
			id: "RST-505",
			setup: func(t *testing.T, fixture *restoreCommandFixture) []string {
				if err := os.Remove(fixture.backupSet.DBBackup); err != nil {
					t.Fatal(err)
				}
				return []string{
					"--manifest", fixture.backupSet.ManifestJSON,
					"--no-snapshot",
				}
			},
		},
		{
			id: "RST-506",
			setup: func(t *testing.T, fixture *restoreCommandFixture) []string {
				mustWriteFile(t, fixture.backupSet.FilesBackup, "not archive")
				mustWriteFile(t, fixture.backupSet.FilesBackup+".sha256", sha256OfFile(t, fixture.backupSet.FilesBackup)+"  "+filepath.Base(fixture.backupSet.FilesBackup)+"\n")
				restoreCLISetHealthyRuntime(t, fixture)
				return []string{
					"--manifest", fixture.backupSet.ManifestJSON,
					"--no-snapshot",
				}
			},
		},
		{
			id: "RST-507",
			setup: func(t *testing.T, fixture *restoreCommandFixture) []string {
				mustWriteFile(t, fixture.backupSet.DBBackup+".sha256", strings.Repeat("0", 64)+"  "+filepath.Base(fixture.backupSet.DBBackup)+"\n")
				return []string{
					"--manifest", fixture.backupSet.ManifestJSON,
					"--no-snapshot",
				}
			},
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.id, func(t *testing.T) {
			lockOpt := isolateRecoveryLocks(t)
			fixture := prepareRestoreCommandFixture(t, "prod", restoreCLIAcceptanceFiles(tc.id))
			if err := os.Remove(filepath.Join(fixture.storageDir, "before.txt")); err != nil {
				t.Fatal(err)
			}
			mustWriteFile(t, filepath.Join(fixture.storageDir, "stale.txt"), "stale\n")

			beforeStorage := collectRelativeTreeEntries(t, fixture.storageDir)
			extraArgs := tc.setup(t, &fixture)
			outcome := executeCLIWithOptions(
				[]testAppOption{
					lockOpt,
					withFixedTestRuntime(fixture.fixedNow, "op-"+strings.ToLower(tc.id)),
				},
				restoreAcceptanceReferenceArgs(fixture, extraArgs...)...,
			)

			assertOrWriteRestoreCLIAcceptanceGoldenJSON(t, normalizeRestoreCLIAcceptanceJSON(t, outcome), filepath.Join("golden", "json", "v2_"+tc.id+".json"))
			assertOrWriteRestoreCLIAcceptanceGoldenJSON(t, captureRestoreCLIAcceptanceDiskSnapshot(t, fixture, outcome, beforeStorage), filepath.Join("golden", "disk", "v2_"+tc.id+".json"))
			assertOrWriteRestoreCLIAcceptanceGoldenJSON(t, captureRestoreCLIAcceptanceRuntimeSnapshot(t, fixture, outcome), filepath.Join("golden", "runtime", "v2_"+tc.id+".json"))
		})
	}
}

func restoreCLISetHealthyRuntime(t *testing.T, fixture *restoreCommandFixture) {
	t.Helper()

	fixture.docker.SetRunningServices(t, "db", "espocrm", "espocrm-daemon", "espocrm-websocket")
	fixture.docker.SetServiceHealth(t, "db", "healthy")
	fixture.docker.SetServiceHealth(t, "espocrm", "healthy")
	fixture.docker.SetServiceHealth(t, "espocrm-daemon", "healthy")
	fixture.docker.SetServiceHealth(t, "espocrm-websocket", "healthy")
}

func restoreCLIAcceptanceFiles(id string) map[string]string {
	if id == "RST-503" {
		return map[string]string{
			"espo/data/nested/file.txt":      "hello\n",
			"espo/custom/modules/module.txt": "custom\n",
			"espo/client/custom/app.js":      "client\n",
			"espo/upload/blob.txt":           "upload\n",
		}
	}
	return map[string]string{
		"espo/file.txt": "restored\n",
	}
}

func normalizeRestoreCLIAcceptanceJSON(t *testing.T, outcome execOutcome) []byte {
	t.Helper()

	obj := parseCLIJSONBytes(t, []byte(outcome.Stdout))
	obj["process_exit_code"] = outcome.ExitCode
	delete(obj, "message")
	delete(obj, "timing")

	replacements := normalizeArtifactPlaceholders(obj, map[string]string{
		"project_dir":             "REPLACE_PROJECT_DIR",
		"compose_file":            "REPLACE_COMPOSE_FILE",
		"env_file":                "REPLACE_ENV_FILE",
		"backup_root":             "REPLACE_BACKUP_ROOT",
		"manifest_txt":            "REPLACE_MANIFEST_TXT",
		"manifest_json":           "REPLACE_MANIFEST_JSON",
		"db_backup":               "REPLACE_DB_BACKUP",
		"files_backup":            "REPLACE_FILES_BACKUP",
		"snapshot_manifest_txt":   "REPLACE_SNAPSHOT_MANIFEST_TXT",
		"snapshot_manifest_json":  "REPLACE_SNAPSHOT_MANIFEST_JSON",
		"snapshot_db_backup":      "REPLACE_SNAPSHOT_DB_BACKUP",
		"snapshot_files_backup":   "REPLACE_SNAPSHOT_FILES_BACKUP",
		"snapshot_db_checksum":    "REPLACE_SNAPSHOT_DB_CHECKSUM",
		"snapshot_files_checksum": "REPLACE_SNAPSHOT_FILES_CHECKSUM",
	})
	if errObj, ok := obj["error"].(map[string]any); ok {
		delete(errObj, "message")
	}
	if items, ok := obj["items"].([]any); ok {
		normalized := make([]any, 0, len(items))
		for _, rawItem := range items {
			item, ok := rawItem.(map[string]any)
			if !ok {
				continue
			}
			normalized = append(normalized, map[string]any{
				"code":   item["code"],
				"status": item["status"],
			})
		}
		obj["items"] = normalized
	}
	normalizeWarningsPaths(obj, replacements)
	return marshalCLIJSON(t, obj)
}

func captureRestoreCLIAcceptanceDiskSnapshot(t *testing.T, fixture restoreCommandFixture, outcome execOutcome, beforeStorage []string) []byte {
	t.Helper()

	itemStatuses := restoreCLIItemStatuses(parseCLIJSONBytes(t, []byte(outcome.Stdout)))
	snapshot := map[string]any{
		"process_exit_code": outcome.ExitCode,
		"before_storage":    beforeStorage,
		"after_storage":     collectRelativeTreeEntries(t, fixture.storageDir),
		"snapshot_entries":  collectRestoreSnapshotEntries(t, fixture.backupRoot, fixture.fixedNow, itemStatuses["snapshot_recovery_point"]),
	}
	if itemStatuses["permission_reconcile"] == "failed" {
		snapshot["storage_modes_after"] = collectRelativeModes(t, fixture.storageDir)
	}
	return marshalRestoreCLIAcceptanceJSON(t, snapshot)
}

func captureRestoreCLIAcceptanceRuntimeSnapshot(t *testing.T, fixture restoreCommandFixture, outcome execOutcome) []byte {
	t.Helper()

	obj := parseCLIJSONBytes(t, []byte(outcome.Stdout))
	details := requireJSONObject(t, obj, "details")
	itemStatuses := restoreCLIItemStatuses(obj)
	restoreCalls := restoreCLIRestoreCalls(details, itemStatuses)
	snapshotCalls := restoreCLISnapshotCalls(details, itemStatuses)
	artifacts := requireJSONObject(t, obj, "artifacts")

	snapshot := map[string]any{
		"process_exit_code":   outcome.ExitCode,
		"restore_calls":       restoreCalls,
		"snapshot_calls":      snapshotCalls,
		"post_check_services": restoreCLIPostCheckServices(details, itemStatuses),
		"restored_db_path":    restoreCLIRestoredDBPath(artifacts, itemStatuses),
	}
	return normalizeRestoreCLIRuntimeSnapshot(t, snapshot)
}

func restoreCLIItemStatuses(obj map[string]any) map[string]string {
	items, _ := obj["items"].([]any)
	statuses := make(map[string]string, len(items))
	for _, rawItem := range items {
		item, ok := rawItem.(map[string]any)
		if !ok {
			continue
		}
		code, _ := item["code"].(string)
		status, _ := item["status"].(string)
		statuses[code] = status
	}
	return statuses
}

func restoreCLIRestoreCalls(details map[string]any, itemStatuses map[string]string) []string {
	if itemStatuses["runtime_prepare"] != "completed" && itemStatuses["runtime_prepare"] != "failed" {
		return nil
	}

	calls := []string{"running_services"}
	if requireJSONBoolValue(details["app_services_were_running"]) && !requireJSONBoolValue(details["no_stop"]) {
		calls = append(calls, "stop_services")
	}
	if status := itemStatuses["db_restore"]; status == "completed" || status == "failed" {
		calls = append(calls, "restore_database")
	}
	if status := itemStatuses["permission_reconcile"]; status == "completed" || status == "failed" {
		calls = append(calls, "reconcile_files_permissions")
	}
	if status := itemStatuses["runtime_return"]; status == "completed" || status == "failed" {
		calls = append(calls, "start_services")
	}
	if status := itemStatuses["post_restore_check"]; status == "completed" || status == "failed" {
		calls = append(calls, "post_restore_check")
	}
	return calls
}

func restoreCLISnapshotCalls(details map[string]any, itemStatuses map[string]string) []string {
	status := itemStatuses["snapshot_recovery_point"]
	if status != "completed" && status != "failed" {
		return nil
	}

	calls := []string{}
	if !requireJSONBoolValue(details["no_stop"]) {
		calls = append(calls, "running_services")
	}
	if !requireJSONBoolValue(details["skip_db"]) {
		calls = append(calls, "dump_database")
	}
	if status == "completed" && !requireJSONBoolValue(details["skip_files"]) {
		calls = append(calls, "archive_files")
	}
	return calls
}

func restoreCLIPostCheckServices(details map[string]any, itemStatuses map[string]string) []string {
	status := itemStatuses["post_restore_check"]
	if status != "completed" && status != "failed" {
		return nil
	}

	services := []string{}
	appServicesWereRunning := requireJSONBoolValue(details["app_services_were_running"])
	noStop := requireJSONBoolValue(details["no_stop"])
	noStart := requireJSONBoolValue(details["no_start"])
	skipDB := requireJSONBoolValue(details["skip_db"])

	if appServicesWereRunning && (noStop || !noStart) {
		services = append(services, "espocrm", "espocrm-daemon", "espocrm-websocket")
	}
	if !skipDB {
		services = append(services, "db")
	}
	if len(services) == 0 {
		return nil
	}
	return services
}

func restoreCLIRestoredDBPath(artifacts map[string]any, itemStatuses map[string]string) string {
	if itemStatuses["db_restore"] != "completed" {
		return ""
	}
	path, _ := artifacts["db_backup"].(string)
	return path
}

func normalizeRestoreCLIRuntimeSnapshot(t *testing.T, snapshot map[string]any) []byte {
	t.Helper()

	if path := stringValueFromJSON(snapshot["restored_db_path"]); strings.TrimSpace(path) != "" {
		snapshot["restored_db_path"] = "REPLACE_DB_BACKUP"
	}
	return marshalRestoreCLIAcceptanceJSON(t, snapshot)
}

func collectRestoreSnapshotEntries(t *testing.T, backupRoot string, createdAt time.Time, snapshotStatus string) []string {
	t.Helper()

	all := collectRelativeTreeEntries(t, backupRoot)
	stamp := createdAt.UTC().Format("2006-01-02_15-04-05")
	entries := []string{}
	for _, entry := range all {
		if strings.Contains(entry, stamp) {
			entries = append(entries, entry)
		}
	}
	if len(entries) == 0 {
		if snapshotStatus == "failed" {
			failedEntries := []string{}
			for _, dir := range []string{"db/", "files/", "manifests/"} {
				if containsString(all, dir) {
					failedEntries = append(failedEntries, dir)
				}
			}
			if len(failedEntries) != 0 {
				sort.Strings(failedEntries)
				return failedEntries
			}
		}
		return nil
	}
	for _, dir := range []string{"db/", "files/", "manifests/"} {
		if containsString(all, dir) {
			entries = append(entries, dir)
		}
	}
	sort.Strings(entries)
	return entries
}

func containsString(items []string, target string) bool {
	for _, item := range items {
		if item == target {
			return true
		}
	}
	return false
}

func marshalRestoreCLIAcceptanceJSON(t *testing.T, value any) []byte {
	t.Helper()

	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func assertOrWriteRestoreCLIAcceptanceGoldenJSON(t *testing.T, got []byte, relPath string) {
	t.Helper()

	path := acceptanceRestoreGoldenPath(relPath)
	gotNorm := normalizeRestoreAcceptanceReferenceJSONBytes(t, got)

	if os.Getenv(updateRestoreAcceptanceCLIEnv) == "1" {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, append(gotNorm, '\n'), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}

	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden %s: %v", path, err)
	}
	wantNorm := normalizeRestoreAcceptanceReferenceJSONBytes(t, want)
	if string(gotNorm) != string(wantNorm) {
		t.Fatalf("golden mismatch for %s\nGOT:\n%s\n\nWANT:\n%s", relPath, gotNorm, wantNorm)
	}
}

func requireJSONBoolValue(raw any) bool {
	value, _ := raw.(bool)
	return value
}
