package cli

import (
	"encoding/json"
	"testing"
)

type jsonFieldTransform func(any) any

func decodeJSONMap(t *testing.T, raw []byte) map[string]any {
	t.Helper()

	var obj map[string]any
	if err := json.Unmarshal(raw, &obj); err != nil {
		t.Fatalf("invalid json output: %v\n%s", err, string(raw))
	}

	return obj
}

func encodeJSONMap(t *testing.T, obj map[string]any) []byte {
	t.Helper()

	out, err := json.Marshal(obj)
	if err != nil {
		t.Fatal(err)
	}

	return out
}

func mergeReplacements(groups ...map[string]string) map[string]string {
	out := map[string]string{}
	for _, group := range groups {
		for source, target := range group {
			out[source] = target
		}
	}
	return out
}

func normalizeJSONValue(value any, replacements map[string]string, transforms map[string]jsonFieldTransform) any {
	switch typed := value.(type) {
	case map[string]any:
		for key, item := range typed {
			if transform, ok := transforms[key]; ok {
				typed[key] = transform(item)
				continue
			}
			typed[key] = normalizeJSONValue(item, replacements, transforms)
		}
		return typed
	case []any:
		for idx, item := range typed {
			typed[idx] = normalizeJSONValue(item, replacements, transforms)
		}
		return typed
	case string:
		return replaceKnownPaths(typed, replacements)
	default:
		return value
	}
}

func operatorRuntimeLockReplacements(obj map[string]any) map[string]string {
	replacements := map[string]string{}

	items, ok := obj["items"].([]any)
	if !ok {
		return replacements
	}

	for _, rawItem := range items {
		item, ok := rawItem.(map[string]any)
		if !ok || item["code"] != "runtime" {
			continue
		}

		runtimeData, ok := item["runtime"].(map[string]any)
		if !ok {
			continue
		}

		addLockReplacement := func(key, placeholder string) {
			lock, ok := runtimeData[key].(map[string]any)
			if !ok {
				return
			}
			metadata, ok := lock["metadata_path"].(string)
			if ok && metadata != "" {
				replacements[metadata] = placeholder
			}
		}

		addLockReplacement("shared_operation_lock", "REPLACE_SHARED_OPERATION_LOCK")
		addLockReplacement("maintenance_lock", "REPLACE_MAINTENANCE_LOCK")
	}

	return replacements
}
