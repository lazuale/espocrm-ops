package cli

import (
	"bytes"
	"testing"

	"github.com/lazuale/espocrm-ops/internal/contract/result"
	restoreusecase "github.com/lazuale/espocrm-ops/internal/usecase/restore"
	rollbackusecase "github.com/lazuale/espocrm-ops/internal/usecase/rollback"
	updateusecase "github.com/lazuale/espocrm-ops/internal/usecase/update"
)

func TestExecutionStepItemMappersPreserveFields(t *testing.T) {
	want := result.SectionItem{
		Code:    "runtime_apply",
		Status:  "would_run",
		Summary: "Apply runtime changes",
		Details: "Recreate services with new image references",
		Action:  "Run the canonical execution flow",
	}

	tests := []struct {
		name    string
		items   []any
		extract func(any) (result.SectionItem, error)
	}{
		{
			name: "restore execute",
			items: restoreExecutionItems([]restoreusecase.ExecuteStep{{
				Code:    want.Code,
				Status:  want.Status,
				Summary: want.Summary,
				Details: want.Details,
				Action:  want.Action,
			}}),
			extract: restoreExecutionItem,
		},
		{
			name: "restore drill",
			items: restoreDrillItems([]restoreusecase.DrillStep{{
				Code:    want.Code,
				Status:  want.Status,
				Summary: want.Summary,
				Details: want.Details,
				Action:  want.Action,
			}}),
			extract: restoreDrillItem,
		},
		{
			name: "update execute",
			items: updateItems([]updateusecase.ExecuteStep{{
				Code:    want.Code,
				Status:  want.Status,
				Summary: want.Summary,
				Details: want.Details,
				Action:  want.Action,
			}}),
			extract: updateItem,
		},
		{
			name: "update plan",
			items: updatePlanItems([]updateusecase.PlanStep{{
				Code:    want.Code,
				Status:  want.Status,
				Summary: want.Summary,
				Details: want.Details,
				Action:  want.Action,
			}}),
			extract: updatePlanItem,
		},
		{
			name: "rollback execute",
			items: rollbackItems([]rollbackusecase.ExecuteStep{{
				Code:    want.Code,
				Status:  want.Status,
				Summary: want.Summary,
				Details: want.Details,
				Action:  want.Action,
			}}),
			extract: rollbackItem,
		},
		{
			name: "rollback plan",
			items: rollbackPlanItems([]rollbackusecase.PlanStep{{
				Code:    want.Code,
				Status:  want.Status,
				Summary: want.Summary,
				Details: want.Details,
				Action:  want.Action,
			}}),
			extract: rollbackPlanItem,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if len(tt.items) != 1 {
				t.Fatalf("expected 1 item, got %d", len(tt.items))
			}

			got, err := tt.extract(tt.items[0])
			if err != nil {
				t.Fatalf("extract item: %v", err)
			}

			if got != want {
				t.Fatalf("mapped section item mismatch: got %#v want %#v", got, want)
			}
		})
	}
}

func TestRenderStepItemsBlockFormatsSharedExecutionSections(t *testing.T) {
	var buf bytes.Buffer

	err := renderStepItemsBlock(&buf, updatePlanItems([]updateusecase.PlanStep{{
		Code:    "doctor",
		Status:  "would_run",
		Summary: "Run doctor checks",
		Details: "Validate runtime readiness before update",
		Action:  "Resolve blocking doctor failures first",
	}}), updatePlanItem, stepRenderOptions{
		Title:      "Plan",
		StatusText: upperSpacedStatusText,
	})
	if err != nil {
		t.Fatalf("render step items: %v", err)
	}

	want := "\nPlan:\n[WOULD RUN] Run doctor checks\n  Validate runtime readiness before update\n  Action: Resolve blocking doctor failures first\n"
	if buf.String() != want {
		t.Fatalf("rendered block mismatch: got %q want %q", buf.String(), want)
	}
}

func TestRenderStepItemsBlockCanIgnoreUnexpectedItems(t *testing.T) {
	var buf bytes.Buffer

	err := renderStepItemsBlock(&buf, []any{
		result.RestoreExecutionItem{
			SectionItem: result.SectionItem{
				Code:    "restore_db",
				Status:  "completed",
				Summary: "Restore database",
			},
		},
		"unexpected-item",
	}, restoreExecutionItem, stepRenderOptions{
		Title:            "Steps",
		IgnoreUnexpected: true,
		StatusText:       upperStatusText,
	})
	if err != nil {
		t.Fatalf("render step items with ignore unexpected: %v", err)
	}

	want := "\nSteps:\n[COMPLETED] Restore database\n"
	if buf.String() != want {
		t.Fatalf("rendered block mismatch: got %q want %q", buf.String(), want)
	}
}

func TestRenderWarningsBlockFormatsWarnings(t *testing.T) {
	var buf bytes.Buffer

	if err := renderWarningsBlock(&buf, []string{"first warning", "second warning"}); err != nil {
		t.Fatalf("render warnings: %v", err)
	}

	want := "\nWarnings:\n- first warning\n- second warning\n"
	if buf.String() != want {
		t.Fatalf("rendered warnings mismatch: got %q want %q", buf.String(), want)
	}
}
