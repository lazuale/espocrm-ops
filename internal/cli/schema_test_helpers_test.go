package cli

import "testing"

func requireJSONPath(t *testing.T, obj map[string]any, path ...string) any {
	t.Helper()

	var cur any = obj
	for _, key := range path {
		m, ok := cur.(map[string]any)
		if !ok {
			t.Fatalf("path %v: %T is not an object", path, cur)
		}

		next, ok := m[key]
		if !ok {
			t.Fatalf("missing json path: %v", path)
		}
		cur = next
	}

	return cur
}
