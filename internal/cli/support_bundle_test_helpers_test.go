package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	platformfs "github.com/lazuale/espocrm-ops/internal/platform/fs"
)

type supportBundleFixture struct {
	restoreCommandFixture
}

func prepareSupportBundleFixture(t *testing.T, scope string) supportBundleFixture {
	t.Helper()

	base := prepareRestoreCommandFixture(t, scope, map[string]string{
		"espo/data/support.txt": "support bundle",
	})
	if err := os.MkdirAll(base.journalDir, 0o755); err != nil {
		t.Fatal(err)
	}

	writeJournalEntryFile(t, base.journalDir, "backup.json", map[string]any{
		"operation_id": "op-backup-1",
		"command":      "backup",
		"started_at":   "2026-04-19T08:00:00Z",
		"finished_at":  "2026-04-19T08:05:00Z",
		"ok":           true,
		"message":      "backup created",
		"details": map[string]any{
			"scope": scope,
		},
		"artifacts": map[string]any{
			"manifest":      base.backupSet.ManifestTXT,
			"manifest_txt":  base.backupSet.ManifestTXT,
			"manifest_json": base.backupSet.ManifestJSON,
			"db_backup":     base.backupSet.DBBackup,
			"files_backup":  base.backupSet.FilesBackup,
		},
	})
	writeJournalEntryFile(t, base.journalDir, "restore.json", map[string]any{
		"operation_id":  "op-restore-1",
		"command":       "restore",
		"started_at":    "2026-04-19T08:30:00Z",
		"finished_at":   "2026-04-19T08:32:00Z",
		"ok":            false,
		"message":       "restore failed",
		"error_code":    "restore_failed",
		"error_message": "runtime return failed",
		"details": map[string]any{
			"scope": scope,
		},
		"items": []any{
			map[string]any{
				"code":    "runtime_return",
				"status":  "failed",
				"summary": "Runtime return failed",
				"details": "HTTP probe failed",
			},
		},
	})

	return supportBundleFixture{restoreCommandFixture: base}
}

func prependSupportBundleFakeDocker(t *testing.T) {
	t.Helper()

	binDir := t.TempDir()
	dockerPath := filepath.Join(binDir, "docker")
	script := `#!/usr/bin/env bash
set -Eeuo pipefail

if [[ "${1:-}" == "version" && "${2:-}" == "--format" && "${3:-}" == "{{.Client.Version}}" ]]; then
  echo "25.0.2"
  exit 0
fi

if [[ "${1:-}" == "version" && "${2:-}" == "--format" && "${3:-}" == "{{.Server.Version}}" ]]; then
  echo "25.0.2"
  exit 0
fi

if [[ "${1:-}" == "version" ]]; then
  cat <<'EOF'
Client:
 Version: 25.0.2
Server:
 Version: 25.0.2
EOF
  exit 0
fi

if [[ "${1:-}" == "compose" && "${2:-}" == "version" && "${3:-}" == "--short" ]]; then
  echo "2.24.1"
  exit 0
fi

if [[ "${1:-}" == "compose" && "${2:-}" == "version" ]]; then
  echo "Docker Compose version v2.24.1"
  exit 0
fi

if [[ "${1:-}" == "compose" ]]; then
  shift
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --project-directory|-f|--env-file)
        shift 2
        ;;
      config)
        shift
        if [[ "${1:-}" == "-q" ]]; then
          exit 0
        fi
        cat <<'YAML'
services:
  espocrm:
    environment:
      DB_PASSWORD: db-secret
      ADMIN_PASSWORD: admin-secret
      API_TOKEN: token-secret
      SITE_URL: http://127.0.0.1:18080
YAML
        exit 0
        ;;
      ps)
        shift
        if [[ "${1:-}" == "--status" && "${2:-}" == "running" && "${3:-}" == "--services" ]]; then
          printf 'db\nespocrm\n'
          exit 0
        fi
        if [[ "${1:-}" == "-q" ]]; then
          echo "mock-${2:-service}"
          exit 0
        fi
        cat <<'EOF'
NAME            IMAGE     COMMAND   SERVICE   STATUS
mock-db         maria     "run"     db        running
mock-espocrm    espo      "run"     espocrm   running
EOF
        exit 0
        ;;
      logs)
        shift
        while [[ $# -gt 0 ]]; do
          case "$1" in
            --no-color)
              shift
              ;;
            --tail)
              shift 2
              ;;
            *)
              shift
              ;;
          esac
        done
        cat <<'EOF'
db | ready
espocrm | request handled
EOF
        exit 0
        ;;
      *)
        shift
        ;;
    esac
  done
fi

if [[ "${1:-}" == "inspect" ]]; then
  echo "healthy"
  exit 0
fi

echo "unexpected docker invocation: $*" >&2
exit 97
`
	if err := os.WriteFile(dockerPath, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func prependSupportBundleUnavailableDocker(t *testing.T) {
	t.Helper()

	binDir := t.TempDir()
	dockerPath := filepath.Join(binDir, "docker")
	script := `#!/usr/bin/env bash
set -Eeuo pipefail
echo "docker unavailable in support-bundle test: $*" >&2
exit 1
`
	if err := os.WriteFile(dockerPath, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func normalizeSupportBundleJSON(t *testing.T, raw []byte) []byte {
	t.Helper()

	var obj map[string]any
	if err := json.Unmarshal(raw, &obj); err != nil {
		t.Fatalf("invalid json output: %v\n%s", err, string(raw))
	}

	if artifacts, ok := obj["artifacts"].(map[string]any); ok {
		for key, placeholder := range map[string]string{
			"project_dir":  "REPLACE_PROJECT_DIR",
			"compose_file": "REPLACE_COMPOSE_FILE",
			"env_file":     "REPLACE_ENV_FILE",
			"backup_root":  "REPLACE_BACKUP_ROOT",
			"bundle_path":  "REPLACE_BUNDLE_PATH",
		} {
			if _, ok := artifacts[key]; ok {
				artifacts[key] = placeholder
			}
		}
	}

	out, err := json.Marshal(obj)
	if err != nil {
		t.Fatal(err)
	}

	return out
}

func unpackSupportBundle(t *testing.T, archivePath string) string {
	t.Helper()

	destDir := t.TempDir()
	if err := platformfs.UnpackTarGz(archivePath, destDir, nil); err != nil {
		t.Fatalf("unpack support bundle: %v", err)
	}
	return destDir
}

func bundledFileByBaseName(t *testing.T, root, base string) string {
	t.Helper()

	found := ""
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if filepath.Base(path) == base {
			found = path
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk bundle: %v", err)
	}
	if found == "" {
		t.Fatalf("could not find bundled file %s under %s", base, root)
	}
	return found
}
