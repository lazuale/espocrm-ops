package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const updateMigrateAcceptanceReferenceEnv = "UPDATE_ACCEPTANCE_MIGRATE_REFERENCE"

func TestAcceptanceReference_MigrateV1_JSONDiskAndRuntime(t *testing.T) {
	cases := []struct {
		id    string
		setup func(*testing.T, *migrateCommandFixture) []string
	}{
		{
			id: "MIG-001",
			setup: func(t *testing.T, fixture *migrateCommandFixture) []string {
				fixture.docker.SetRunningServices(t, "espocrm", "espocrm-daemon", "espocrm-websocket")
				fixture.docker.SetServiceHealth(t, "db", "healthy")
				fixture.docker.EnableLog(t)
				return nil
			},
		},
		{
			id: "MIG-002",
			setup: func(t *testing.T, fixture *migrateCommandFixture) []string {
				fixture.docker.SetRunningServices(t, "espocrm", "espocrm-daemon", "espocrm-websocket")
				fixture.docker.SetServiceHealth(t, "db", "healthy")
				fixture.docker.EnableLog(t)
				return []string{
					"--db-backup", fixture.sourceBackup.DBBackup,
					"--files-backup", fixture.sourceBackup.FilesBackup,
				}
			},
		},
		{
			id: "MIG-101",
			setup: func(t *testing.T, fixture *migrateCommandFixture) []string {
				fixture.docker.SetRunningServices(t, "espocrm", "espocrm-daemon", "espocrm-websocket")
				fixture.docker.SetServiceHealth(t, "db", "healthy")
				fixture.docker.EnableLog(t)
				return []string{
					"--skip-files",
					"--db-backup", fixture.sourceBackup.DBBackup,
				}
			},
		},
		{
			id: "MIG-102",
			setup: func(t *testing.T, fixture *migrateCommandFixture) []string {
				fixture.docker.SetRunningServices(t, "espocrm", "espocrm-daemon", "espocrm-websocket")
				fixture.docker.SetServiceHealth(t, "db", "healthy")
				fixture.docker.EnableLog(t)
				return []string{
					"--skip-db",
					"--files-backup", fixture.sourceBackup.FilesBackup,
				}
			},
		},
		{
			id: "MIG-205",
			setup: func(t *testing.T, fixture *migrateCommandFixture) []string {
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
				fixture.docker.EnableLog(t)
				return nil
			},
		},
		{
			id: "MIG-206",
			setup: func(t *testing.T, fixture *migrateCommandFixture) []string {
				otherSet := writeBackupSet(t, fixture.sourceRoot, "espocrm-dev", "2026-04-19_07-00-00", "dev", nil)
				fixture.docker.EnableLog(t)
				return []string{
					"--db-backup", fixture.sourceBackup.DBBackup,
					"--files-backup", otherSet.FilesBackup,
				}
			},
		},
		{
			id: "MIG-207",
			setup: func(t *testing.T, fixture *migrateCommandFixture) []string {
				fixture.docker.SetRunningServices(t, "espocrm", "espocrm-daemon", "espocrm-websocket")
				fixture.docker.SetServiceHealth(t, "db", "healthy")
				fixture.docker.EnableLog(t)
				return []string{
					"--db-backup", fixture.sourceBackup.DBBackup,
				}
			},
		},
		{
			id: "MIG-208",
			setup: func(t *testing.T, fixture *migrateCommandFixture) []string {
				fixture.docker.SetRunningServices(t, "espocrm", "espocrm-daemon", "espocrm-websocket")
				fixture.docker.SetServiceHealth(t, "db", "healthy")
				fixture.docker.EnableLog(t)
				return []string{
					"--files-backup", fixture.sourceBackup.FilesBackup,
				}
			},
		},
		{
			id: "MIG-301",
			setup: func(t *testing.T, fixture *migrateCommandFixture) []string {
				writeDoctorEnvFile(t, fixture.projectDir, "dev", map[string]string{
					"ESPOCRM_IMAGE": "espocrm/espocrm:9.3.4-apache",
				})
				writeDoctorEnvFile(t, fixture.projectDir, "prod", map[string]string{
					"ESPOCRM_IMAGE": "espocrm/espocrm:9.4.0-apache",
				})
				fixture.docker.EnableLog(t)
				return nil
			},
		},
		{
			id: "MIG-402",
			setup: func(t *testing.T, fixture *migrateCommandFixture) []string {
				fixture.docker.SetRunningServices(t, "espocrm", "espocrm-daemon", "espocrm-websocket")
				fixture.docker.SetServiceHealth(t, "db", "healthy")
				fixture.docker.EnableLog(t)
				return []string{"--no-start"}
			},
		},
		{
			id: "MIG-403",
			setup: func(t *testing.T, fixture *migrateCommandFixture) []string {
				fixture.docker.SetRunningServices(t, "espocrm", "espocrm-daemon", "espocrm-websocket")
				fixture.docker.SetServiceHealth(t, "db", "healthy")
				fixture.docker.SetServiceHealth(t, "espocrm", "unhealthy")
				fixture.docker.SetHealthFailureMessage(t, "target health failed")
				fixture.docker.EnableLog(t)
				return nil
			},
		},
		{
			id: "MIG-504",
			setup: func(t *testing.T, fixture *migrateCommandFixture) []string {
				fixture.docker.SetRunningServices(t, "espocrm", "espocrm-daemon", "espocrm-websocket")
				fixture.docker.SetServiceHealth(t, "db", "healthy")
				fixture.docker.SetFailOnMatch(t, ":/espo-storage")
				fixture.docker.EnableLog(t)
				return nil
			},
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.id, func(t *testing.T) {
			lockOpt := isolateRecoveryLocks(t)
			fixture := prepareMigrateCommandFixture(t)
			targetBackupRoot := filepath.Join(fixture.projectDir, "backups", "prod")
			beforeStorage := collectRelativeTreeEntries(t, fixture.storageDir)
			beforeSourceBackupRoot := collectRelativeTreeEntries(t, fixture.sourceRoot)
			beforeTargetBackupRoot := collectRelativeTreeEntries(t, targetBackupRoot)
			beforeRunning := readRunningServicesSnapshot(t, fixture.docker.stateDir)

			extraArgs := tc.setup(t, &fixture)
			outcome := executeCLIWithOptions(
				[]testAppOption{
					lockOpt,
					withFixedTestRuntime(fixture.fixedNow, "op-"+strings.ToLower(tc.id)),
				},
				migrateAcceptanceReferenceArgs(fixture, extraArgs...)...,
			)

			normalizedJSON := normalizeMigrateAcceptanceReferenceJSON(t, fixture, outcome)
			assertOrWriteMigrateAcceptanceReferenceGoldenJSON(t, normalizedJSON, filepath.Join("golden", "json", "v1_"+tc.id+".json"))

			disk := captureMigrateAcceptanceReferenceDiskSnapshot(t, fixture, outcome, beforeStorage, beforeSourceBackupRoot, beforeTargetBackupRoot)
			assertOrWriteMigrateAcceptanceReferenceGoldenJSON(t, disk, filepath.Join("golden", "disk", "v1_"+tc.id+".json"))

			runtime := captureMigrateAcceptanceReferenceRuntimeSnapshot(t, fixture, outcome, beforeRunning)
			assertOrWriteMigrateAcceptanceReferenceGoldenJSON(t, runtime, filepath.Join("golden", "runtime", "v1_"+tc.id+".json"))
		})
	}
}

