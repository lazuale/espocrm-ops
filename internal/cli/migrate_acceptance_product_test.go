package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const updateMigrateProductJSONEnv = "UPDATE_ACCEPTANCE_MIGRATE_PRODUCT_JSON"

func TestAcceptance_MigrateCLI_ProductJSON(t *testing.T) {
	cases := []struct {
		id           string
		wantExitCode int
		setup        func(*testing.T, *migrateCommandFixture) []string
	}{
		{
			id:           "MIG-001",
			wantExitCode: 0,
			setup: func(t *testing.T, fixture *migrateCommandFixture) []string {
				migrateCLISetHealthyRuntime(t, fixture)
				return nil
			},
		},
		{
			id:           "MIG-205",
			wantExitCode: 4,
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
			id:           "MIG-503",
			wantExitCode: 6,
			setup: func(t *testing.T, fixture *migrateCommandFixture) []string {
				migrateCLISetHealthyRuntime(t, fixture)
				mustMkdirAll(t, filepath.Join(filepath.Dir(fixture.storageDir), ".espo.new"))
				return nil
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
					withFixedTestRuntime(fixture.fixedNow, "op-product-"+strings.ToLower(tc.id)),
				},
				migrateCLIAcceptanceArgs(fixture, true, true, extraArgs...)...,
			)
			if outcome.ExitCode != tc.wantExitCode {
				t.Fatalf("expected exit code %d, got %d stdout=%s stderr=%s", tc.wantExitCode, outcome.ExitCode, outcome.Stdout, outcome.Stderr)
			}

			assertOrWriteMigrateProductJSONGolden(t, normalizeMigrateCLIProductJSON(t, fixture, outcome), "v2_"+tc.id+".json")
		})
	}
}

func normalizeMigrateCLIProductJSON(t *testing.T, fixture migrateCommandFixture, outcome execOutcome) []byte {
	t.Helper()

	obj := parseCLIJSONBytes(t, []byte(outcome.Stdout))
	targetBackupRoot := filepath.Join(fixture.projectDir, "backups", "prod")
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
	for path, placeholder := range map[string]string{
		fixture.projectDir: "REPLACE_PROJECT_DIR",
		filepath.Join(fixture.projectDir, "compose.yaml"): "REPLACE_COMPOSE_FILE",
		filepath.Join(fixture.projectDir, ".env.dev"):     "REPLACE_SOURCE_ENV_FILE",
		filepath.Join(fixture.projectDir, ".env.prod"):    "REPLACE_TARGET_ENV_FILE",
		fixture.sourceRoot:                "REPLACE_SOURCE_BACKUP_ROOT",
		targetBackupRoot:                  "REPLACE_TARGET_BACKUP_ROOT",
		fixture.storageDir:                "REPLACE_STORAGE_DIR",
		fixture.sourceBackup.ManifestTXT:  "REPLACE_MANIFEST_TXT",
		fixture.sourceBackup.ManifestJSON: "REPLACE_MANIFEST_JSON",
		fixture.sourceBackup.DBBackup:     "REPLACE_DB_BACKUP",
		fixture.sourceBackup.FilesBackup:  "REPLACE_FILES_BACKUP",
	} {
		if strings.TrimSpace(path) != "" {
			replacements[path] = placeholder
		}
	}

	if message, ok := obj["message"].(string); ok {
		obj["message"] = replaceKnownPaths(message, replacements)
	}
	if errObj, ok := obj["error"].(map[string]any); ok {
		if message, ok := errObj["message"].(string); ok {
			errObj["message"] = replaceKnownPaths(message, replacements)
		}
	}
	normalizeWarningsPaths(obj, replacements)
	normalizeItemStringFields(obj, replacements, "summary", "details", "action")
	return marshalCLIJSON(t, obj)
}

func assertOrWriteMigrateProductJSONGolden(t *testing.T, got []byte, name string) {
	t.Helper()

	path := migrateProductJSONGoldenPath(name)
	gotNorm := normalizeMigrateAcceptanceReferenceJSONBytes(t, got)

	if os.Getenv(updateMigrateProductJSONEnv) == "1" {
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
		t.Fatalf("golden mismatch for %s\nGOT:\n%s\n\nWANT:\n%s", name, gotNorm, wantNorm)
	}
}

func migrateProductJSONGoldenPath(name string) string {
	return filepath.Join("..", "..", "acceptance", "v2", "migrate", "cutover", "product_json", name)
}
