package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const updateBackupVerifyCLIAcceptanceEnv = "UPDATE_ACCEPTANCE_BACKUP_VERIFY_CLI"

func TestAcceptance_BackupVerifyCLI_V2_JSONAndDisk(t *testing.T) {
	cases := []struct {
		id    string
		setup func(*testing.T, string) ([]string, string)
	}{
		{
			id: "BKV-001",
			setup: func(t *testing.T, root string) ([]string, string) {
				set := writeBackupSet(t, root, "espocrm-prod", "2026-04-07_01-00-00", "prod", nil)
				writeBackupVerifyReferenceManifest(t, set, sha256OfFile(t, set.DBBackup), sha256OfFile(t, set.FilesBackup))
				return []string{"--manifest", set.ManifestJSON}, root
			},
		},
		{
			id: "BKV-002",
			setup: func(t *testing.T, root string) ([]string, string) {
				set := writeBackupSet(t, root, "espocrm-prod", "2026-04-07_01-00-00", "prod", nil)
				writeBackupVerifyReferenceManifest(t, set, sha256OfFile(t, set.DBBackup), sha256OfFile(t, set.FilesBackup))
				writeIncompleteManifestForRootSelection(t, root, "espocrm-prod", "2026-04-07_02-00-00", "prod")
				return []string{"--backup-root", root}, root
			},
		},
		{
			id: "BKV-201",
			setup: func(t *testing.T, root string) ([]string, string) {
				return nil, root
			},
		},
		{
			id: "BKV-202",
			setup: func(t *testing.T, root string) ([]string, string) {
				set := writeBackupSet(t, root, "espocrm-prod", "2026-04-07_01-00-00", "prod", nil)
				return []string{"--manifest", set.ManifestJSON, "--backup-root", root}, root
			},
		},
		{
			id: "BKV-301",
			setup: func(t *testing.T, root string) ([]string, string) {
				manifestPath := filepath.Join(root, "manifests", "espocrm-prod_2026-04-07_01-00-00.manifest.json")
				mustMkdirAll(t, filepath.Dir(manifestPath))
				mustWriteFile(t, manifestPath, "{")
				return []string{"--manifest", manifestPath}, root
			},
		},
		{
			id: "BKV-302",
			setup: func(t *testing.T, root string) ([]string, string) {
				set := writeBackupSet(t, root, "espocrm-prod", "2026-04-07_01-00-00", "prod", nil)
				writeBackupVerifyReferenceManifest(t, set, strings.Repeat("0", 64), sha256OfFile(t, set.FilesBackup))
				return []string{"--manifest", set.ManifestJSON}, root
			},
		},
		{
			id: "BKV-303",
			setup: func(t *testing.T, root string) ([]string, string) {
				set := writeBackupSet(t, root, "espocrm-prod", "2026-04-07_01-00-00", "prod", nil)
				writeGzipFile(t, set.FilesBackup, []byte("not a tar stream"))
				writeBackupVerifyReferenceManifest(t, set, sha256OfFile(t, set.DBBackup), sha256OfFile(t, set.FilesBackup))
				return []string{"--manifest", set.ManifestJSON}, root
			},
		},
		{
			id: "BKV-304",
			setup: func(t *testing.T, root string) ([]string, string) {
				set := writeBackupSet(t, root, "espocrm-prod", "2026-04-07_01-00-00", "prod", nil)
				if err := os.Remove(set.DBBackup); err != nil {
					t.Fatal(err)
				}
				return []string{"--manifest", set.ManifestJSON}, root
			},
		},
		{
			id: "BKV-305",
			setup: func(t *testing.T, root string) ([]string, string) {
				manifestPath := writeBackupVerifyPartialManifest(t, root, "espocrm-prod", "2026-04-07_01-00-00")
				return []string{"--manifest", manifestPath}, root
			},
		},
		{
			id: "BKV-401",
			setup: func(t *testing.T, root string) ([]string, string) {
				writeBackupVerifyPartialManifest(t, root, "espocrm-prod", "2026-04-07_01-00-00")
				return []string{"--backup-root", root}, root
			},
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.id, func(t *testing.T) {
			tmp := t.TempDir()
			root := filepath.Join(tmp, "backups")
			journalDir := filepath.Join(tmp, "journal")
			extraArgs, backupRoot := tc.setup(t, root)
			before := collectRelativeTreeEntries(t, backupRoot)

			args := []string{"--journal-dir", journalDir, "--json", "backup", "verify"}
			args = append(args, extraArgs...)
			outcome := executeCLIWithOptions([]testAppOption{
				withFixedTestRuntime(time.Date(2026, 4, 15, 12, 0, 0, 0, time.UTC), "op-"+strings.ToLower(tc.id)),
			}, args...)

			normalizedJSON := normalizeBackupVerifyReferenceJSON(t, outcome)
			assertOrWriteBackupVerifyReferenceGoldenJSON(t, normalizedJSON, filepath.Join("golden", "json", "v2_"+tc.id+".json"))

			snapshot := marshalBackupVerifyReferenceJSON(t, map[string]any{
				"process_exit_code": outcome.ExitCode,
				"before_entries":    before,
				"after_entries":     collectRelativeTreeEntries(t, backupRoot),
			})
			assertOrWriteBackupVerifyReferenceGoldenJSON(t, snapshot, filepath.Join("golden", "disk", "v2_"+tc.id+".json"))
		})
	}
}

