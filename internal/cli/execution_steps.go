package cli

import (
	"fmt"
	"io"
	"strings"

	"github.com/lazuale/espocrm-ops/internal/contract/result"
	restoreusecase "github.com/lazuale/espocrm-ops/internal/usecase/restore"
	rollbackusecase "github.com/lazuale/espocrm-ops/internal/usecase/rollback"
	updateusecase "github.com/lazuale/espocrm-ops/internal/usecase/update"
)

type stepRenderOptions struct {
	Title            string
	IgnoreUnexpected bool
	StatusText       func(string) string
}

func renderWarningsBlock(w io.Writer, warnings []string) error {
	if len(warnings) == 0 {
		return nil
	}

	if _, err := fmt.Fprintln(w, "\nWarnings:"); err != nil {
		return err
	}
	for _, warning := range warnings {
		if _, err := fmt.Fprintf(w, "- %s\n", warning); err != nil {
			return err
		}
	}

	return nil
}

func renderStepItemsBlock(w io.Writer, items []any, extract func(any) (result.SectionItem, error), opts stepRenderOptions) error {
	if len(items) == 0 {
		return nil
	}

	if opts.StatusText == nil {
		opts.StatusText = func(status string) string { return strings.ToUpper(status) }
	}
	if opts.Title == "" {
		opts.Title = "Steps"
	}

	if _, err := fmt.Fprintf(w, "\n%s:\n", opts.Title); err != nil {
		return err
	}
	for _, rawItem := range items {
		item, err := extract(rawItem)
		if err != nil {
			if opts.IgnoreUnexpected {
				continue
			}
			return err
		}
		if _, err := fmt.Fprintf(w, "[%s] %s\n", opts.StatusText(item.Status), item.Summary); err != nil {
			return err
		}
		if strings.TrimSpace(item.Details) != "" {
			if _, err := fmt.Fprintf(w, "  %s\n", item.Details); err != nil {
				return err
			}
		}
		if strings.TrimSpace(item.Action) != "" {
			if _, err := fmt.Fprintf(w, "  Action: %s\n", item.Action); err != nil {
				return err
			}
		}
	}

	return nil
}

func restoreExecutionItems(steps []restoreusecase.ExecuteStep) []any {
	return mapExecutionItems(steps, func(step restoreusecase.ExecuteStep) result.RestoreExecutionItem {
		return result.RestoreExecutionItem{
			SectionItem: newSectionItem(step.Code, step.Status, step.Summary, step.Details, step.Action),
		}
	})
}

func restoreDrillItems(steps []restoreusecase.DrillStep) []any {
	return mapExecutionItems(steps, func(step restoreusecase.DrillStep) result.RestoreDrillItem {
		return result.RestoreDrillItem{
			SectionItem: newSectionItem(step.Code, step.Status, step.Summary, step.Details, step.Action),
		}
	})
}

func updateItems(steps []updateusecase.ExecuteStep) []any {
	return mapExecutionItems(steps, func(step updateusecase.ExecuteStep) result.UpdateItem {
		return result.UpdateItem{
			SectionItem: newSectionItem(step.Code, step.Status, step.Summary, step.Details, step.Action),
		}
	})
}

func updatePlanItems(steps []updateusecase.PlanStep) []any {
	return mapExecutionItems(steps, func(step updateusecase.PlanStep) result.UpdatePlanItem {
		return result.UpdatePlanItem{
			SectionItem: newSectionItem(step.Code, step.Status, step.Summary, step.Details, step.Action),
		}
	})
}

func rollbackItems(steps []rollbackusecase.ExecuteStep) []any {
	return mapExecutionItems(steps, func(step rollbackusecase.ExecuteStep) result.RollbackItem {
		return result.RollbackItem{
			SectionItem: newSectionItem(step.Code, step.Status, step.Summary, step.Details, step.Action),
		}
	})
}

func rollbackPlanItems(steps []rollbackusecase.PlanStep) []any {
	return mapExecutionItems(steps, func(step rollbackusecase.PlanStep) result.RollbackPlanItem {
		return result.RollbackPlanItem{
			SectionItem: newSectionItem(step.Code, step.Status, step.Summary, step.Details, step.Action),
		}
	})
}

func newSectionItem(code, status, summary, details, action string) result.SectionItem {
	return result.SectionItem{
		Code:    code,
		Status:  status,
		Summary: summary,
		Details: details,
		Action:  action,
	}
}

func mapExecutionItems[S any, I any](steps []S, build func(S) I) []any {
	items := make([]any, 0, len(steps))
	for _, step := range steps {
		items = append(items, build(step))
	}
	return items
}

func restoreExecutionItem(raw any) (result.SectionItem, error) {
	item, ok := raw.(result.RestoreExecutionItem)
	if !ok {
		return result.SectionItem{}, fmt.Errorf("unexpected restore item type %T", raw)
	}
	return item.SectionItem, nil
}

func restoreDrillItem(raw any) (result.SectionItem, error) {
	item, ok := raw.(result.RestoreDrillItem)
	if !ok {
		return result.SectionItem{}, fmt.Errorf("unexpected restore-drill item type %T", raw)
	}
	return item.SectionItem, nil
}

func updateItem(raw any) (result.SectionItem, error) {
	item, ok := raw.(result.UpdateItem)
	if !ok {
		return result.SectionItem{}, fmt.Errorf("unexpected update item type %T", raw)
	}
	return item.SectionItem, nil
}

func updatePlanItem(raw any) (result.SectionItem, error) {
	item, ok := raw.(result.UpdatePlanItem)
	if !ok {
		return result.SectionItem{}, fmt.Errorf("unexpected update-plan item type %T", raw)
	}
	return item.SectionItem, nil
}

func rollbackItem(raw any) (result.SectionItem, error) {
	item, ok := raw.(result.RollbackItem)
	if !ok {
		return result.SectionItem{}, fmt.Errorf("unexpected rollback item type %T", raw)
	}
	return item.SectionItem, nil
}

func rollbackPlanItem(raw any) (result.SectionItem, error) {
	item, ok := raw.(result.RollbackPlanItem)
	if !ok {
		return result.SectionItem{}, fmt.Errorf("unexpected rollback-plan item type %T", raw)
	}
	return item.SectionItem, nil
}

func upperStatusText(status string) string {
	return strings.ToUpper(status)
}

func upperSpacedStatusText(status string) string {
	return strings.ToUpper(strings.ReplaceAll(status, "_", " "))
}
