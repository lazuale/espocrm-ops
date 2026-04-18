package journal

import "testing"

func TestExplainBuildsRollbackReport(t *testing.T) {
	entry := Entry{
		OperationID:  "op-rollback-1",
		Command:      "rollback",
		StartedAt:    "2026-04-18T13:00:00Z",
		FinishedAt:   "2026-04-18T13:00:03Z",
		OK:           false,
		Message:      "rollback failed",
		ErrorCode:    "rollback_failed",
		ErrorMessage: "rollback target selection failed",
		Details: map[string]any{
			"scope":          "prod",
			"selection_mode": "auto_latest_valid",
		},
		Artifacts: map[string]any{
			"selected_prefix": "espocrm-prod",
			"selected_stamp":  "2026-04-18_10-00-00",
			"manifest_json":   "/tmp/manifest.json",
			"db_backup":       "/tmp/db.sql.gz",
			"files_backup":    "/tmp/files.tar.gz",
		},
		Items: []any{
			map[string]any{
				"code":    "target_selection",
				"status":  "failed",
				"summary": "Rollback target selection failed",
				"details": "could not find a valid backup set",
				"action":  "Resolve the rollback target selection error before rerunning rollback.",
			},
			map[string]any{
				"code":    "runtime_prepare",
				"status":  "not_run",
				"summary": "Runtime preparation did not run because rollback target selection failed",
			},
		},
	}

	report := Explain(entry)

	if report.Scope != "prod" {
		t.Fatalf("unexpected scope: %q", report.Scope)
	}
	if report.DurationMS != 3000 {
		t.Fatalf("unexpected duration: %d", report.DurationMS)
	}
	if report.Target == nil || report.Target.Prefix != "espocrm-prod" {
		t.Fatalf("unexpected target: %#v", report.Target)
	}
	if len(report.Steps) != 2 {
		t.Fatalf("unexpected steps: %#v", report.Steps)
	}
	if report.Steps[1].Status != "blocked" {
		t.Fatalf("expected blocked downstream step, got %#v", report.Steps[1])
	}
	if report.Counts.Failed != 1 || report.Counts.Blocked != 1 {
		t.Fatalf("unexpected counts: %#v", report.Counts)
	}
	if report.Failure == nil {
		t.Fatal("expected failure attribution")
	}
	if report.Failure.StepCode != "target_selection" {
		t.Fatalf("unexpected failure attribution: %#v", report.Failure)
	}
	if report.Recovery == nil {
		t.Fatal("expected recovery evaluation")
	}
	if report.Recovery.Decision != RecoveryDecisionRetryFromStart {
		t.Fatalf("expected retry-from-start recovery, got %#v", report.Recovery)
	}
}

func TestExplainBuildsUpdateRecoveryResume(t *testing.T) {
	entry := Entry{
		OperationID:  "op-update-2",
		Command:      "update",
		StartedAt:    "2026-04-19T08:00:00Z",
		FinishedAt:   "2026-04-19T08:00:05Z",
		OK:           false,
		ErrorCode:    "update_failed",
		ErrorMessage: "http probe failed",
		Details: map[string]any{
			"scope": "prod",
		},
		Items: []any{
			map[string]any{
				"code":    "runtime_apply",
				"status":  "completed",
				"summary": "Runtime apply completed",
			},
			map[string]any{
				"code":    "runtime_readiness",
				"status":  "failed",
				"summary": "Runtime readiness checks failed",
			},
		},
	}

	report := Explain(entry)

	if report.Recovery == nil {
		t.Fatal("expected recovery evaluation")
	}
	if report.Recovery.Decision != RecoveryDecisionResumeFromCheckpoint {
		t.Fatalf("expected resume decision, got %#v", report.Recovery)
	}
	if report.Recovery.ResumeStep != "runtime_readiness" {
		t.Fatalf("unexpected resume step: %#v", report.Recovery)
	}
	if !report.Recovery.Retryable || !report.Recovery.Resumable {
		t.Fatalf("expected retryable+resumable recovery, got %#v", report.Recovery)
	}
}

func TestExplainBuildsRollbackRecoveryRefusalAfterFilesRestoreFailure(t *testing.T) {
	entry := Entry{
		OperationID:  "op-rollback-2",
		Command:      "rollback",
		StartedAt:    "2026-04-19T09:00:00Z",
		FinishedAt:   "2026-04-19T09:00:05Z",
		OK:           false,
		ErrorCode:    "rollback_failed",
		ErrorMessage: "files restore failed",
		Details: map[string]any{
			"scope":                    "prod",
			"requested_selection_mode": "explicit",
		},
		Artifacts: map[string]any{
			"requested_db_backup":    "/tmp/db.sql.gz",
			"requested_files_backup": "/tmp/files.tar.gz",
			"db_backup":              "/tmp/db.sql.gz",
			"files_backup":           "/tmp/files.tar.gz",
		},
		Items: []any{
			map[string]any{
				"code":    "files_restore",
				"status":  "failed",
				"summary": "Files restore failed",
			},
			map[string]any{
				"code":    "runtime_return",
				"status":  "not_run",
				"summary": "Contour return did not run because the files restore failed",
			},
		},
	}

	report := Explain(entry)

	if report.Recovery == nil {
		t.Fatal("expected recovery evaluation")
	}
	if report.Recovery.Decision != RecoveryDecisionRefused {
		t.Fatalf("expected refused recovery, got %#v", report.Recovery)
	}
	if report.Recovery.Retryable || report.Recovery.Resumable {
		t.Fatalf("expected non-retryable refused recovery, got %#v", report.Recovery)
	}
}
