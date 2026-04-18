package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/lazuale/espocrm-ops/internal/platform/locks"
)

func prependFakeDockerForRollbackPlanCLITest(t *testing.T) {
	t.Helper()
	prependFakeDockerForUpdatePlanCLITest(t)
}

func isolateRollbackPlanLocks(t *testing.T) {
	t.Helper()

	restore := locks.SetLockDirForTest(t.TempDir())
	t.Cleanup(restore)
}

func writeRollbackBackupSet(t *testing.T, backupRoot, prefix, stamp, scope string) {
	t.Helper()

	dbPath := filepath.Join(backupRoot, "db", prefix+"_"+stamp+".sql.gz")
	filesPath := filepath.Join(backupRoot, "files", prefix+"_files_"+stamp+".tar.gz")
	manifestTXT := filepath.Join(backupRoot, "manifests", prefix+"_"+stamp+".manifest.txt")
	manifestJSON := filepath.Join(backupRoot, "manifests", prefix+"_"+stamp+".manifest.json")

	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(filesPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(manifestJSON), 0o755); err != nil {
		t.Fatal(err)
	}

	writeGzipFile(t, dbPath, []byte("select 1;"))
	writeTarGzFile(t, filesPath, map[string]string{
		"storage/test.txt": "hello",
	})
	if err := os.WriteFile(dbPath+".sha256", []byte(sha256OfFile(t, dbPath)+"  "+filepath.Base(dbPath)+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filesPath+".sha256", []byte(sha256OfFile(t, filesPath)+"  "+filepath.Base(filesPath)+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manifestTXT, []byte("created_at=2026-04-18T10:00:00Z\ncontour="+scope+"\ncompose_project="+prefix+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeJSON(t, manifestJSON, map[string]any{
		"version":    1,
		"scope":      scope,
		"created_at": "2026-04-18T10:00:00Z",
		"artifacts": map[string]any{
			"db_backup":    filepath.Base(dbPath),
			"files_backup": filepath.Base(filesPath),
		},
		"checksums": map[string]any{
			"db_backup":    sha256OfFile(t, dbPath),
			"files_backup": sha256OfFile(t, filesPath),
		},
	})
}

func normalizeRollbackPlanJSON(t *testing.T, raw []byte) []byte {
	t.Helper()

	var obj map[string]any
	if err := json.Unmarshal(raw, &obj); err != nil {
		t.Fatalf("invalid json output: %v\n%s", err, string(raw))
	}

	replacements := map[string]string{}
	if artifacts, ok := obj["artifacts"].(map[string]any); ok {
		for key, placeholder := range map[string]string{
			"project_dir":   "REPLACE_PROJECT_DIR",
			"compose_file":  "REPLACE_COMPOSE_FILE",
			"env_file":      "REPLACE_ENV_FILE",
			"backup_root":   "REPLACE_BACKUP_ROOT",
			"manifest_txt":  "REPLACE_MANIFEST_TXT",
			"manifest_json": "REPLACE_MANIFEST_JSON",
			"db_backup":     "REPLACE_DB_BACKUP",
			"files_backup":  "REPLACE_FILES_BACKUP",
		} {
			if value, ok := artifacts[key].(string); ok && value != "" {
				replacements[value] = placeholder
				artifacts[key] = placeholder
			}
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
