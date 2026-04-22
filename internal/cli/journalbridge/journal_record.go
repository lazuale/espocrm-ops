package journalbridge

import (
	"encoding/json"
	"fmt"

	operationtrace "github.com/lazuale/espocrm-ops/internal/app/operationtrace"
	"github.com/lazuale/espocrm-ops/internal/contract/result"
)

func RecordFromResult(res *result.Result) operationtrace.JournalRecord {
	// Journal payload stores JSON-compatible maps, so CLI projects the bounded
	// result payload families through their existing JSON shape here.
	artifacts, artifactErr := projectJournalObject(res.Artifacts)
	appendJournalShapeWarning(res, "artifacts", artifactErr)

	details, detailsErr := projectJournalObject(res.Details)
	appendJournalShapeWarning(res, "details", detailsErr)

	items, itemsErr := projectJournalItems(res.Items)
	appendJournalShapeWarning(res, "items", itemsErr)

	return operationtrace.JournalRecord{
		DryRun:   res.DryRun,
		Message:  res.Message,
		Warnings: append([]string(nil), res.Warnings...),
		Payload: operationtrace.JournalPayload{
			Artifacts: artifacts,
			Details:   details,
			Items:     items,
		},
	}
}

func appendJournalShapeWarning(res *result.Result, field string, err error) {
	if err == nil {
		return
	}

	res.Warnings = append(res.Warnings, fmt.Sprintf("failed to serialize journal %s: %v", field, err))
}

func ApplyExecutionCompletion(res *result.Result, completion operationtrace.Completion) {
	res.OK = true
	res.Timing = &result.TimingInfo{
		StartedAt:  completion.StartedAt,
		FinishedAt: completion.FinishedAt,
		DurationMS: completion.DurationMS,
	}
	res.Warnings = completion.Warnings
}

func projectJournalObject(v any) (map[string]any, error) {
	if v == nil {
		return nil, nil
	}
	if typed, ok := v.(map[string]any); ok {
		return cloneJSONObject(typed), nil
	}

	raw, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("marshal object: %w", err)
	}

	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("unmarshal object into map: %w", err)
	}

	return out, nil
}

func projectJournalItems(v []result.ItemPayload) ([]any, error) {
	if len(v) == 0 {
		return nil, nil
	}

	raw, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("marshal array: %w", err)
	}

	var out []any
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("unmarshal array: %w", err)
	}

	return out, nil
}

func cloneJSONObject(src map[string]any) map[string]any {
	if src == nil {
		return nil
	}

	dst := make(map[string]any, len(src))
	for key, value := range src {
		dst[key] = value
	}

	return dst
}
