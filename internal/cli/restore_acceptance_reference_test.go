package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	restoreusecase "github.com/lazuale/espocrm-ops/internal/app/restore"
)

const updateRestoreAcceptanceReferenceEnv = "UPDATE_ACCEPTANCE_RESTORE_REFERENCE"

func TestAcceptanceReference_RestoreV1_JSONDiskAndRuntime(t *testing.T) {
	cases := []struct {
		id    string
		setup func(*testing.T, *restoreCommandFixture) restoreusecase.ExecuteRequest
	}{
		{
			id: "RST-205",
			setup: func(t *testing.T, fixture *restoreCommandFixture) restoreusecase.ExecuteRequest {
				fixture.docker.SetRunningServices(t, "db", "espocrm", "espocrm-daemon", "espocrm-websocket")
				fixture.docker.EnableLog(t)
				req := restoreLegacyReferenceRequest(*fixture)
				req.ManifestPath = writeRestoreReferencePartialManifest(t, *fixture)
				req.SkipFiles = true
				req.NoSnapshot = true
				return req
			},
		},
		{
			id: "RST-303",
			setup: func(t *testing.T, fixture *restoreCommandFixture) restoreusecase.ExecuteRequest {
				fixture.docker.SetRunningServices(t, "db", "espocrm", "espocrm-daemon", "espocrm-websocket")
				fixture.docker.SetFailOnMatch(t, "mariadb-dump -u espocrm espocrm --single-transaction --quick --routines --triggers --events")
				fixture.docker.EnableLog(t)
				req := restoreLegacyReferenceRequest(*fixture)
				req.ManifestPath = fixture.backupSet.ManifestJSON
				return req
			},
		},
		{
			id: "RST-402",
			setup: func(t *testing.T, fixture *restoreCommandFixture) restoreusecase.ExecuteRequest {
				fixture.docker.SetRunningServices(t, "db", "espocrm", "espocrm-daemon", "espocrm-websocket")
				fixture.docker.EnableLog(t)
				req := restoreLegacyReferenceRequest(*fixture)
				req.ManifestPath = fixture.backupSet.ManifestJSON
				req.NoSnapshot = true
				req.NoStop = true
				return req
			},
		},
		{
			id: "RST-403",
			setup: func(t *testing.T, fixture *restoreCommandFixture) restoreusecase.ExecuteRequest {
				fixture.docker.SetRunningServices(t, "db", "espocrm", "espocrm-daemon", "espocrm-websocket")
				fixture.docker.EnableLog(t)
				req := restoreLegacyReferenceRequest(*fixture)
				req.ManifestPath = fixture.backupSet.ManifestJSON
				req.NoSnapshot = true
				req.NoStart = true
				return req
			},
		},
		{
			id: "RST-404",
			setup: func(t *testing.T, fixture *restoreCommandFixture) restoreusecase.ExecuteRequest {
				fixture.docker.SetRunningServices(t, "db", "espocrm", "espocrm-daemon", "espocrm-websocket")
				fixture.docker.SetFailOnMatch(t, "up -d espocrm espocrm-daemon espocrm-websocket")
				fixture.docker.EnableLog(t)
				req := restoreLegacyReferenceRequest(*fixture)
				req.ManifestPath = fixture.backupSet.ManifestJSON
				req.NoSnapshot = true
				return req
			},
		},
		{
			id: "RST-503",
			setup: func(t *testing.T, fixture *restoreCommandFixture) restoreusecase.ExecuteRequest {
				fixture.docker.SetRunningServices(t, "db", "espocrm", "espocrm-daemon", "espocrm-websocket")
				fixture.docker.SetFailOnMatch(t, ":/espo-storage")
				fixture.docker.EnableLog(t)
				req := restoreLegacyReferenceRequest(*fixture)
				req.ManifestPath = fixture.backupSet.ManifestJSON
				req.NoSnapshot = true
				return req
			},
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.id, func(t *testing.T) {
			lockOpt := isolateRecoveryLocks(t)
			fixture := prepareRestoreCommandFixture(t, "prod", map[string]string{
				"espo/data/nested/file.txt":      "hello",
				"espo/custom/modules/module.txt": "custom",
				"espo/client/custom/app.js":      "client",
				"espo/upload/blob.txt":           "upload",
			})

			request := tc.setup(t, &fixture)
			beforeStorage := collectRelativeTreeEntries(t, fixture.storageDir)
			beforeBackupRoot := collectRelativeTreeEntries(t, fixture.backupRoot)
			beforeRunning := readRunningServicesSnapshot(t, fixture.docker.stateDir)

			outcome := executeRestoreLegacyReferenceCLI(
				[]testAppOption{
					lockOpt,
					withFixedTestRuntime(fixture.fixedNow, "op-"+strings.ToLower(tc.id)),
				},
				fixture.journalDir,
				request,
			)

			normalizedJSON := normalizeRestoreAcceptanceReferenceJSON(t, outcome)
			assertOrWriteRestoreAcceptanceReferenceGoldenJSON(t, normalizedJSON, filepath.Join("golden", "json", "v1_"+tc.id+".json"))

			disk := captureRestoreAcceptanceReferenceDiskSnapshot(t, fixture, outcome, beforeStorage, beforeBackupRoot)
			assertOrWriteRestoreAcceptanceReferenceGoldenJSON(t, disk, filepath.Join("golden", "disk", "v1_"+tc.id+".json"))

			runtime := captureRestoreAcceptanceReferenceRuntimeSnapshot(t, fixture, outcome, beforeRunning)
			assertOrWriteRestoreAcceptanceReferenceGoldenJSON(t, runtime, filepath.Join("golden", "runtime", "v1_"+tc.id+".json"))
		})
	}
}

