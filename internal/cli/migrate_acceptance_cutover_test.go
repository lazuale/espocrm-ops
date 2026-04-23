package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const updateMigrateAcceptanceCLIEnv = "UPDATE_ACCEPTANCE_MIGRATE_CUTOVER"

func TestAcceptance_MigrateCLI_V2_JSONDiskAndRuntime(t *testing.T) {
	cases := []struct {
		id              string
		withForce       bool
		withConfirmProd bool
		expectNoJournal bool
		setup           func(*testing.T, *migrateCommandFixture) []string
	}{
		{
			id:              "MIG-001",
			withForce:       true,
			withConfirmProd: true,
			setup: func(t *testing.T, fixture *migrateCommandFixture) []string {
				migrateCLISetHealthyRuntime(t, fixture)
				return nil
			},
		},
		{
			id:              "MIG-002",
			withForce:       true,
			withConfirmProd: true,
			setup: func(t *testing.T, fixture *migrateCommandFixture) []string {
				migrateCLISetHealthyRuntime(t, fixture)
				return []string{
					"--db-backup", fixture.sourceBackup.DBBackup,
					"--files-backup", fixture.sourceBackup.FilesBackup,
				}
			},
		},
		{
			id:              "MIG-101",
			withForce:       true,
			withConfirmProd: true,
			setup: func(t *testing.T, fixture *migrateCommandFixture) []string {
				migrateCLISetHealthyRuntime(t, fixture)
				return []string{
					"--db-backup", fixture.sourceBackup.DBBackup,
					"--skip-files",
				}
			},
		},
		{
			id:              "MIG-102",
			withForce:       true,
			withConfirmProd: true,
			setup: func(t *testing.T, fixture *migrateCommandFixture) []string {
				migrateCLISetHealthyRuntime(t, fixture)
				return []string{
					"--files-backup", fixture.sourceBackup.FilesBackup,
					"--skip-db",
				}
			},
		},
		{
			id:              "MIG-201",
			withConfirmProd: true,
			expectNoJournal: true,
			setup: func(t *testing.T, fixture *migrateCommandFixture) []string {
				return nil
			},
		},
		{
			id:              "MIG-202",
			withForce:       true,
			expectNoJournal: true,
			setup: func(t *testing.T, fixture *migrateCommandFixture) []string {
				return nil
			},
		},
		{
			id:              "MIG-203",
			withForce:       true,
			withConfirmProd: true,
			expectNoJournal: true,
			setup: func(t *testing.T, fixture *migrateCommandFixture) []string {
				return []string{
					"--from", "prod",
					"--to", "prod",
				}
			},
		},
		{
			id:              "MIG-204",
			withForce:       true,
			withConfirmProd: true,
			expectNoJournal: true,
			setup: func(t *testing.T, fixture *migrateCommandFixture) []string {
				return []string{"--skip-db", "--skip-files"}
			},
		},
		{
			id:              "MIG-205",
			withForce:       true,
			withConfirmProd: true,
			setup: func(t *testing.T, fixture *migrateCommandFixture) []string {
				otherSet := writeBackupSet(t, fixture.sourceRoot, "espocrm-dev", "2026-04-19_07-30-00", "dev", map[string]string{
					"espo/other.txt": "other\n",
				})
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
				return nil
			},
		},
		{
			id:              "MIG-206",
			withForce:       true,
			withConfirmProd: true,
			setup: func(t *testing.T, fixture *migrateCommandFixture) []string {
				otherSet := writeBackupSet(t, fixture.sourceRoot, "espocrm-dev", "2026-04-19_08-30-00", "dev", map[string]string{
					"espo/other.txt": "other\n",
				})
				return []string{
					"--db-backup", fixture.sourceBackup.DBBackup,
					"--files-backup", otherSet.FilesBackup,
				}
			},
		},
		{
			id:              "MIG-207",
			withForce:       true,
			withConfirmProd: true,
			expectNoJournal: true,
			setup: func(t *testing.T, fixture *migrateCommandFixture) []string {
				return []string{"--db-backup", fixture.sourceBackup.DBBackup}
			},
		},
		{
			id:              "MIG-208",
			withForce:       true,
			withConfirmProd: true,
			expectNoJournal: true,
			setup: func(t *testing.T, fixture *migrateCommandFixture) []string {
				return []string{"--files-backup", fixture.sourceBackup.FilesBackup}
			},
		},
		{
			id:              "MIG-301",
			withForce:       true,
			withConfirmProd: true,
			setup: func(t *testing.T, fixture *migrateCommandFixture) []string {
				writeDoctorEnvFile(t, fixture.projectDir, "dev", map[string]string{
					"ESPOCRM_IMAGE": "espocrm/espocrm:9.3.4-apache",
				})
				writeDoctorEnvFile(t, fixture.projectDir, "prod", map[string]string{
					"ESPOCRM_IMAGE": "espocrm/espocrm:9.4.0-apache",
				})
				fixture.docker.SetRunningServices(t, "espocrm", "espocrm-daemon", "espocrm-websocket")
				return nil
			},
		},
		{
			id:              "MIG-401",
			withForce:       true,
			withConfirmProd: true,
			setup: func(t *testing.T, fixture *migrateCommandFixture) []string {
				migrateCLISetHealthyRuntime(t, fixture)
				return nil
			},
		},
		{
			id:              "MIG-402",
			withForce:       true,
			withConfirmProd: true,
			setup: func(t *testing.T, fixture *migrateCommandFixture) []string {
				migrateCLISetHealthyRuntime(t, fixture)
				return []string{"--no-start"}
			},
		},
		{
			id:              "MIG-403",
			withForce:       true,
			withConfirmProd: true,
			setup: func(t *testing.T, fixture *migrateCommandFixture) []string {
				migrateCLISetHealthyRuntime(t, fixture)
				fixture.docker.SetServiceHealth(t, "espocrm", "unhealthy")
				fixture.docker.SetHealthFailureMessage(t, "target health failed")
				return nil
			},
		},
		{
			id:              "MIG-501",
			withForce:       true,
			withConfirmProd: true,
			setup: func(t *testing.T, fixture *migrateCommandFixture) []string {
				migrateCLISetHealthyRuntime(t, fixture)
				fixture.docker.SetFailOnMatch(t, "mariadb-dump -u espocrm espocrm --single-transaction --quick --routines --triggers --events")
				return nil
			},
		},
		{
			id:              "MIG-502",
			withForce:       true,
			withConfirmProd: true,
			setup: func(t *testing.T, fixture *migrateCommandFixture) []string {
				migrateCLISetHealthyRuntime(t, fixture)
				fixture.docker.SetFailOnMatch(t, "exec -i -e MYSQL_PWD mock-db mariadb -u root")
				return nil
			},
		},
		{
			id:              "MIG-503",
			withForce:       true,
			withConfirmProd: true,
			setup: func(t *testing.T, fixture *migrateCommandFixture) []string {
				migrateCLISetHealthyRuntime(t, fixture)
				mustMkdirAll(t, filepath.Join(filepath.Dir(fixture.storageDir), ".espo.new"))
				return nil
			},
		},
		{
			id:              "MIG-504",
			withForce:       true,
			withConfirmProd: true,
			setup: func(t *testing.T, fixture *migrateCommandFixture) []string {
				migrateCLISetHealthyRuntime(t, fixture)
				fixture.docker.SetFailOnMatch(t, ":/espo-storage")
				return nil
			},
		},
		{
			id:              "MIG-505",
			withForce:       true,
			withConfirmProd: true,
			setup: func(t *testing.T, fixture *migrateCommandFixture) []string {
				return []string{
					"--db-backup", filepath.Join(fixture.sourceRoot, "db", "missing.sql.gz"),
					"--skip-files",
				}
			},
		},
		{
			id:              "MIG-506",
			withForce:       true,
			withConfirmProd: true,
			setup: func(t *testing.T, fixture *migrateCommandFixture) []string {
				mustWriteFile(t, fixture.sourceBackup.FilesBackup, "broken")
				mustWriteFile(t, fixture.sourceBackup.FilesBackup+".sha256", sha256OfFile(t, fixture.sourceBackup.FilesBackup)+"  "+filepath.Base(fixture.sourceBackup.FilesBackup)+"\n")
				return nil
			},
		},
		{
			id:              "MIG-507",
			withForce:       true,
			withConfirmProd: true,
			setup: func(t *testing.T, fixture *migrateCommandFixture) []string {
				mustWriteFile(t, fixture.sourceBackup.DBBackup+".sha256", strings.Repeat("0", 64)+"  "+filepath.Base(fixture.sourceBackup.DBBackup)+"\n")
				return []string{
					"--db-backup", fixture.sourceBackup.DBBackup,
					"--skip-files",
				}
			},
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.id, func(t *testing.T) {
			lockOpt := isolateRecoveryLocks(t)
			fixture := prepareMigrateCommandFixture(t)
			extraArgs := tc.setup(t, &fixture)

			outcome := executeCLIWithOptions(
				[]testAppOption{
					lockOpt,
					withFixedTestRuntime(fixture.fixedNow, "op-"+strings.ToLower(tc.id)),
				},
				migrateCLIAcceptanceArgs(fixture, tc.withForce, tc.withConfirmProd, extraArgs...)...,
			)

			if tc.expectNoJournal {
				assertNoJournalFiles(t, fixture.journalDir)
			}
			assertMigrateCLISelectionSurface(t, outcome)

			assertOrWriteMigrateCLIAcceptanceGoldenJSON(t, normalizeMigrateCLIAcceptanceJSON(t, outcome), filepath.Join("golden", "json", "v2_"+tc.id+".json"))
			assertOrWriteMigrateCLIAcceptanceGoldenJSON(t, captureMigrateCLIAcceptanceDiskSnapshot(t, fixture, outcome), filepath.Join("golden", "disk", "v2_"+tc.id+".json"))
			assertOrWriteMigrateCLIAcceptanceGoldenJSON(t, captureMigrateCLIAcceptanceRuntimeSnapshot(t, fixture, outcome), filepath.Join("golden", "runtime", "v2_"+tc.id+".json"))
		})
	}
}

