package cli

import "testing"

func normalizeOverviewJSON(t *testing.T, raw []byte, fixture supportBundleFixture) []byte {
	t.Helper()

	obj := decodeJSONMap(t, raw)

	replacements := mergeReplacements(map[string]string{
		fixture.projectDir:                   "REPLACE_PROJECT_DIR",
		fixture.projectDir + "/compose.yaml": "REPLACE_COMPOSE_FILE",
		fixture.projectDir + "/.env.dev":     "REPLACE_ENV_FILE",
		fixture.projectDir + "/.env.prod":    "REPLACE_ENV_FILE",
		fixture.backupRoot:                   "REPLACE_BACKUP_ROOT",
	}, operatorRuntimeLockReplacements(obj))

	obj = normalizeJSONValue(obj, replacements, map[string]jsonFieldTransform{
		"age_hours": func(any) any { return float64(0) },
	}).(map[string]any)

	return encodeJSONMap(t, obj)
}