func writeBackupVerifyReferenceManifest(t *testing.T, set backupSetFixture, dbChecksum, filesChecksum string) {
	t.Helper()

	writeJSON(t, set.ManifestJSON, map[string]any{
		"version":              1,
		"scope":                "prod",
		"created_at":           "2026-04-07T01:00:00Z",
		"db_backup_created":    true,
		"files_backup_created": true,
		"artifacts": map[string]any{
			"db_backup":    filepath.Base(set.DBBackup),
			"files_backup": filepath.Base(set.FilesBackup),
		},
		"checksums": map[string]any{
			"db_backup":    dbChecksum,
			"files_backup": filesChecksum,
		},
	})
}

func writeBackupVerifyPartialManifest(t *testing.T, root, prefix, stamp string) string {
	t.Helper()

	manifestPath := filepath.Join(root, "manifests", prefix+"_"+stamp+".manifest.json")
	mustMkdirAll(t, filepath.Dir(manifestPath))
	writeJSON(t, manifestPath, map[string]any{
		"version":              1,
		"scope":                "prod",
		"created_at":           "2026-04-07T01:00:00Z",
		"db_backup_created":    true,
		"files_backup_created": false,
		"artifacts": map[string]any{
			"db_backup":    prefix + "_" + stamp + ".sql.gz",
			"files_backup": "",
		},
		"checksums": map[string]any{
			"db_backup":    strings.Repeat("a", 64),
			"files_backup": "",
		},
	})
	return manifestPath
}

func normalizeBackupVerifyReferenceJSON(t *testing.T, outcome execOutcome) []byte {
	t.Helper()

	obj := parseCLIJSONBytes(t, []byte(outcome.Stdout))
	obj["process_exit_code"] = outcome.ExitCode

	replacements := normalizeArtifactPlaceholders(obj, map[string]string{
		"backup_root":    "REPLACE_BACKUP_ROOT",
		"manifest":       "REPLACE_MANIFEST",
		"db_backup":      "REPLACE_DB_BACKUP",
		"db_checksum":    "REPLACE_DB_CHECKSUM",
		"files_backup":   "REPLACE_FILES_BACKUP",
		"files_checksum": "REPLACE_FILES_CHECKSUM",
	})
	normalizeWarningsPaths(obj, replacements)
	normalizeItemStringFields(obj, replacements, "summary", "details", "action")

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
	delete(obj, "message")
	delete(obj, "timing")
	delete(obj, "warnings")
	return marshalBackupVerifyReferenceJSON(t, obj)
}

func marshalBackupVerifyReferenceJSON(t *testing.T, value any) []byte {
	t.Helper()

	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func assertOrWriteBackupVerifyReferenceGoldenJSON(t *testing.T, got []byte, relPath string) {
	t.Helper()

	path := filepath.Join("..", "..", "acceptance", "v2", "backup_verify", relPath)
	gotNorm := normalizeBackupVerifyReferenceJSONBytes(t, got)
	if os.Getenv(updateBackupVerifyCLIAcceptanceEnv) == "1" {
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
	wantNorm := normalizeBackupVerifyReferenceJSONBytes(t, want)
	if string(gotNorm) != string(wantNorm) {
		t.Fatalf("golden mismatch for %s\nGOT:\n%s\n\nWANT:\n%s", relPath, gotNorm, wantNorm)
	}
}

func normalizeBackupVerifyReferenceJSONBytes(t *testing.T, raw []byte) []byte {
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
