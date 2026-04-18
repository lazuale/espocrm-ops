package cli

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestGolden_Rollback_JSON(t *testing.T) {
	isolateRollbackPlanLocks(t)

	tmp := t.TempDir()
	projectDir := filepath.Join(tmp, "project")
	journalDir := filepath.Join(tmp, "journal")
	stateDir := filepath.Join(tmp, "docker-state")
	storageDir := filepath.Join(projectDir, "runtime", "prod", "espo")
	fixedNow := time.Date(2026, 4, 18, 13, 0, 0, 0, time.UTC)
	appPort := freeTCPPort(t)
	wsPort := freeTCPPort(t)

	useJournalClockForTest(t, fixedNow)

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
	if err := os.WriteFile(filepath.Join(stateDir, "running-services"), []byte("espocrm\nespocrm-daemon\nespocrm-websocket\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeUpdateRuntimeStatusFile(t, stateDir, "db", "healthy")
	writeUpdateRuntimeStatusFile(t, stateDir, "espocrm", "healthy")
	writeUpdateRuntimeStatusFile(t, stateDir, "espocrm-daemon", "healthy")
	writeUpdateRuntimeStatusFile(t, stateDir, "espocrm-websocket", "healthy")

	listener, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", appPort))
	if err != nil {
		t.Fatal(err)
	}
	server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})}
	go func() {
		_ = server.Serve(listener)
	}()
	defer func() {
		_ = server.Close()
	}()

	writeDoctorEnvFile(t, projectDir, "prod", map[string]string{
		"APP_PORT":      fmt.Sprintf("%d", appPort),
		"WS_PORT":       fmt.Sprintf("%d", wsPort),
		"SITE_URL":      "http://" + listener.Addr().String(),
		"WS_PUBLIC_URL": fmt.Sprintf("ws://127.0.0.1:%d", wsPort),
	})
	writeRollbackBackupSet(t, filepath.Join(projectDir, "backups", "prod"), "espocrm-prod", "2026-04-18_10-00-00", "prod")

	prependFakeDockerForRollbackCLITest(t)
	t.Setenv("DOCKER_MOCK_ROLLBACK_STATE_DIR", stateDir)
	t.Setenv("DOCKER_MOCK_ROLLBACK_DUMP_STDOUT", "create table test(id int);\n")

	outcome := executeCLIWithOptions(
		[]testAppOption{withFixedTestRuntime(fixedNow, "op-rollback-1")},
		"--journal-dir", journalDir,
		"--json",
		"rollback",
		"--scope", "prod",
		"--project-dir", projectDir,
		"--force",
		"--confirm-prod", "prod",
		"--timeout", "10",
	)
	if outcome.ExitCode != 0 {
		t.Fatalf("command failed\nstdout=%s\nstderr=%s", outcome.Stdout, outcome.Stderr)
	}

	normalized := normalizeRollbackJSON(t, []byte(outcome.Stdout))
	assertGoldenJSON(t, normalized, filepath.Join("testdata", "rollback_ok.golden.json"))
}