func assertMigrateCLISelectionSurface(t *testing.T, outcome execOutcome) {
	t.Helper()

	obj := parseCLIJSONBytes(t, []byte(outcome.Stdout))
	details, ok := obj["details"].(map[string]any)
	if !ok {
		return
	}
	selectionMode := strings.TrimSpace(stringValueFromJSON(details["selection_mode"]))
	if selectionMode == "" {
		return
	}
	sourceKind := strings.TrimSpace(stringValueFromJSON(details["source_kind"]))
	switch selectionMode {
	case "auto_latest_complete":
		if sourceKind != "backup_root" {
			t.Fatalf("auto latest migrate selection must expose source_kind=backup_root, got %q", sourceKind)
		}
	case "explicit_pair", "explicit_db_only", "explicit_files_only":
		if sourceKind != "direct" {
			t.Fatalf("explicit migrate selection %q must expose source_kind=direct, got %q", selectionMode, sourceKind)
		}
	default:
		t.Fatalf("migrate selection_mode %q is outside retained product vocabulary", selectionMode)
	}
}

func migrateCLIAcceptanceArgs(fixture migrateCommandFixture, withForce, withConfirmProd bool, extraArgs ...string) []string {
	args := []string{
		"--journal-dir", fixture.journalDir,
		"--json",
		"migrate",
		"--from", "dev",
		"--to", "prod",
		"--project-dir", fixture.projectDir,
	}
	if withForce {
		args = append(args, "--force")
	}
	if withConfirmProd {
		args = append(args, "--confirm-prod", "prod")
	}
	args = append(args, extraArgs...)
	return args
}

