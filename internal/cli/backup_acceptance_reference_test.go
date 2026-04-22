package cli

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

const updateBackupAcceptanceReferenceEnv = "UPDATE_ACCEPTANCE_BACKUP_REFERENCE"

func TestAcceptanceReference_BackupCLI_JSONAndDisk(t *testing.T) {
	cases := []struct {
		id        string
		extraArgs []string
		setup     func(*testing.T, *backupCommandFixture)
	}{
		{
			id: "BKP-001",
			setup: func(t *testing.T, fixture *backupCommandFixture) {
				fixture.docker.SetRunningServices(t, "db", "espocrm", "espocrm-daemon", "espocrm-websocket")
				fixture.docker.EnableLog(t)
			},
		},
		{
			id: "BKP-002",
			setup: func(t *testing.T, fixture *backupCommandFixture) {
				fixture.docker.SetRunningServices(t, "db")
				fixture.docker.EnableLog(t)
			},
		},
		{
			id:        "BKP-003",
			extraArgs: []string{"--no-stop"},
			setup: func(t *testing.T, fixture *backupCommandFixture) {
				fixture.docker.SetRunningServices(t, "db", "espocrm", "espocrm-daemon", "espocrm-websocket")
				fixture.docker.EnableLog(t)
			},
		},
		{
			id:        "BKP-101",
			extraArgs: []string{"--skip-db", "--no-stop"},
			setup: func(t *testing.T, fixture *backupCommandFixture) {
				fixture.docker.SetFailOnAnyCall(t)
			},
		},
		{
			id:        "BKP-102",
			extraArgs: []string{"--skip-files", "--no-stop"},
			setup: func(t *testing.T, fixture *backupCommandFixture) {
				fixture.docker.SetRunningServices(t, "db")
				fixture.docker.EnableLog(t)
			},
		},
		{
			id:        "BKP-204",
			extraArgs: []string{},
			setup: func(t *testing.T, fixture *backupCommandFixture) {
				fixture.docker.SetRunningServices(t, "db", "espocrm", "espocrm-daemon", "espocrm-websocket")
				fixture.docker.SetFailOnMatch(t, "ps --status running --services")
				fixture.docker.EnableLog(t)
			},
		},
		{
			id:        "BKP-205",
			extraArgs: []string{},
			setup: func(t *testing.T, fixture *backupCommandFixture) {
				fixture.docker.SetRunningServices(t, "espocrm", "espocrm-daemon", "espocrm-websocket")
				fixture.docker.EnableLog(t)
			},
		},
		{
			id:        "BKP-206",
			extraArgs: []string{"--skip-db", "--no-stop"},
			setup: func(t *testing.T, fixture *backupCommandFixture) {
				fixture.docker.SetFailOnMatch(t, "run --pull=never --rm --entrypoint tar")
				fixture.docker.EnableLog(t)
				prependFailingTar(t, "mock tar failed")
			},
		},
		{
			id:        "BKP-207",
			extraArgs: []string{"--no-stop"},
			setup: func(t *testing.T, fixture *backupCommandFixture) {
				fixture.docker.SetRunningServices(t, "db")
				fixture.docker.EnableLog(t)
				manifestDir := filepath.Join(fixture.projectDir, "backups", "dev", "manifests")
				mustMkdirAll(t, manifestDir)
				if err := os.Chmod(manifestDir, 0o555); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			id:        "BKP-208",
			extraArgs: []string{"--no-stop"},
			setup: func(t *testing.T, fixture *backupCommandFixture) {
				fixture.docker.SetRunningServices(t, "db")
				fixture.docker.EnableLog(t)
				backupRoot := filepath.Join(fixture.projectDir, "backups", "dev")
				set := writeBackupSet(t, backupRoot, "espocrm-test-dev", "2026-04-01_00-00-00", "dev", nil)
				if err := os.Remove(set.DBBackup); err != nil {
					t.Fatal(err)
				}
				if err := os.Mkdir(set.DBBackup, 0o755); err != nil {
					t.Fatal(err)
				}
				mustWriteFile(t, filepath.Join(set.DBBackup, "blocker.txt"), "keep\n")
			},
		},
		{
			id:        "BKP-209",
			extraArgs: []string{},
			setup: func(t *testing.T, fixture *backupCommandFixture) {
				fixture.docker.SetRunningServices(t, "db", "espocrm", "espocrm-daemon", "espocrm-websocket")
				fixture.docker.SetFailOnMatch(t, "up -d espocrm espocrm-daemon espocrm-websocket")
				fixture.docker.EnableLog(t)
			},
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.id, func(t *testing.T) {
			fixture := prepareBackupCommandFixture(t)
			if tc.setup != nil {
				tc.setup(t, &fixture)
			}

			outcome := executeCLIWithOptions(
				[]testAppOption{
					withFixedTestRuntime(fixture.fixedNow, "op-"+strings.ToLower(tc.id)),
				},
				backupAcceptanceArgs(fixture, tc.extraArgs...)...,
			)

			normalizedJSON := normalizeBackupAcceptanceJSON(t, outcome)
			assertOrWriteBackupAcceptanceGoldenJSON(t, normalizedJSON, filepath.Join("golden", "json", "v2_"+tc.id+".json"))

			snapshot := captureBackupAcceptanceDiskSnapshot(t, fixture, outcome)
			assertOrWriteBackupAcceptanceGoldenJSON(t, snapshot, filepath.Join("golden", "disk", "v2_"+tc.id+".json"))
		})
	}
}

func backupAcceptanceArgs(fixture backupCommandFixture, extraArgs ...string) []string {
	args := []string{
		"--journal-dir", fixture.journalDir,
		"--json",
		"backup",
		"--scope", "dev",
		"--project-dir", fixture.projectDir,
		"--compose-file", fixture.composeFile,
		"--env-file", fixture.envFile,
	}
	args = append(args, extraArgs...)
	return args
}

func prependFailingTar(t *testing.T, message string) {
	t.Helper()

	binDir := t.TempDir()
	tarPath := filepath.Join(binDir, "tar")
	script := fmt.Sprintf("#!/usr/bin/env bash\nset -Eeuo pipefail\necho %q >&2\nexit 2\n", message)
	if err := os.WriteFile(tarPath, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func normalizeBackupAcceptanceJSON(t *testing.T, outcome execOutcome) []byte {
	t.Helper()

	obj := parseCLIJSONBytes(t, []byte(outcome.Stdout))
	obj["process_exit_code"] = outcome.ExitCode

	replacements := normalizeArtifactPlaceholders(obj, map[string]string{
		"project_dir":    "REPLACE_PROJECT_DIR",
		"compose_file":   "REPLACE_COMPOSE_FILE",
		"env_file":       "REPLACE_ENV_FILE",
		"backup_root":    "REPLACE_BACKUP_ROOT",
		"manifest_txt":   "REPLACE_MANIFEST_TXT",
		"manifest_json":  "REPLACE_MANIFEST_JSON",
		"db_backup":      "REPLACE_DB_BACKUP",
		"files_backup":   "REPLACE_FILES_BACKUP",
		"db_checksum":    "REPLACE_DB_CHECKSUM",
		"files_checksum": "REPLACE_FILES_CHECKSUM",
	})
	normalizeWarningsPaths(obj, replacements)
	normalizeItemStringFields(obj, replacements, "summary", "details", "action")

	if warnings, ok := obj["warnings"].([]any); ok {
		semantics := make([]any, 0, len(warnings))
		for _, rawWarning := range warnings {
			warning, _ := rawWarning.(string)
			if strings.TrimSpace(warning) == "" {
				continue
			}
			semantics = append(semantics, backupWarningSemantic(warning))
		}
		if len(semantics) != 0 {
			obj["warning_semantics"] = semantics
		}
	}
	delete(obj, "warnings")
	delete(obj, "message")

	if errObj, ok := obj["error"].(map[string]any); ok {
		delete(errObj, "message")
	}

	if items, ok := obj["items"].([]any); ok {
		normalizedItems := make([]any, 0, len(items))
		for _, rawItem := range items {
			item, ok := rawItem.(map[string]any)
			if !ok {
				continue
			}
			normalizedItems = append(normalizedItems, map[string]any{
				"code":   item["code"],
				"status": item["status"],
			})
		}
		obj["items"] = normalizedItems
	}

	return marshalCLIJSON(t, obj)
}

func backupWarningSemantic(warning string) string {
	switch {
	case strings.Contains(warning, "Docker helper fallback"):
		return "docker_helper_fallback"
	case strings.Contains(warning, "runtime was returned to its prior state"):
		return "runtime_return_after_failure"
	default:
		return warning
	}
}

func captureBackupAcceptanceDiskSnapshot(t *testing.T, fixture backupCommandFixture, outcome execOutcome) []byte {
	t.Helper()

	obj := parseCLIJSONBytes(t, []byte(outcome.Stdout))
	artifacts := map[string]any{}
	if rawArtifacts, ok := obj["artifacts"].(map[string]any); ok {
		artifacts = rawArtifacts
	}
	backupRoot := filepath.Join(fixture.projectDir, "backups", "dev")
	if rawBackupRoot, ok := artifacts["backup_root"].(string); ok && strings.TrimSpace(rawBackupRoot) != "" {
		backupRoot = rawBackupRoot
	}

	snapshot := map[string]any{
		"process_exit_code":      outcome.ExitCode,
		"backup_root_entries":    collectRelativeTreeEntries(t, backupRoot),
		"backup_root_tmp_files":  collectRelativeTmpFiles(t, backupRoot),
		"running_services_after": readRunningServicesSnapshot(t, fixture.docker.stateDir),
	}

	if _, err := os.Stat(fixture.docker.logPath); err == nil {
		snapshot["docker_log"] = normalizeDockerLogLines(t, fixture, fixture.docker.ReadLog(t))
	}

	if path := strings.TrimSpace(stringValueFromJSON(artifacts["manifest_txt"])); fileExists(path) {
		snapshot["manifest_txt"] = mustReadTextFile(t, path)
	}
	if path := strings.TrimSpace(stringValueFromJSON(artifacts["manifest_json"])); fileExists(path) {
		snapshot["manifest_json"] = mustReadJSONFile(t, path)
	}
	if path := strings.TrimSpace(stringValueFromJSON(artifacts["db_backup"])); fileExists(path) {
		snapshot["db_backup_sql"] = mustReadGzipText(t, path)
	}
	if path := strings.TrimSpace(stringValueFromJSON(artifacts["files_backup"])); fileExists(path) {
		snapshot["files_archive_entries"] = mustReadTarEntries(t, path)
	}
	if path := strings.TrimSpace(stringValueFromJSON(artifacts["db_checksum"])); fileExists(path) {
		snapshot["db_checksum"] = mustReadTextFile(t, path)
	}
	if path := strings.TrimSpace(stringValueFromJSON(artifacts["files_checksum"])); fileExists(path) {
		snapshot["files_checksum"] = mustReadTextFile(t, path)
	}

	return marshalBackupAcceptanceSnapshot(t, snapshot)
}

func stringValueFromJSON(value any) string {
	text, _ := value.(string)
	return text
}

func fileExists(path string) bool {
	if strings.TrimSpace(path) == "" {
		return false
	}
	_, err := os.Stat(path)
	return err == nil
}

func collectRelativeTreeEntries(t *testing.T, root string) []string {
	t.Helper()

	entries := []string{}
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == root {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if entry.IsDir() {
			entries = append(entries, rel+"/")
			return nil
		}
		entries = append(entries, rel)
		return nil
	})
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(entries)
	return entries
}

func collectRelativeTmpFiles(t *testing.T, root string) []string {
	t.Helper()

	all := collectRelativeTreeEntries(t, root)
	tmp := make([]string, 0)
	for _, entry := range all {
		if strings.HasSuffix(entry, ".tmp") {
			tmp = append(tmp, entry)
		}
	}
	return tmp
}

func readRunningServicesSnapshot(t *testing.T, stateDir string) []string {
	t.Helper()

	path := filepath.Join(stateDir, "running-services")
	raw, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		t.Fatal(err)
	}

	lines := strings.Split(strings.ReplaceAll(string(raw), "\r", ""), "\n")
	services := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		services = append(services, line)
	}
	return services
}

