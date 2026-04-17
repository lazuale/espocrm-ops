package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSchema_RestoreDB_JSON_DryRun(t *testing.T) {
	tmp := t.TempDir()
	journalDir := filepath.Join(tmp, "journal")

	opts := []testAppOption{
		withFixedTestRuntime(time.Date(2026, 4, 15, 12, 0, 0, 0, time.UTC), "op-rd-1"),
	}

	dbName := "db.sql.gz"
	filesName := "files.tar.gz"
	manifestPath := filepath.Join(tmp, "manifest.json")
	dbPath := filepath.Join(tmp, dbName)
	filesPath := filepath.Join(tmp, filesName)
	passwordPath := filepath.Join(tmp, "secret")

	writeGzipFile(t, dbPath, []byte("select 1;"))
	writeTarGzFile(t, filesPath, map[string]string{
		"storage/a.txt": "hello",
	})
	writeJSON(t, manifestPath, map[string]any{
		"version":    1,
		"scope":      "prod",
		"created_at": "2026-04-15T11:00:00Z",
		"artifacts": map[string]any{
			"db_backup":    dbName,
			"files_backup": filesName,
		},
		"checksums": map[string]any{
			"db_backup":    sha256OfFile(t, dbPath),
			"files_backup": sha256OfFile(t, filesPath),
		},
	})
	if err := os.WriteFile(passwordPath, []byte("secret\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	prependFakeDockerForCLITest(t)

	out, err := runRootCommandWithOptions(t, opts,
		"--journal-dir", journalDir,
		"--json",
		"restore-db",
		"--manifest", manifestPath,
		"--db-container", "espocrm-db",
		"--db-name", "espocrm",
		"--db-user", "espocrm",
		"--db-password-file", passwordPath,
		"--dry-run",
	)
	if err != nil {
		t.Fatalf("command failed: %v\noutput=%s", err, out)
	}

	var obj map[string]any
	if err := json.Unmarshal([]byte(out), &obj); err != nil {
		t.Fatal(err)
	}

	requireJSONPath(t, obj, "command")
	requireJSONPath(t, obj, "ok")
	requireJSONPath(t, obj, "message")
	requireJSONPath(t, obj, "details", "dry_run")
	requireJSONPath(t, obj, "details", "db_user")
	requireJSONPath(t, obj, "details", "plan", "source_kind")
	requireJSONPath(t, obj, "details", "plan", "source_path")
	requireJSONPath(t, obj, "details", "plan", "destructive")
	requireJSONPath(t, obj, "details", "plan", "changes")
	requireJSONPath(t, obj, "details", "plan", "non_changes")
	requireJSONPath(t, obj, "details", "plan", "checks")
	requireJSONPath(t, obj, "details", "plan", "next_step")
	requireJSONPath(t, obj, "artifacts", "manifest")
	requireJSONPath(t, obj, "artifacts", "db_container")
	requireJSONPath(t, obj, "artifacts", "db_name")

	if obj["command"] != "restore-db" {
		t.Fatalf("unexpected command: %v", obj["command"])
	}
	if ok, _ := obj["ok"].(bool); !ok {
		t.Fatalf("expected ok=true, got %v", obj["ok"])
	}
	if dryRun, _ := requireJSONPath(t, obj, "details", "dry_run").(bool); !dryRun {
		t.Fatalf("expected details.dry_run=true")
	}
	if dbUser := requireJSONPath(t, obj, "details", "db_user"); dbUser != "espocrm" {
		t.Fatalf("unexpected details.db_user: %v", dbUser)
	}
	if sourceKind := requireJSONPath(t, obj, "details", "plan", "source_kind"); sourceKind != "manifest" {
		t.Fatalf("unexpected details.plan.source_kind: %v", sourceKind)
	}
	if destructive, _ := requireJSONPath(t, obj, "details", "plan", "destructive").(bool); !destructive {
		t.Fatalf("expected details.plan.destructive=true")
	}
	checks, ok := requireJSONPath(t, obj, "details", "plan", "checks").([]any)
	if !ok || len(checks) == 0 {
		t.Fatalf("expected non-empty details.plan.checks, got %v", requireJSONPath(t, obj, "details", "plan", "checks"))
	}
}

func prependFakeDockerForCLITest(t *testing.T) {
	t.Helper()

	binDir := t.TempDir()
	dockerPath := filepath.Join(binDir, "docker")
	script := `#!/usr/bin/env bash
set -euo pipefail

if [[ "${1:-}" == "version" ]]; then
  echo "11.0.0"
  exit 0
fi

if [[ "${1:-}" == "inspect" ]]; then
  echo "${DOCKER_INSPECT_RUNNING:-true}"
  exit 0
fi

if [[ "${1:-}" == "exec" && "${3:-}" == "mariadb" && "${4:-}" == "--version" ]]; then
  if [[ "${DOCKER_MARIADB_AVAILABLE:-true}" == "true" ]]; then
    echo "mariadb from 11.0.0"
    exit 0
  fi
  echo "mariadb missing" >&2
  exit 2
fi

if [[ "${1:-}" == "exec" && "${3:-}" == "mysql" && "${4:-}" == "--version" ]]; then
  if [[ "${DOCKER_MYSQL_AVAILABLE:-true}" != "true" ]]; then
    echo "mysql missing" >&2
    exit 2
  fi
  echo "mysql from 8.0.0"
  exit 0
fi

if [[ "${1:-}" == "exec" && "${2:-}" == "-i" ]]; then
  if [[ -n "${DOCKER_EXEC_STDERR:-}" ]]; then
    printf '%s\n' "$DOCKER_EXEC_STDERR" >&2
  fi
  if [[ -n "${DOCKER_EXEC_EXIT_CODE:-}" ]]; then
    cat >/dev/null
    exit "$DOCKER_EXEC_EXIT_CODE"
  fi
  cat >/dev/null
  exit 0
fi

echo "unexpected docker invocation: $*" >&2
exit 98
`
	if err := os.WriteFile(dockerPath, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
}
