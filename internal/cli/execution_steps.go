package cli

import (
	"fmt"
	"io"
	"strings"

	restoreusecase "github.com/lazuale/espocrm-ops/internal/app/restore"
	"github.com/lazuale/espocrm-ops/internal/contract/result"
)

type stepRenderOptions struct {
	Title            string
	IgnoreUnexpected bool
	StatusText       func(string) string
}

func renderStepItemsBlock(w io.Writer, items []result.ItemPayload, extract func(result.ItemPayload) (result.SectionItem, error), opts stepRenderOptions) error {
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

func restoreExecutionItems(steps []restoreusecase.ExecuteStep) []result.ItemPayload {
	return mapExecutionItems(steps, func(step restoreusecase.ExecuteStep) result.RestoreItem {
		return result.RestoreItem{
			SectionItem: newSectionItem(step.Code, step.Status, step.Summary, step.Details, step.Action),
		}
	})
}

func newSectionItem(code string, status fmt.Stringer, summary, details, action string) result.SectionItem {
	return result.SectionItem{
		Code:    code,
		Status:  status.String(),
		Summary: summary,
		Details: details,
		Action:  action,
	}
}

func mapExecutionItems[S any, I result.ItemPayload](steps []S, build func(S) I) []result.ItemPayload {
	items := make([]result.ItemPayload, 0, len(steps))
	for _, step := range steps {
		items = append(items, build(step))
	}
	return items
}

func restoreExecutionItem(raw result.ItemPayload) (result.SectionItem, error) {
	item, ok := raw.(result.RestoreItem)
	if !ok {
		return result.SectionItem{}, fmt.Errorf("unexpected restore item type %T", raw)
	}
	return item.SectionItem, nil
}

func migrateExecutionItem(raw result.ItemPayload) (result.SectionItem, error) {
	item, ok := raw.(result.MigrateItem)
	if !ok {
		return result.SectionItem{}, fmt.Errorf("unexpected migrate item type %T", raw)
	}

	return item.SectionItem, nil
}

func upperStatusText(status string) string {
	return strings.ToUpper(status)
}
