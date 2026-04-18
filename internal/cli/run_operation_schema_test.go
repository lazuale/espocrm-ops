package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lazuale/espocrm-ops/internal/contract/exitcode"
)

func TestSchema_RunOperation_JSON_Success(t *testing.T) {
	tmp := t.TempDir()
	envFile := filepath.Join(tmp, ".env.dev")
	envBody := strings.Join([]string{
		"ESPO_CONTOUR=dev",
		"COMPOSE_PROJECT_NAME=espocrm-test",
		"DB_STORAGE_DIR=./runtime/db",
		"ESPO_STORAGE_DIR=./runtime/espo",
		"BACKUP_ROOT=./backups/dev",
	}, "\n") + "\n"
	if err := os.WriteFile(envFile, []byte(envBody), 0o600); err != nil {
		t.Fatal(err)
	}

	outcome := executeCLI(
		"--json",
		"run-operation",
		"--scope", "dev",
		"--operation", "backup",
		"--project-dir", tmp,
		"--env-file", envFile,
		"--",
		"bash", "-lc", "exit 0",
	)

	if outcome.ExitCode != exitcode.OK {
		t.Fatalf("expected OK exit code, got %d with stdout=%q stderr=%q", outcome.ExitCode, outcome.Stdout, outcome.Stderr)
	}
	if outcome.Stderr != "" {
		t.Fatalf("expected empty stderr, got %q", outcome.Stderr)
	}
	if !strings.Contains(outcome.Stdout, `"command": "run-operation"`) {
		t.Fatalf("expected run-operation command in json output, got %s", outcome.Stdout)
	}
	if !strings.Contains(outcome.Stdout, `"operation": "backup"`) {
		t.Fatalf("expected backup operation in json output, got %s", outcome.Stdout)
	}
}

func TestRunOperation_TextChildFailureDoesNotAddOuterError(t *testing.T) {
	tmp := t.TempDir()
	envFile := filepath.Join(tmp, ".env.dev")
	envBody := strings.Join([]string{
		"ESPO_CONTOUR=dev",
		"COMPOSE_PROJECT_NAME=espocrm-test",
		"DB_STORAGE_DIR=./runtime/db",
		"ESPO_STORAGE_DIR=./runtime/espo",
		"BACKUP_ROOT=./backups/dev",
	}, "\n") + "\n"
	if err := os.WriteFile(envFile, []byte(envBody), 0o600); err != nil {
		t.Fatal(err)
	}

	outcome := executeCLI(
		"run-operation",
		"--scope", "dev",
		"--operation", "backup",
		"--project-dir", tmp,
		"--env-file", envFile,
		"--",
		"bash", "-lc", "printf 'child failed\\n' >&2; exit 7",
	)

	if outcome.ExitCode != 7 {
		t.Fatalf("expected child exit code 7, got %d with stdout=%q stderr=%q", outcome.ExitCode, outcome.Stdout, outcome.Stderr)
	}
	if strings.Contains(outcome.Stderr, "ERROR:") {
		t.Fatalf("expected child stderr passthrough without outer ERROR prefix, got %q", outcome.Stderr)
	}
	if !strings.Contains(outcome.Stderr, "child failed") {
		t.Fatalf("expected child stderr output, got %q", outcome.Stderr)
	}
}
