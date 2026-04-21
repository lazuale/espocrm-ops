package cli

import (
	"encoding/json"
	"fmt"

	operationusecase "github.com/lazuale/espocrm-ops/internal/app/operation"
	"github.com/lazuale/espocrm-ops/internal/contract/result"
)

func journalRecordFromResult(res *result.Result) operationusecase.JournalRecord {
	artifacts, artifactErr := toJSONObjectMap(res.Artifacts)
	if artifactErr != nil {
		res.Warnings = append(res.Warnings, fmt.Sprintf("failed to serialize journal artifacts: %v", artifactErr))
	}

	details, detailsErr := toJSONObjectMap(res.Details)
	if detailsErr != nil {
		res.Warnings = append(res.Warnings, fmt.Sprintf("failed to serialize journal details: %v", detailsErr))
	}

	items, itemsErr := toJSONArray(res.Items)
	if itemsErr != nil {
		res.Warnings = append(res.Warnings, fmt.Sprintf("failed to serialize journal items: %v", itemsErr))
	}

	return operationusecase.JournalRecord{
		DryRun:   res.DryRun,
		Message:  res.Message,
		Warnings: append([]string(nil), res.Warnings...),
		Payload: operationusecase.JournalPayload{
			Artifacts: artifacts,
			Details:   details,
			Items:     items,
		},
	}
}

func applyExecutionCompletion(res *result.Result, completion operationusecase.Completion) {
	res.OK = true
	res.Timing = &result.TimingInfo{
		StartedAt:  completion.StartedAt,
		FinishedAt: completion.FinishedAt,
		DurationMS: completion.DurationMS,
	}
	res.Warnings = completion.Warnings
}

func toJSONObjectMap(v any) (map[string]any, error) {
	if v == nil {
		return nil, nil
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

func toJSONArray(v any) ([]any, error) {
	if v == nil {
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
