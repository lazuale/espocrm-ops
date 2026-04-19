package cli

import (
	"strings"
	"testing"
)

func TestMaintenanceTextShowsCanonicalSectionSummary(t *testing.T) {
	fixture := prepareMaintenanceFixture(t, "dev")
	useJournalClockForTest(t, fixture.fixedNow)

	out, err := runRootCommandWithOptions(
		t,
		[]testAppOption{withFixedTestRuntime(fixture.fixedNow, "op-maintenance-text-1")},
		"--journal-dir", fixture.journalDir,
		"maintenance",
		"--scope", "dev",
		"--project-dir", fixture.projectDir,
		"--journal-keep-days", "30",
		"--journal-keep-last", "2",
		"--unattended",
	)
	if err != nil {
		t.Fatalf("command failed: %v\noutput=%s", err, out)
	}

	for _, fragment := range []string{
		"EspoCRM maintenance run",
		"Mode: preview",
		"Unattended: yes",
		"Outcome: preview_found_cleanup_candidates",
		"Candidate items: 7",
		"Removed items: 0",
		"Included sections: context, journal, reports, support, restore_drill",
		"Context:",
		"Details: Using ",
		"for unattended execution.",
		"Counts: checked=3 candidates=1 kept=1 protected=1 removed=0 failed=0",
		"Counts: checked=4 candidates=2 kept=0 protected=2 removed=0 failed=0",
		"Counts: checked=2 candidates=1 kept=0 protected=1 removed=0 failed=0",
		"Counts: checked=4 candidates=3 kept=1 protected=0 removed=0 failed=0",
		"Journal:",
		"Reports:",
		"Support:",
		"Restore Drill:",
		"Keep days: 30",
		"Keep last: 2",
		"Retention days: 30",
		"Retention days: 14",
		"Retention days: 7",
		"Unattended: yes",
		"KEEP  2026-04-19T08:00:00Z  backup  COMPLETED",
		"PROTECT  2026-04-19T08:30:00Z  restore  FAILED",
		"WOULD_REMOVE  report_txt",
		"WOULD_REMOVE  support_bundle",
		"WOULD_REMOVE  restore_drill_storage_dir",
	} {
		if !strings.Contains(out, fragment) {
			t.Fatalf("expected output to contain %q\n%s", fragment, out)
		}
	}
}
