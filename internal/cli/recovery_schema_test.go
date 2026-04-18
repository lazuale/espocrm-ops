package cli

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lazuale/espocrm-ops/internal/contract/exitcode"
)

func TestSchema_Update_JSON_RecoveryResume(t *testing.T) {
	tmp := t.TempDir()
	journalDir := filepath.Join(tmp, "journal")
	projectDir := filepath.Join(tmp, "project")
	stateDir := filepath.Join(tmp, "docker-state")
	logPath := filepath.Join(tmp, "docker.log")
	appPort := freeTCPPort(t)
	wsPort := freeTCPPort(t)

	if err := os.MkdirAll(journalDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(projectDir, "runtime", "prod", "db"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(projectDir, "runtime", "prod", "espo"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(projectDir, "backups", "prod"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		t.Fatal(err)
	}
	composeFile := filepath.Join(projectDir, "compose.yaml")
	if err := os.WriteFile(composeFile, []byte("services: {}\n"), 0o644); err != nil {
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

	envFile := writeDoctorEnvFile(t, projectDir, "prod", map[string]string{
		"APP_PORT":      fmt.Sprintf("%d", appPort),
		"WS_PORT":       fmt.Sprintf("%d", wsPort),
		"SITE_URL":      "http://" + listener.Addr().String(),
		"WS_PUBLIC_URL": fmt.Sprintf("ws://127.0.0.1:%d", wsPort),
	})

	prependFakeDockerForUpdateCLITest(t)
	t.Setenv("DOCKER_MOCK_UPDATE_STATE_DIR", stateDir)
	t.Setenv("DOCKER_MOCK_UPDATE_LOG", logPath)

	writeJournalEntryFile(t, journalDir, "failed-update.json", map[string]any{
		"operation_id":  "op-update-failed-1",
		"command":       "update",
		"started_at":    "2026-04-19T10:00:00Z",
		"finished_at":   "2026-04-19T10:00:05Z",
		"ok":            false,
		"message":       "update failed",
		"error_code":    "update_failed",
		"error_message": "http probe failed",
		"details": map[string]any{
			"scope":                  "prod",
			"timeout_seconds":        10,
			"skip_doctor":            false,
			"skip_backup":            false,
			"skip_pull":              false,
			"skip_http_probe":        false,
			"started_db_temporarily": true,
		},
		"artifacts": map[string]any{
			"project_dir":   projectDir,
			"compose_file":  composeFile,
			"env_file":      envFile,
			"backup_root":   filepath.Join(projectDir, "backups", "prod"),
			"site_url":      "http://" + listener.Addr().String(),
			"manifest_json": filepath.Join(projectDir, "backups", "prod", "manifest.json"),
		},
		"items": []map[string]any{
			{"code": "operation_preflight", "status": "completed", "summary": "Update preflight completed"},
			{"code": "doctor", "status": "completed", "summary": "Doctor completed"},
			{"code": "backup_recovery_point", "status": "completed", "summary": "Recovery-point creation completed"},
			{"code": "runtime_apply", "status": "completed", "summary": "Runtime apply completed"},
			{"code": "runtime_readiness", "status": "failed", "summary": "Runtime readiness checks failed", "details": "http probe failed"},
		},
	})

	outcome := executeCLI(
		"--journal-dir", journalDir,
		"--json",
		"update",
		"--recover-operation", "op-update-failed-1",
		"--recover-mode", "resume",
	)
	if outcome.ExitCode != 0 {
		t.Fatalf("command failed\nstdout=%s\nstderr=%s", outcome.Stdout, outcome.Stderr)
	}

	var obj map[string]any
	if err := json.Unmarshal([]byte(outcome.Stdout), &obj); err != nil {
		t.Fatal(err)
	}

	if obj["message"] != "update recovery completed" {
		t.Fatalf("unexpected message: %v", obj["message"])
	}
	recovery := requireJSONPath(t, obj, "details", "recovery").(map[string]any)
	if recovery["source_operation_id"] != "op-update-failed-1" {
		t.Fatalf("unexpected source operation: %#v", recovery)
	}
	if recovery["applied_mode"] != "resume" || recovery["resume_step"] != "runtime_readiness" {
		t.Fatalf("unexpected recovery details: %#v", recovery)
	}

	items := requireJSONPath(t, obj, "items").([]any)
	if len(items) != 5 {
		t.Fatalf("expected five recovery items, got %#v", items)
	}
	if items[1].(map[string]any)["status"] != "skipped" {
		t.Fatalf("expected skipped doctor step, got %#v", items[1])
	}
	if items[2].(map[string]any)["status"] != "skipped" {
		t.Fatalf("expected skipped backup step, got %#v", items[2])
	}
	if items[3].(map[string]any)["status"] != "skipped" {
		t.Fatalf("expected skipped runtime apply step, got %#v", items[3])
	}
	if items[4].(map[string]any)["status"] != "completed" {
		t.Fatalf("expected completed runtime readiness step, got %#v", items[4])
	}

	rawLog, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	log := string(rawLog)
	if strings.Contains(log, " pull ") || strings.Contains(log, " up -d ") || strings.Contains(log, " mariadb-dump ") {
		t.Fatalf("resume should not rerun update runtime apply or backup work:\n%s", log)
	}
}

func TestSchema_Update_JSON_RecoveryRefused(t *testing.T) {
	tmp := t.TempDir()
	journalDir := filepath.Join(tmp, "journal")

	if err := os.MkdirAll(journalDir, 0o755); err != nil {
		t.Fatal(err)
	}

	writeJournalEntryFile(t, journalDir, "failed-update.json", map[string]any{
		"operation_id":  "op-update-failed-2",
		"command":       "update",
		"started_at":    "2026-04-19T11:00:00Z",
		"finished_at":   "2026-04-19T11:00:05Z",
		"ok":            false,
		"message":       "update failed",
		"error_code":    "update_failed",
		"error_message": "doctor failed",
		"details": map[string]any{
			"scope": "prod",
		},
		"items": []map[string]any{
			{"code": "doctor", "status": "failed", "summary": "Doctor stopped the update"},
			{"code": "runtime_apply", "status": "not_run", "summary": "Runtime apply did not run because doctor failed"},
		},
	})

	outcome := executeCLI(
		"--journal-dir", journalDir,
		"--json",
		"update",
		"--recover-operation", "op-update-failed-2",
		"--recover-mode", "resume",
	)
	if outcome.ExitCode != exitcode.ValidationError {
		t.Fatalf("expected validation exit code, got %d stdout=%s stderr=%s", outcome.ExitCode, outcome.Stdout, outcome.Stderr)
	}

	var obj map[string]any
	if err := json.Unmarshal([]byte(outcome.Stdout), &obj); err != nil {
		t.Fatal(err)
	}

	if code := requireJSONPath(t, obj, "error", "code"); code != "update_recovery_refused" {
		t.Fatalf("unexpected error code: %v", code)
	}
}

func TestSchema_Rollback_JSON_RecoveryResume(t *testing.T) {
	tmp := t.TempDir()
	journalDir := filepath.Join(tmp, "journal")
	projectDir := filepath.Join(tmp, "project")
	stateDir := filepath.Join(tmp, "docker-state")
	logPath := filepath.Join(tmp, "docker.log")
	appPort := freeTCPPort(t)
	wsPort := freeTCPPort(t)

	if err := os.MkdirAll(journalDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(projectDir, "runtime", "prod", "db"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(projectDir, "runtime", "prod", "espo"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(projectDir, "backups", "prod"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		t.Fatal(err)
	}
	composeFile := filepath.Join(projectDir, "compose.yaml")
	if err := os.WriteFile(composeFile, []byte("services: {}\n"), 0o644); err != nil {
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

	envFile := writeDoctorEnvFile(t, projectDir, "prod", map[string]string{
		"APP_PORT":      fmt.Sprintf("%d", appPort),
		"WS_PORT":       fmt.Sprintf("%d", wsPort),
		"SITE_URL":      "http://" + listener.Addr().String(),
		"WS_PUBLIC_URL": fmt.Sprintf("ws://127.0.0.1:%d", wsPort),
	})

	prependFakeDockerForRollbackCLITest(t)
	t.Setenv("DOCKER_MOCK_ROLLBACK_STATE_DIR", stateDir)
	t.Setenv("DOCKER_MOCK_ROLLBACK_LOG", logPath)

	writeJournalEntryFile(t, journalDir, "failed-rollback.json", map[string]any{
		"operation_id":  "op-rollback-failed-1",
		"command":       "rollback",
		"started_at":    "2026-04-19T12:00:00Z",
		"finished_at":   "2026-04-19T12:00:08Z",
		"ok":            false,
		"message":       "rollback failed",
		"error_code":    "rollback_failed",
		"error_message": "contour return failed",
		"details": map[string]any{
			"scope":                    "prod",
			"requested_selection_mode": "auto_latest_valid",
			"selection_mode":           "auto_latest_valid",
			"timeout_seconds":          10,
			"snapshot_enabled":         true,
			"no_start":                 false,
			"skip_http_probe":          false,
			"started_db_temporarily":   true,
		},
		"artifacts": map[string]any{
			"project_dir":            projectDir,
			"compose_file":           composeFile,
			"env_file":               envFile,
			"backup_root":            filepath.Join(projectDir, "backups", "prod"),
			"site_url":               "http://" + listener.Addr().String(),
			"selected_prefix":        "espocrm-prod",
			"selected_stamp":         "2026-04-18_10-00-00",
			"manifest_json":          filepath.Join(projectDir, "backups", "prod", "manifest.json"),
			"db_backup":              filepath.Join(projectDir, "backups", "prod", "db.sql.gz"),
			"files_backup":           filepath.Join(projectDir, "backups", "prod", "files.tar.gz"),
			"snapshot_manifest_json": filepath.Join(projectDir, "backups", "prod", "snapshot-manifest.json"),
		},
		"items": []map[string]any{
			{"code": "operation_preflight", "status": "completed", "summary": "Rollback preflight completed"},
			{"code": "doctor", "status": "completed", "summary": "Doctor completed"},
			{"code": "target_selection", "status": "completed", "summary": "Automatic rollback target selection completed"},
			{"code": "runtime_prepare", "status": "completed", "summary": "Runtime preparation completed"},
			{"code": "snapshot_recovery_point", "status": "completed", "summary": "Emergency recovery-point creation completed"},
			{"code": "db_restore", "status": "completed", "summary": "Database restore completed"},
			{"code": "files_restore", "status": "completed", "summary": "Files restore completed"},
			{"code": "runtime_return", "status": "failed", "summary": "Contour return failed", "details": "http probe failed"},
		},
	})

	outcome := executeCLI(
		"--journal-dir", journalDir,
		"--json",
		"rollback",
		"--recover-operation", "op-rollback-failed-1",
		"--recover-mode", "resume",
		"--force",
		"--confirm-prod", "prod",
	)
	if outcome.ExitCode != 0 {
		t.Fatalf("command failed\nstdout=%s\nstderr=%s", outcome.Stdout, outcome.Stderr)
	}

	var obj map[string]any
	if err := json.Unmarshal([]byte(outcome.Stdout), &obj); err != nil {
		t.Fatal(err)
	}

	if obj["message"] != "rollback recovery completed" {
		t.Fatalf("unexpected message: %v", obj["message"])
	}
	recovery := requireJSONPath(t, obj, "details", "recovery").(map[string]any)
	if recovery["applied_mode"] != "resume" || recovery["resume_step"] != "runtime_return" {
		t.Fatalf("unexpected recovery details: %#v", recovery)
	}

	items := requireJSONPath(t, obj, "items").([]any)
	if len(items) != 8 {
		t.Fatalf("expected eight rollback recovery items, got %#v", items)
	}
	for _, index := range []int{1, 2, 3, 4, 5, 6} {
		if items[index].(map[string]any)["status"] != "skipped" {
			t.Fatalf("expected skipped recovered step at index %d, got %#v", index, items[index])
		}
	}
	if items[7].(map[string]any)["status"] != "completed" {
		t.Fatalf("expected completed runtime_return step, got %#v", items[7])
	}

	rawLog, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	log := string(rawLog)
	if !strings.Contains(log, " up -d") {
		t.Fatalf("expected rollback recovery to restart the contour:\n%s", log)
	}
	if strings.Contains(log, " stop ") || strings.Contains(log, " mariadb ") || strings.Contains(log, " mariadb-dump ") {
		t.Fatalf("runtime_return resume should not redo rollback prepare or restore work:\n%s", log)
	}
}
