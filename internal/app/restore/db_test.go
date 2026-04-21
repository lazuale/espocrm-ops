package restore

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	domainbackup "github.com/lazuale/espocrm-ops/internal/domain/backup"
	platformconfig "github.com/lazuale/espocrm-ops/internal/platform/config"
)

func TestRestoreDB_DryRun(t *testing.T) {
	tmp := t.TempDir()

	dbName := "db_20260415_123456.sql.gz"
	filesName := "files_20260415_123456.tar.gz"
	dbPath := filepath.Join(tmp, "db", dbName)
	filesPath := filepath.Join(tmp, "files", filesName)
	manifestPath := filepath.Join(tmp, "manifests", "manifest.json")

	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(filesPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(manifestPath), 0o755); err != nil {
		t.Fatal(err)
	}

	writeGzipFile(t, dbPath, []byte("select 1;"))
	writeTarGz(t, filesPath, map[string]string{
		"storage/test.txt": "hello",
	})
	writeManifest(t, manifestPath, domainbackup.Manifest{
		Version:   1,
		Scope:     "prod",
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
		Artifacts: domainbackup.ManifestArtifacts{
			DBBackup:    dbName,
			FilesBackup: filesName,
		},
		Checksums: domainbackup.ManifestChecksums{
			DBBackup:    sha256OfFile(t, dbPath),
			FilesBackup: sha256OfFile(t, filesPath),
		},
	})
	prependFakeDocker(t, "", "")
	t.Setenv("ESPOPS_DB_ROOT_PASSWORD", "must-not-leak-into-usecase")

	plan, err := RestoreDB(RestoreDBRequest{
		ManifestPath: manifestPath,
		DBContainer:  "db-container",
		DBName:       "espocrm",
		DBUser:       "espocrm",
		DBPassword:   "secret",
		DryRun:       true,
	})
	if err != nil {
		t.Fatalf("RestoreDB dry-run failed: %v", err)
	}
	if plan.Plan.SourceKind != RestoreSourceManifest {
		t.Fatalf("unexpected source kind: %s", plan.Plan.SourceKind)
	}
	if plan.Plan.SourcePath != dbPath {
		t.Fatalf("unexpected source path: %s", plan.Plan.SourcePath)
	}
	if !strings.Contains(plan.Plan.NextStep, "root password") {
		t.Fatalf("expected next step to mention root password, got %q", plan.Plan.NextStep)
	}
}

func TestRestoreDB_DirectBackupDryRun(t *testing.T) {
	tmp := t.TempDir()

	dbPath := filepath.Join(tmp, "db.sql.gz")
	writeGzipFile(t, dbPath, []byte("select 1;"))
	writeSHA256Sidecar(t, dbPath)
	prependFakeDocker(t, "", "")

	plan, err := RestoreDB(RestoreDBRequest{
		DBBackup:    dbPath,
		DBContainer: "db-container",
		DBName:      "espocrm",
		DBUser:      "espocrm",
		DBPassword:  "secret",
		DryRun:      true,
	})
	if err != nil {
		t.Fatalf("RestoreDB direct dry-run failed: %v", err)
	}
	if plan.Plan.SourceKind != RestoreSourceDirectBackup {
		t.Fatalf("unexpected source kind: %s", plan.Plan.SourceKind)
	}
	if plan.Plan.SourcePath != dbPath {
		t.Fatalf("unexpected source path: %s", plan.Plan.SourcePath)
	}
}

func TestRestoreDB_RequiresTypedRootPasswordSourceOutsideDryRun(t *testing.T) {
	tmp := t.TempDir()

	dbPath := filepath.Join(tmp, "db.sql.gz")
	writeGzipFile(t, dbPath, []byte("select 1;"))
	writeSHA256Sidecar(t, dbPath)
	prependFakeDocker(t, "", "")

	_, err := RestoreDB(RestoreDBRequest{
		DBBackup:    dbPath,
		DBContainer: "db-container",
		DBName:      "espocrm",
		DBUser:      "espocrm",
		DBPassword:  "secret",
	})
	if err == nil {
		t.Fatal("expected missing root password error")
	}

	var preflightErr PreflightError
	if !errors.As(err, &preflightErr) {
		t.Fatalf("expected PreflightError, got %T: %v", err, err)
	}
	if preflightErr.ErrorKind() != "validation" {
		t.Fatalf("expected validation kind, got %q", preflightErr.ErrorKind())
	}
	if preflightErr.ErrorCode() != "preflight_failed" {
		t.Fatalf("expected preflight_failed code, got %q", preflightErr.ErrorCode())
	}

	var requiredErr platformconfig.PasswordRequiredError
	if !errors.As(err, &requiredErr) {
		t.Fatalf("expected PasswordRequiredError cause, got %T: %v", err, err)
	}
	if requiredErr.Label != "db root password" {
		t.Fatalf("unexpected required label: %q", requiredErr.Label)
	}
}

func prependFakeDocker(t *testing.T, logPath, stdinPath string) {
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
  echo true
  exit 0
fi

if [[ "${1:-}" == "exec" && "${3:-}" == "mariadb" && "${4:-}" == "--version" ]]; then
  echo "mariadb from 11.0.0"
  exit 0
fi

if [[ "${1:-}" == "exec" && "${2:-}" == "-i" ]]; then
  if [[ -n "${DOCKER_LOG:-}" ]]; then
    printf '%s\n' "$*" > "$DOCKER_LOG"
  fi
  if [[ -n "${DOCKER_STDIN_LOG:-}" ]]; then
    cat > "$DOCKER_STDIN_LOG"
  else
    cat >/dev/null
  fi
  exit 0
fi

echo "unexpected docker invocation: $*" >&2
exit 98
`
	if err := os.WriteFile(dockerPath, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	if logPath != "" {
		t.Setenv("DOCKER_LOG", logPath)
	}
	if stdinPath != "" {
		t.Setenv("DOCKER_STDIN_LOG", stdinPath)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
}