func migrateCLISetHealthyRuntime(t *testing.T, fixture *migrateCommandFixture) {
	t.Helper()

	fixture.docker.SetRunningServices(t, "db", "espocrm", "espocrm-daemon", "espocrm-websocket")
	fixture.docker.SetServiceHealth(t, "db", "healthy")
	fixture.docker.SetServiceHealth(t, "espocrm", "healthy")
	fixture.docker.SetServiceHealth(t, "espocrm-daemon", "healthy")
	fixture.docker.SetServiceHealth(t, "espocrm-websocket", "healthy")
}

func normalizeMigrateCLIAcceptanceJSON(t *testing.T, outcome execOutcome) []byte {
	t.Helper()

	obj := parseCLIJSONBytes(t, []byte(outcome.Stdout))
	obj["process_exit_code"] = outcome.ExitCode
	delete(obj, "message")
	delete(obj, "timing")

	if details, ok := obj["details"].(map[string]any); ok {
		if _, ok := details["selection_mode"]; !ok {
			details["selection_mode"] = ""
		}
		if _, ok := details["source_kind"]; !ok {
			details["source_kind"] = ""
		}
	}
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

	replacements := normalizeArtifactPlaceholders(obj, map[string]string{
		"project_dir":             "REPLACE_PROJECT_DIR",
		"compose_file":            "REPLACE_COMPOSE_FILE",
		"source_env_file":         "REPLACE_SOURCE_ENV_FILE",
		"target_env_file":         "REPLACE_TARGET_ENV_FILE",
		"source_backup_root":      "REPLACE_SOURCE_BACKUP_ROOT",
		"target_backup_root":      "REPLACE_TARGET_BACKUP_ROOT",
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
	normalizeWarningsPaths(obj, replacements)
	return marshalCLIJSON(t, obj)
}

func captureMigrateCLIAcceptanceDiskSnapshot(t *testing.T, fixture migrateCommandFixture, outcome execOutcome) []byte {
	t.Helper()

	targetBackupRoot := filepath.Join(fixture.projectDir, "backups", "prod")
	obj := parseCLIJSONBytes(t, []byte(outcome.Stdout))
	artifacts, _ := obj["artifacts"].(map[string]any)

	snapshot := map[string]any{
		"process_exit_code":        outcome.ExitCode,
		"storage_entries_after":    collectRelativeTreeEntries(t, fixture.storageDir),
		"storage_modes_after":      collectRelativeModes(t, fixture.storageDir),
		"source_backup_root_after": collectRelativeTreeEntries(t, fixture.sourceRoot),
		"target_backup_root_after": collectRelativeTreeEntries(t, targetBackupRoot),
	}
	if path := strings.TrimSpace(stringValueFromJSON(artifacts["snapshot_manifest_json"])); fileExists(path) {
		manifest := mustReadJSONFile(t, path)
		if manifestObj, ok := manifest.(map[string]any); ok {
			delete(manifestObj, "checksums")
		}
		snapshot["snapshot_manifest_json"] = manifest
	}
	if path := strings.TrimSpace(stringValueFromJSON(artifacts["snapshot_db_backup"])); fileExists(path) {
		snapshot["snapshot_db_sql"] = mustReadGzipText(t, path)
	}
	if path := strings.TrimSpace(stringValueFromJSON(artifacts["snapshot_files_backup"])); fileExists(path) {
		snapshot["snapshot_files_entries"] = mustReadTarEntries(t, path)
	}

	for _, replacement := range []struct {
		path        string
		placeholder string
	}{
		{fixture.projectDir, "REPLACE_PROJECT_DIR"},
		{fixture.sourceRoot, "REPLACE_SOURCE_BACKUP_ROOT"},
		{targetBackupRoot, "REPLACE_TARGET_BACKUP_ROOT"},
		{fixture.storageDir, "REPLACE_STORAGE_DIR"},
	} {
		replaceMigrateCutoverStringValues(snapshot, replacement.path, replacement.placeholder)
	}
	return marshalBackupAcceptanceSnapshot(t, snapshot)
}

func captureMigrateCLIAcceptanceRuntimeSnapshot(t *testing.T, fixture migrateCommandFixture, outcome execOutcome) []byte {
	t.Helper()

	obj := parseCLIJSONBytes(t, []byte(outcome.Stdout))
	details, _ := obj["details"].(map[string]any)
	itemStatuses := migrateCLIAcceptanceItemStatuses(obj)
	artifacts, _ := obj["artifacts"].(map[string]any)

	snapshot := map[string]any{
		"process_exit_code":   outcome.ExitCode,
		"migrate_calls":       migrateCLIAcceptanceCalls(details, itemStatuses),
		"snapshot_calls":      migrateCLIAcceptanceSnapshotCalls(details, itemStatuses),
		"post_check_services": migrateCLIAcceptancePostCheckServices(details, itemStatuses),
		"restored_db_path":    migrateCLIAcceptanceRestoredDBPath(artifacts, itemStatuses),
		"running_after":       readRunningServicesSnapshot(t, fixture.docker.stateDir),
	}
	for _, replacement := range []struct {
		path        string
		placeholder string
	}{
		{fixture.projectDir, "REPLACE_PROJECT_DIR"},
		{fixture.sourceRoot, "REPLACE_SOURCE_BACKUP_ROOT"},
		{filepath.Join(fixture.projectDir, "backups", "prod"), "REPLACE_TARGET_BACKUP_ROOT"},
		{fixture.sourceBackup.DBBackup, "REPLACE_DB_BACKUP"},
		{fixture.sourceBackup.FilesBackup, "REPLACE_FILES_BACKUP"},
	} {
		replaceMigrateCutoverStringValues(snapshot, replacement.path, replacement.placeholder)
	}
	return marshalBackupAcceptanceSnapshot(t, snapshot)
}

func migrateCLIAcceptanceItemStatuses(obj map[string]any) map[string]string {
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

func migrateCLIAcceptanceCalls(details map[string]any, itemStatuses map[string]string) []string {
	status := itemStatuses["runtime_prepare"]
	if status != "completed" && status != "failed" {
		return nil
	}

	calls := []string{"running_services"}
	if requireJSONBoolValue(details["started_db_temporarily"]) {
		calls = append(calls, "start_db")
	}
	if requireJSONBoolValue(details["app_services_were_running"]) {
		calls = append(calls, "stop_services")
	}
	if status := itemStatuses["db_restore"]; status == "completed" || status == "failed" {
		calls = append(calls, "restore_database")
	}
	if status := itemStatuses["permission_reconcile"]; status == "completed" || status == "failed" {
		calls = append(calls, "reconcile_files_permissions")
	}
	if status := itemStatuses["runtime_return"]; status == "completed" || status == "failed" {
		switch {
		case requireJSONBoolValue(details["app_services_were_running"]):
			calls = append(calls, "start_services")
		case requireJSONBoolValue(details["started_db_temporarily"]):
			calls = append(calls, "stop_services")
		}
	}
	if status := itemStatuses["post_migrate_check"]; status == "completed" || status == "failed" {
		calls = append(calls, "post_migrate_check")
		if status == "completed" {
			calls = append(calls, "running_services")
		}
	}
	return calls
}

func migrateCLIAcceptanceSnapshotCalls(details map[string]any, itemStatuses map[string]string) []string {
	status := itemStatuses["target_snapshot"]
	if status != "completed" && status != "failed" {
		return nil
	}

	calls := []string{"running_services"}
	if requireJSONBoolValue(details["app_services_were_running"]) {
		calls = append(calls, "stop_services")
	}
	if !requireJSONBoolValue(details["skip_db"]) {
		calls = append(calls, "dump_database")
	}
	if status == "completed" && !requireJSONBoolValue(details["skip_files"]) {
		calls = append(calls, "archive_files")
	}
	if requireJSONBoolValue(details["app_services_were_running"]) {
		calls = append(calls, "start_services")
	}
	return calls
}

func migrateCLIAcceptancePostCheckServices(details map[string]any, itemStatuses map[string]string) []string {
	status := itemStatuses["post_migrate_check"]
	if status != "completed" && status != "failed" {
		return nil
	}

	if requireJSONBoolValue(details["no_start"]) {
		return []string{"db"}
	}
	if requireJSONBoolValue(details["app_services_were_running"]) {
		return []string{"db", "espocrm", "espocrm-daemon", "espocrm-websocket"}
	}
	if requireJSONBoolValue(details["started_db_temporarily"]) {
		return nil
	}
	return []string{"db"}
}

func migrateCLIAcceptanceRestoredDBPath(artifacts map[string]any, itemStatuses map[string]string) string {
	if itemStatuses["db_restore"] != "completed" {
		return ""
	}
	return stringValueFromJSON(artifacts["db_backup"])
}

func replaceMigrateCutoverStringValues(value any, old, new string) {
	if strings.TrimSpace(old) == "" || old == new {
		return
	}
	switch typed := value.(type) {
	case map[string]any:
		for key, nested := range typed {
			if text, ok := nested.(string); ok {
				typed[key] = strings.ReplaceAll(text, old, new)
				continue
			}
			replaceMigrateCutoverStringValues(nested, old, new)
		}
	case []any:
		for idx, nested := range typed {
			if text, ok := nested.(string); ok {
				typed[idx] = strings.ReplaceAll(text, old, new)
				continue
			}
			replaceMigrateCutoverStringValues(nested, old, new)
		}
	}
}

func assertOrWriteMigrateCLIAcceptanceGoldenJSON(t *testing.T, got []byte, relPath string) {
	t.Helper()

	path := migrateCLIAcceptanceGoldenPath(relPath)
	gotNorm := normalizeMigrateAcceptanceReferenceJSONBytes(t, got)

	if os.Getenv(updateMigrateAcceptanceCLIEnv) == "1" {
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
	wantNorm := normalizeMigrateAcceptanceReferenceJSONBytes(t, want)
	if string(gotNorm) != string(wantNorm) {
		t.Fatalf("golden mismatch for %s\nGOT:\n%s\n\nWANT:\n%s", relPath, gotNorm, wantNorm)
	}
}

func migrateCLIAcceptanceGoldenPath(relPath string) string {
	return filepath.Join("..", "..", "acceptance", "v2", "migrate", "cutover", relPath)
}