func restoreLegacyReferenceRequest(fixture restoreCommandFixture) restoreusecase.ExecuteRequest {
	return restoreusecase.ExecuteRequest{
		Scope:       "prod",
		ProjectDir:  fixture.projectDir,
		ComposeFile: filepath.Join(fixture.projectDir, "compose.yaml"),
	}
}

func restoreAcceptanceReferenceArgs(fixture restoreCommandFixture, extraArgs ...string) []string {
	args := []string{
		"--journal-dir", fixture.journalDir,
		"--json",
		"restore",
		"--scope", "prod",
		"--project-dir", fixture.projectDir,
		"--force",
		"--confirm-prod", "prod",
	}
	args = append(args, extraArgs...)
	return args
}

func writeRestoreReferencePartialManifest(t *testing.T, fixture restoreCommandFixture) string {
	t.Helper()

	manifestPath := filepath.Join(fixture.backupRoot, "manifests", "espocrm-prod_2026-04-19_08-00-00.partial.manifest.json")
	writeJSON(t, manifestPath, map[string]any{
		"version":              1,
		"scope":                "prod",
		"created_at":           "2026-04-18T10:00:00Z",
		"db_backup_created":    true,
		"files_backup_created": false,
		"artifacts": map[string]any{
			"db_backup":    filepath.Base(fixture.backupSet.DBBackup),
			"files_backup": "",
		},
		"checksums": map[string]any{
			"db_backup":    sha256OfFile(t, fixture.backupSet.DBBackup),
			"files_backup": "",
		},
	})
	return manifestPath
}

