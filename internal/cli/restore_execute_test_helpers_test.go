package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

type restoreBackupSet struct {
	DBBackup     string
	FilesBackup  string
	ManifestTXT  string
	ManifestJSON string
}

type restoreCommandFixture struct {
	projectDir string
	journalDir string
	stateDir   string
	logPath    string
	storageDir string
	backupRoot string
	backupSet  restoreBackupSet
	fixedNow   time.Time
}

func prepareRestoreCommandFixture(t *testing.T, scope string, files map[string]string) restoreCommandFixture {
	t.Helper()

	tmp := t.TempDir()
	projectDir := filepath.Join(tmp, "project")
	journalDir := filepath.Join(tmp, "journal")
	stateDir := filepath.Join(tmp, "docker-state")
	logPath := filepath.Join(tmp, "docker.log")
	storageDir := filepath.Join(projectDir, "runtime", scope, "espo")
	backupRoot := filepath.Join(projectDir, "backups", scope)
	fixedNow := time.Date(2026, 4, 19, 9, 0, 0, 0, time.UTC)

	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(storageDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, "compose.yaml"), []byte("services: {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(storageDir, "before.txt"), []byte("before\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	writeDoctorEnvFile(t, projectDir, scope, nil)

	return restoreCommandFixture{
		projectDir: projectDir,
		journalDir: journalDir,
		stateDir:   stateDir,
		logPath:    logPath,
		storageDir: storageDir,
		backupRoot: backupRoot,
		backupSet:  writeRestoreBackupSet(t, backupRoot, "espocrm-"+scope, "2026-04-19_08-00-00", scope, files),
		fixedNow:   fixedNow,
	}
}

func writeRestoreBackupSet(t *testing.T, backupRoot, prefix, stamp, scope string, files map[string]string) restoreBackupSet {
	t.Helper()

	if len(files) == 0 {
		files = map[string]string{
			"espo/data/test.txt": "hello",
		}
	}

	set := restoreBackupSet{
		DBBackup:     filepath.Join(backupRoot, "db", prefix+"_"+stamp+".sql.gz"),
		FilesBackup:  filepath.Join(backupRoot, "files", prefix+"_files_"+stamp+".tar.gz"),
		ManifestTXT:  filepath.Join(backupRoot, "manifests", prefix+"_"+stamp+".manifest.txt"),
		ManifestJSON: filepath.Join(backupRoot, "manifests", prefix+"_"+stamp+".manifest.json"),
	}

	if err := os.MkdirAll(filepath.Dir(set.DBBackup), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(set.FilesBackup), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(set.ManifestJSON), 0o755); err != nil {
		t.Fatal(err)
	}

	writeGzipFile(t, set.DBBackup, []byte("select 1;"))
	writeTarGzFile(t, set.FilesBackup, files)
	if err := os.WriteFile(set.DBBackup+".sha256", []byte(sha256OfFile(t, set.DBBackup)+"  "+filepath.Base(set.DBBackup)+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(set.FilesBackup+".sha256", []byte(sha256OfFile(t, set.FilesBackup)+"  "+filepath.Base(set.FilesBackup)+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(set.ManifestTXT, []byte("created_at=2026-04-19T08:00:00Z\ncontour="+scope+"\ncompose_project="+prefix+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeJSON(t, set.ManifestJSON, map[string]any{
		"version":    1,
		"scope":      scope,
		"created_at": "2026-04-19T08:00:00Z",
		"artifacts": map[string]any{
			"db_backup":    filepath.Base(set.DBBackup),
			"files_backup": filepath.Base(set.FilesBackup),
		},
		"checksums": map[string]any{
			"db_backup":    sha256OfFile(t, set.DBBackup),
			"files_backup": sha256OfFile(t, set.FilesBackup),
		},
	})

	return set
}

func normalizeRestoreJSON(t *testing.T, raw []byte) []byte {
	t.Helper()

	var obj map[string]any
	if err := json.Unmarshal(raw, &obj); err != nil {
		t.Fatalf("invalid json output: %v\n%s", err, string(raw))
	}

	replacements := map[string]string{}
	if artifacts, ok := obj["artifacts"].(map[string]any); ok {
		for key, placeholder := range map[string]string{
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
		} {
			value, ok := artifacts[key].(string)
			if !ok || value == "" {
				continue
			}
			replacements[value] = placeholder
			artifacts[key] = placeholder
		}
	}

	if warnings, ok := obj["warnings"].([]any); ok {
		for idx, rawWarning := range warnings {
			warning, ok := rawWarning.(string)
			if !ok {
				continue
			}
			warnings[idx] = replaceKnownPaths(warning, replacements)
		}
	}

	if items, ok := obj["items"].([]any); ok {
		for _, rawItem := range items {
			item, ok := rawItem.(map[string]any)
			if !ok {
				continue
			}
			if value, ok := item["details"].(string); ok {
				item["details"] = replaceKnownPaths(value, replacements)
			}
			if value, ok := item["action"].(string); ok {
				item["action"] = replaceKnownPaths(value, replacements)
			}
		}
	}

	out, err := json.Marshal(obj)
	if err != nil {
		t.Fatal(err)
	}

	return out
}