func migrateAcceptanceReferenceArgs(fixture migrateCommandFixture, extraArgs ...string) []string {
	args := []string{
		"--journal-dir", fixture.journalDir,
		"--json",
		"migrate",
		"--from", "dev",
		"--to", "prod",
		"--project-dir", fixture.projectDir,
		"--force",
		"--confirm-prod", "prod",
	}
	args = append(args, extraArgs...)
	return args
}

func normalizeMigrateAcceptanceReferenceJSON(t *testing.T, fixture migrateCommandFixture, outcome execOutcome) []byte {
	t.Helper()

	obj := parseCLIJSONBytes(t, []byte(outcome.Stdout))
	obj["process_exit_code"] = outcome.ExitCode

	replacements := map[string]string{
		fixture.projectDir: "REPLACE_PROJECT_DIR",
		filepath.Join(fixture.projectDir, "compose.yaml"): "REPLACE_COMPOSE_FILE",
		filepath.Join(fixture.projectDir, ".env.dev"):     "REPLACE_SOURCE_ENV_FILE",
		filepath.Join(fixture.projectDir, ".env.prod"):    "REPLACE_TARGET_ENV_FILE",
		fixture.sourceRoot: "REPLACE_SOURCE_BACKUP_ROOT",
		filepath.Join(fixture.projectDir, "backups", "prod"): "REPLACE_TARGET_BACKUP_ROOT",
		fixture.storageDir:                "REPLACE_STORAGE_DIR",
		fixture.sourceBackup.ManifestTXT:  "REPLACE_MANIFEST_TXT",
		fixture.sourceBackup.ManifestJSON: "REPLACE_MANIFEST_JSON",
		fixture.sourceBackup.DBBackup:     "REPLACE_DB_BACKUP",
		fixture.sourceBackup.FilesBackup:  "REPLACE_FILES_BACKUP",
	}
	normalizeArtifactPlaceholders(obj, map[string]string{
		"project_dir":            "REPLACE_PROJECT_DIR",
		"compose_file":           "REPLACE_COMPOSE_FILE",
		"source_env_file":        "REPLACE_SOURCE_ENV_FILE",
		"target_env_file":        "REPLACE_TARGET_ENV_FILE",
		"source_backup_root":     "REPLACE_SOURCE_BACKUP_ROOT",
		"target_backup_root":     "REPLACE_TARGET_BACKUP_ROOT",
		"manifest_txt":           "REPLACE_MANIFEST_TXT",
		"manifest_json":          "REPLACE_MANIFEST_JSON",
		"db_backup":              "REPLACE_DB_BACKUP",
		"files_backup":           "REPLACE_FILES_BACKUP",
		"requested_db_backup":    "REPLACE_REQUESTED_DB_BACKUP",
		"requested_files_backup": "REPLACE_REQUESTED_FILES_BACKUP",
	})
	replacements[filepath.Join(fixture.projectDir, "compose.yaml")] = "REPLACE_COMPOSE_FILE"
	replacements[fixture.sourceBackup.DBBackup] = "REPLACE_DB_BACKUP"
	replacements[fixture.sourceBackup.FilesBackup] = "REPLACE_FILES_BACKUP"
	normalizeWarningsPaths(obj, replacements)
	normalizeItemStringFields(obj, replacements, "summary", "details", "action")
	if message, ok := obj["message"].(string); ok {
		obj["message"] = replaceKnownPaths(message, replacements)
	}
	if errObj, ok := obj["error"].(map[string]any); ok {
		if message, ok := errObj["message"].(string); ok {
			errObj["message"] = replaceKnownPaths(message, replacements)
		}
	}
	return marshalCLIJSON(t, obj)
}