func normalizeDockerLogLines(t *testing.T, fixture backupCommandFixture, raw string) []string {
	t.Helper()

	replacements := map[string]string{
		fixture.projectDir:  "REPLACE_PROJECT_DIR",
		fixture.composeFile: "REPLACE_COMPOSE_FILE",
		fixture.envFile:     "REPLACE_ENV_FILE",
		filepath.Join(fixture.projectDir, "backups", "dev"): "REPLACE_BACKUP_ROOT",
		fixture.storageDir:               "REPLACE_STORAGE_DIR",
		filepath.Dir(fixture.storageDir): "REPLACE_STORAGE_PARENT",
	}

	lines := strings.Split(strings.ReplaceAll(raw, "\r", ""), "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		out = append(out, replaceKnownPaths(line, replacements))
	}
	return out
}

func mustReadTextFile(t *testing.T, path string) string {
	t.Helper()

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func mustReadJSONFile(t *testing.T, path string) any {
	t.Helper()

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	var obj any
	if err := json.Unmarshal(raw, &obj); err != nil {
		t.Fatalf("invalid json file %s: %v", path, err)
	}
	return obj
}

func mustReadGzipText(t *testing.T, path string) string {
	t.Helper()

	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer closeCLIArchiveResource(t, "gzip file", f)

	gz, err := gzip.NewReader(f)
	if err != nil {
		t.Fatal(err)
	}
	defer closeCLIArchiveResource(t, "gzip reader", gz)

	raw, err := io.ReadAll(gz)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func mustReadTarEntries(t *testing.T, path string) []string {
	t.Helper()

	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer closeCLIArchiveResource(t, "tar archive file", f)

	gz, err := gzip.NewReader(f)
	if err != nil {
		t.Fatal(err)
	}
	defer closeCLIArchiveResource(t, "tar archive gzip reader", gz)

	tr := tar.NewReader(gz)
	entries := []string{}
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		entries = append(entries, hdr.Name)
		if _, err := io.Copy(io.Discard, tr); err != nil {
			t.Fatal(err)
		}
	}
	sort.Strings(entries)
	return entries
}

func marshalBackupAcceptanceSnapshot(t *testing.T, snapshot map[string]any) []byte {
	t.Helper()

	raw, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func assertOrWriteBackupAcceptanceGoldenJSON(t *testing.T, got []byte, relPath string) {
	t.Helper()

	path := acceptanceBackupGoldenPath(relPath)

	var gotObj any
	if err := json.Unmarshal(got, &gotObj); err != nil {
		t.Fatalf("invalid json for %s: %v\n%s", relPath, err, string(got))
	}
	gotNorm, err := json.MarshalIndent(gotObj, "", "  ")
	if err != nil {
		t.Fatal(err)
	}

	if os.Getenv(updateBackupAcceptanceReferenceEnv) == "1" {
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

	var wantObj any
	if err := json.Unmarshal(want, &wantObj); err != nil {
		t.Fatalf("invalid golden json %s: %v", path, err)
	}
	wantNorm, err := json.MarshalIndent(wantObj, "", "  ")
	if err != nil {
		t.Fatal(err)
	}

	if string(gotNorm) != string(wantNorm) {
		t.Fatalf("golden mismatch for %s\nGOT:\n%s\n\nWANT:\n%s", relPath, gotNorm, wantNorm)
	}
}

func acceptanceBackupGoldenPath(relPath string) string {
	return filepath.Join("..", "..", "acceptance", "v2", "backup", relPath)
}
