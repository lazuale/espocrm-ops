package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lazuale/espocrm-ops/internal/contract/exitcode"
)

func TestSchema_Maintenance_JSON_UnattendedApplyRequiresExplicitAllow(t *testing.T) {
	fixture := prepareMaintenanceFixture(t, "dev")
	useJournalClockForTest(t, fixture.fixedNow)

	outcome := executeCLIWithOptions(
		[]testAppOption{withFixedTestRuntime(fixture.fixedNow, "op-maintenance-unattended-blocked-1")},
		"--journal-dir", fixture.journalDir,
		"--json",
		"maintenance",
		"--scope", "dev",
		"--project-dir", fixture.projectDir,
		"--journal-keep-days", "30",
		"--journal-keep-last", "2",
		"--unattended",
		"--apply",
	)
	if outcome.ExitCode != exitcode.ValidationError {
		t.Fatalf("expected validation exit code %d, got %d\nstdout=%s\nstderr=%s", exitcode.ValidationError, outcome.ExitCode, outcome.Stdout, outcome.Stderr)
	}

	var obj map[string]any
	if err := json.Unmarshal([]byte(outcome.Stdout), &obj); err != nil {
		t.Fatal(err)
	}

	if code := requireJSONPath(t, obj, "error", "code"); code != "maintenance_blocked" {
		t.Fatalf("unexpected error code: %v", code)
	}
	if mode := requireJSONPath(t, obj, "details", "mode"); mode != "apply" {
		t.Fatalf("unexpected mode: %v", mode)
	}
	if unattended, _ := requireJSONPath(t, obj, "details", "unattended").(bool); !unattended {
		t.Fatalf("expected unattended=true")
	}
	if outcomeValue := requireJSONPath(t, obj, "details", "outcome"); outcomeValue != "blocked" {
		t.Fatalf("unexpected outcome: %v", outcomeValue)
	}

	items := requireJSONPath(t, obj, "items").([]any)
	if len(items) != 5 {
		t.Fatalf("expected 5 maintenance sections, got %d", len(items))
	}
	context := items[0].(map[string]any)
	if status := context["status"]; status != "failed" {
		t.Fatalf("expected failed context section, got %v", status)
	}
	if code := context["failure_code"]; code != "unattended_apply_requires_explicit_allow" {
		t.Fatalf("unexpected context failure code: %v", code)
	}

	for _, path := range []string{
		fixture.oldJournalPath,
		fixture.oldReportTXT,
		fixture.oldReportJSON,
		fixture.oldSupportBundle,
		fixture.oldRestoreEnvFile,
		fixture.restoreStorageDir,
		fixture.restoreBackupDir,
	} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected blocked unattended apply to keep %s: %v", path, err)
		}
	}
}

func TestMaintenanceUnattendedApplyRemovesExpiredArtifacts(t *testing.T) {
	fixture := prepareMaintenanceFixture(t, "dev")
	useJournalClockForTest(t, fixture.fixedNow)

	apply := executeCLIWithOptions(
		[]testAppOption{withFixedTestRuntime(fixture.fixedNow, "op-maintenance-unattended-apply-1")},
		"--journal-dir", fixture.journalDir,
		"--json",
		"maintenance",
		"--scope", "dev",
		"--project-dir", fixture.projectDir,
		"--journal-keep-days", "30",
		"--journal-keep-last", "10",
		"--unattended",
		"--apply",
		"--allow-unattended-apply",
	)
	if apply.ExitCode != exitcode.OK {
		t.Fatalf("expected success exit code, got %d\nstdout=%s\nstderr=%s", apply.ExitCode, apply.Stdout, apply.Stderr)
	}

	var applyObj map[string]any
	if err := json.Unmarshal([]byte(apply.Stdout), &applyObj); err != nil {
		t.Fatal(err)
	}
	if outcome := requireJSONPath(t, applyObj, "details", "outcome"); outcome != "apply_removed_items" {
		t.Fatalf("unexpected outcome: %v", outcome)
	}
	if removed, _ := requireJSONPath(t, applyObj, "details", "removed_items").(float64); int(removed) != 7 {
		t.Fatalf("unexpected removed_items: %v", removed)
	}
	if candidates, _ := requireJSONPath(t, applyObj, "details", "candidate_items").(float64); int(candidates) != 7 {
		t.Fatalf("unexpected candidate_items: %v", candidates)
	}

	for _, path := range []string{
		fixture.oldJournalPath,
		fixture.oldReportTXT,
		fixture.oldReportJSON,
		fixture.oldSupportBundle,
		fixture.oldRestoreEnvFile,
		fixture.restoreStorageDir,
		fixture.restoreBackupDir,
	} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("expected unattended apply to remove %s, got err=%v", path, err)
		}
	}

	for _, path := range []string{
		fixture.recentReportTXT,
		fixture.recentReportJSON,
		fixture.recentSupportBundle,
		fixture.recentRestoreEnvFile,
	} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected unattended apply to keep %s: %v", path, err)
		}
	}

	noop := executeCLIWithOptions(
		[]testAppOption{withFixedTestRuntime(fixture.fixedNow, "op-maintenance-unattended-apply-2")},
		"--journal-dir", fixture.journalDir,
		"--json",
		"maintenance",
		"--scope", "dev",
		"--project-dir", fixture.projectDir,
		"--journal-keep-days", "30",
		"--journal-keep-last", "10",
		"--unattended",
		"--apply",
		"--allow-unattended-apply",
	)
	if noop.ExitCode != exitcode.OK {
		t.Fatalf("expected success exit code on noop apply, got %d\nstdout=%s\nstderr=%s", noop.ExitCode, noop.Stdout, noop.Stderr)
	}

	var noopObj map[string]any
	if err := json.Unmarshal([]byte(noop.Stdout), &noopObj); err != nil {
		t.Fatal(err)
	}
	if outcome := requireJSONPath(t, noopObj, "details", "outcome"); outcome != "nothing_to_do" {
		t.Fatalf("unexpected noop outcome: %v", outcome)
	}
	if removed, _ := requireJSONPath(t, noopObj, "details", "removed_items").(float64); int(removed) != 0 {
		t.Fatalf("unexpected noop removed_items: %v", removed)
	}
	if candidates, _ := requireJSONPath(t, noopObj, "details", "candidate_items").(float64); int(candidates) != 0 {
		t.Fatalf("unexpected noop candidate_items: %v", candidates)
	}
}