func captureMigrateAcceptanceReferenceDiskSnapshot(t *testing.T, fixture migrateCommandFixture, outcome execOutcome, beforeStorage, beforeSourceBackupRoot, beforeTargetBackupRoot []string) []byte {
	t.Helper()

	targetBackupRoot := filepath.Join(fixture.projectDir, "backups", "prod")
	snapshot := map[string]any{
		"process_exit_code":         outcome.ExitCode,
		"before_storage":            beforeStorage,
		"after_storage":             collectRelativeTreeEntries(t, fixture.storageDir),
		"storage_modes_after":       collectRelativeModes(t, fixture.storageDir),
		"before_source_backup_root": beforeSourceBackupRoot,
		"after_source_backup_root":  collectRelativeTreeEntries(t, fixture.sourceRoot),
		"before_target_backup_root": beforeTargetBackupRoot,
		"after_target_backup_root":  collectRelativeTreeEntries(t, targetBackupRoot),
	}

	artifacts := migrateReferenceArtifacts(t, outcome)
	if path := strings.TrimSpace(stringValueFromJSON(artifacts["manifest_json"])); fileExists(path) {
		snapshot["manifest_json"] = mustReadJSONFile(t, path)
	}

	raw, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func captureMigrateAcceptanceReferenceRuntimeSnapshot(t *testing.T, fixture migrateCommandFixture, outcome execOutcome, beforeRunning []string) []byte {
	t.Helper()

	snapshot := map[string]any{
		"process_exit_code":       outcome.ExitCode,
		"running_services_before": beforeRunning,
		"running_services_after":  readRunningServicesSnapshot(t, fixture.docker.stateDir),
	}
	if _, err := os.Stat(fixture.docker.logPath); err == nil {
		snapshot["docker_log"] = normalizeMigrateAcceptanceReferenceDockerLog(t, fixture, outcome)
	}

	raw, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func normalizeMigrateAcceptanceReferenceDockerLog(t *testing.T, fixture migrateCommandFixture, outcome execOutcome) []string {
	t.Helper()

	replacements := map[string]string{
		fixture.projectDir: "REPLACE_PROJECT_DIR",
		filepath.Join(fixture.projectDir, "compose.yaml"): "REPLACE_COMPOSE_FILE",
		filepath.Join(fixture.projectDir, ".env.dev"):     "REPLACE_SOURCE_ENV_FILE",
		filepath.Join(fixture.projectDir, ".env.prod"):    "REPLACE_TARGET_ENV_FILE",
		fixture.sourceRoot: "REPLACE_SOURCE_BACKUP_ROOT",
		filepath.Join(fixture.projectDir, "backups", "prod"): "REPLACE_TARGET_BACKUP_ROOT",
		fixture.storageDir:                "REPLACE_STORAGE_DIR",
		filepath.Dir(fixture.storageDir):  "REPLACE_STORAGE_PARENT",
		fixture.sourceBackup.ManifestJSON: "REPLACE_MANIFEST_JSON",
		fixture.sourceBackup.DBBackup:     "REPLACE_DB_BACKUP",
		fixture.sourceBackup.FilesBackup:  "REPLACE_FILES_BACKUP",
		os.TempDir():                      "REPLACE_HELPER_TMP_DIR",
	}

	raw := fixture.docker.ReadLog(t)
	lines := strings.Split(strings.ReplaceAll(raw, "\r", ""), "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		line = replaceKnownPaths(line, replacements)
		out = append(out, line)
	}

	obj := parseCLIJSONBytes(t, []byte(outcome.Stdout))
	if errObj, ok := obj["error"].(map[string]any); ok {
		if message, ok := errObj["message"].(string); ok && strings.Contains(message, "REPLACE_") {
			_ = message
		}
	}
	return out
}

func migrateReferenceArtifacts(t *testing.T, outcome execOutcome) map[string]any {
	t.Helper()

	obj := parseCLIJSONBytes(t, []byte(outcome.Stdout))
	artifacts, _ := obj["artifacts"].(map[string]any)
	return artifacts
}

func assertOrWriteMigrateAcceptanceReferenceGoldenJSON(t *testing.T, got []byte, relPath string) {
	t.Helper()

	path := acceptanceMigrateGoldenPath(relPath)
	gotNorm := normalizeMigrateAcceptanceReferenceJSONBytes(t, got)

	if os.Getenv(updateMigrateAcceptanceReferenceEnv) == "1" {
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

func normalizeMigrateAcceptanceReferenceJSONBytes(t *testing.T, raw []byte) []byte {
	t.Helper()

	var obj any
	if err := json.Unmarshal(raw, &obj); err != nil {
		t.Fatalf("invalid json: %v\n%s", err, string(raw))
	}
	out, err := json.MarshalIndent(obj, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func acceptanceMigrateGoldenPath(relPath string) string {
	return filepath.Join("..", "..", "acceptance", "v2", "migrate", relPath)
}