func normalizeRestoreAcceptanceReferenceJSON(t *testing.T, outcome execOutcome) []byte {
	t.Helper()

	obj := parseCLIJSONBytes(t, []byte(outcome.Stdout))
	obj["process_exit_code"] = outcome.ExitCode

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

func captureRestoreAcceptanceReferenceDiskSnapshot(t *testing.T, fixture restoreCommandFixture, outcome execOutcome, beforeStorage, beforeBackupRoot []string) []byte {
	t.Helper()

	artifacts := restoreReferenceArtifacts(t, outcome)
	snapshot := map[string]any{
		"process_exit_code":   outcome.ExitCode,
		"before_storage":      beforeStorage,
		"after_storage":       collectRelativeTreeEntries(t, fixture.storageDir),
		"before_backup_root":  beforeBackupRoot,
		"after_backup_root":   collectRelativeTreeEntries(t, fixture.backupRoot),
		"storage_modes_after": collectRelativeModes(t, fixture.storageDir),
	}

	if path := strings.TrimSpace(stringValueFromJSON(artifacts["manifest_json"])); fileExists(path) {
		snapshot["manifest_json"] = normalizeRestoreReferenceManifestJSON(mustReadJSONFile(t, path), false)
	}
	if path := strings.TrimSpace(stringValueFromJSON(artifacts["snapshot_manifest_json"])); fileExists(path) {
		snapshot["snapshot_manifest_json"] = normalizeRestoreReferenceManifestJSON(mustReadJSONFile(t, path), true)
	}
	if path := strings.TrimSpace(stringValueFromJSON(artifacts["snapshot_db_backup"])); fileExists(path) {
		snapshot["snapshot_db_backup_sql"] = mustReadGzipText(t, path)
	}
	if path := strings.TrimSpace(stringValueFromJSON(artifacts["snapshot_files_backup"])); fileExists(path) {
		snapshot["snapshot_files_archive_entries"] = mustReadTarEntries(t, path)
	}
	if path := strings.TrimSpace(stringValueFromJSON(artifacts["snapshot_db_checksum"])); fileExists(path) {
		snapshot["snapshot_db_checksum"] = mustReadTextFile(t, path)
	}
	if path := strings.TrimSpace(stringValueFromJSON(artifacts["snapshot_files_checksum"])); fileExists(path) {
		snapshot["snapshot_files_checksum"] = mustReadTextFile(t, path)
	}

	return marshalRestoreAcceptanceReferenceSnapshot(t, snapshot)
}

func captureRestoreAcceptanceReferenceRuntimeSnapshot(t *testing.T, fixture restoreCommandFixture, outcome execOutcome, beforeRunning []string) []byte {
	t.Helper()

	snapshot := map[string]any{
		"process_exit_code":       outcome.ExitCode,
		"running_services_before": beforeRunning,
		"running_services_after":  readRunningServicesSnapshot(t, fixture.docker.stateDir),
	}
	if _, err := os.Stat(fixture.docker.logPath); err == nil {
		snapshot["docker_log"] = normalizeRestoreAcceptanceReferenceDockerLog(t, fixture, outcome)
	}

	return marshalRestoreAcceptanceReferenceSnapshot(t, snapshot)
}

func normalizeRestoreAcceptanceReferenceDockerLog(t *testing.T, fixture restoreCommandFixture, outcome execOutcome) []string {
	t.Helper()

	artifacts := restoreReferenceArtifacts(t, outcome)
	replacements := map[string]string{
		fixture.projectDir: "REPLACE_PROJECT_DIR",
		filepath.Join(fixture.projectDir, "compose.yaml"): "REPLACE_COMPOSE_FILE",
		filepath.Join(fixture.projectDir, ".env.prod"):    "REPLACE_ENV_FILE",
		fixture.backupRoot:               "REPLACE_BACKUP_ROOT",
		fixture.storageDir:               "REPLACE_STORAGE_DIR",
		filepath.Dir(fixture.storageDir): "REPLACE_STORAGE_PARENT",
		os.TempDir():                     "REPLACE_HELPER_TMP_DIR",
	}
	for _, key := range []string{
		"manifest_json",
		"db_backup",
		"files_backup",
		"snapshot_manifest_json",
		"snapshot_db_backup",
		"snapshot_files_backup",
	} {
		value := strings.TrimSpace(stringValueFromJSON(artifacts[key]))
		if value == "" {
			continue
		}
		replacements[value] = fmt.Sprintf("REPLACE_%s", strings.ToUpper(key))
	}

	lines := strings.Split(strings.ReplaceAll(fixture.docker.ReadLog(t), "\r", ""), "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		line = replaceKnownPaths(line, replacements)
		line = backupHelperTmpNamePattern.ReplaceAllString(line, "espops-backup-v2-files-helper-REPLACE.tar.gz")
		out = append(out, line)
	}
	return out
}

func restoreReferenceArtifacts(t *testing.T, outcome execOutcome) map[string]any {
	t.Helper()

	obj := parseCLIJSONBytes(t, []byte(outcome.Stdout))
	artifacts, _ := obj["artifacts"].(map[string]any)
	if artifacts == nil {
		return map[string]any{}
	}
	return artifacts
}

func collectRelativeModes(t *testing.T, root string) map[string]string {
	t.Helper()

	entries := collectRelativeTreeEntries(t, root)
	modes := make(map[string]string, len(entries))
	for _, entry := range entries {
		path := filepath.Join(root, filepath.FromSlash(strings.TrimSuffix(entry, "/")))
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		modes[entry] = fmt.Sprintf("%03o", info.Mode().Perm())
	}
	return modes
}

func normalizeRestoreReferenceManifestJSON(raw any, snapshot bool) any {
	obj, ok := raw.(map[string]any)
	if !ok {
		return raw
	}

	checksums, ok := obj["checksums"].(map[string]any)
	if !ok {
		return obj
	}

	dbPlaceholder := "REPLACE_DB_CHECKSUM"
	filesPlaceholder := "REPLACE_FILES_CHECKSUM"
	if snapshot {
		dbPlaceholder = "REPLACE_SNAPSHOT_DB_CHECKSUM"
		filesPlaceholder = "REPLACE_SNAPSHOT_FILES_CHECKSUM"
	}

	if value, ok := checksums["db_backup"].(string); ok && strings.TrimSpace(value) != "" {
		checksums["db_backup"] = dbPlaceholder
	}
	if value, ok := checksums["files_backup"].(string); ok && strings.TrimSpace(value) != "" {
		checksums["files_backup"] = filesPlaceholder
	}

	return obj
}

func marshalRestoreAcceptanceReferenceSnapshot(t *testing.T, snapshot map[string]any) []byte {
	t.Helper()

	raw, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func assertOrWriteRestoreAcceptanceReferenceGoldenJSON(t *testing.T, got []byte, relPath string) {
	t.Helper()

	path := acceptanceRestoreGoldenPath(relPath)
	gotNorm := normalizeRestoreAcceptanceReferenceJSONBytes(t, got)

	if os.Getenv(updateRestoreAcceptanceReferenceEnv) == "1" {
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

func normalizeRestoreAcceptanceReferenceJSONBytes(t *testing.T, raw []byte) []byte {
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

func acceptanceRestoreGoldenPath(relPath string) string {
	return filepath.Join("..", "..", "acceptance", "v2", "restore", relPath)
}