func TestSchema_Maintenance_JSON_UnattendedApplyPartialFailure(t *testing.T) {
	fixture := prepareMaintenanceFixture(t, "dev")
	useJournalClockForTest(t, fixture.fixedNow)

	envFile := filepath.Join(fixture.projectDir, ".env.dev")
	raw, err := os.ReadFile(envFile)
	if err != nil {
		t.Fatal(err)
	}
	updated := strings.Replace(string(raw), "REPORT_RETENTION_DAYS=30", "REPORT_RETENTION_DAYS=invalid", 1)
	if updated == string(raw) {
		t.Fatalf("expected env fixture to contain REPORT_RETENTION_DAYS")
	}
	if err := os.WriteFile(envFile, []byte(updated), 0o600); err != nil {
		t.Fatal(err)
	}

	outcome := executeCLIWithOptions(
		[]testAppOption{withFixedTestRuntime(fixture.fixedNow, "op-maintenance-unattended-partial-1")},
		"--journal-dir", fixture.journalDir,
		"--json",
		"maintenance",
		"--scope", "dev",
		"--project-dir", fixture.projectDir,
		"--journal-keep-days", "30",
		"--journal-keep-last", "2",
		"--unattended",
		"--apply",
		"--allow-unattended-apply",
	)
	if outcome.ExitCode != exitcode.ValidationError {
		t.Fatalf("expected validation exit code %d, got %d\nstdout=%s\nstderr=%s", exitcode.ValidationError, outcome.ExitCode, outcome.Stdout, outcome.Stderr)
	}

	var obj map[string]any
	if err := json.Unmarshal([]byte(outcome.Stdout), &obj); err != nil {
		t.Fatal(err)
	}

	if code := requireJSONPath(t, obj, "error", "code"); code != "maintenance_failed" {
		t.Fatalf("unexpected error code: %v", code)
	}
	if outcomeValue := requireJSONPath(t, obj, "details", "outcome"); outcomeValue != "partial_failure" {
		t.Fatalf("unexpected outcome: %v", outcomeValue)
	}
	if removed, _ := requireJSONPath(t, obj, "details", "removed_items").(float64); int(removed) != 5 {
		t.Fatalf("unexpected removed_items: %v", removed)
	}
	if failedSections := requireJSONPath(t, obj, "details", "failed_sections").([]any); len(failedSections) != 1 || failedSections[0] != "reports" {
		t.Fatalf("unexpected failed sections: %#v", failedSections)
	}

	foundFailedReports := false
	for _, rawItem := range requireJSONPath(t, obj, "items").([]any) {
		item := rawItem.(map[string]any)
		if item["code"] == "reports" && item["status"] == "failed" {
			foundFailedReports = true
		}
	}
	if !foundFailedReports {
		t.Fatalf("expected failed reports section in items: %#v", requireJSONPath(t, obj, "items"))
	}
}
