package cli

import (
	"encoding/json"
	"testing"

	"github.com/lazuale/espocrm-ops/internal/contract/exitcode"
)

func TestSchema_Overview_JSON_FailureIncludesFailedAndOmittedSections(t *testing.T) {
	fixture := prepareSupportBundleFixture(t, "dev")
	prependSupportBundleUnavailableDocker(t)

	outcome := executeCLIWithOptions(
		[]testAppOption{withFixedTestRuntime(fixture.fixedNow, "op-overview-fail-1")},
		"--journal-dir", fixture.journalDir,
		"--json",
		"overview",
		"--scope", "dev",
		"--project-dir", fixture.projectDir,
	)

	if outcome.ExitCode != exitcode.ValidationError {
		t.Fatalf("expected validation exit code %d, got %d\nstdout=%s\nstderr=%s", exitcode.ValidationError, outcome.ExitCode, outcome.Stdout, outcome.Stderr)
	}
	if outcome.Stderr != "" {
		t.Fatalf("expected empty stderr, got %q", outcome.Stderr)
	}

	var obj map[string]any
	if err := json.Unmarshal([]byte(outcome.Stdout), &obj); err != nil {
		t.Fatal(err)
	}

	if command := requireJSONPath(t, obj, "command"); command != "overview" {
		t.Fatalf("unexpected command: %v", command)
	}
	if ok, _ := requireJSONPath(t, obj, "ok").(bool); ok {
		t.Fatalf("expected ok=false")
	}
	if code := requireJSONPath(t, obj, "error", "code"); code != "overview_failed" {
		t.Fatalf("unexpected error code: %v", code)
	}

	if failed := requireJSONPath(t, obj, "details", "failed_sections").([]any); len(failed) != 1 || failed[0] != "doctor" {
		t.Fatalf("unexpected failed sections: %#v", failed)
	}
	if omitted := requireJSONPath(t, obj, "details", "omitted_sections").([]any); len(omitted) != 1 || omitted[0] != "runtime" {
		t.Fatalf("unexpected omitted sections: %#v", omitted)
	}

	items := requireJSONPath(t, obj, "items").([]any)
	if len(items) != 5 {
		t.Fatalf("expected 5 overview sections, got %d", len(items))
	}

	statusByCode := map[string]string{}
	for _, rawItem := range items {
		item := rawItem.(map[string]any)
		code := item["code"].(string)
		statusByCode[code] = item["status"].(string)
	}

	if statusByCode["context"] != "included" {
		t.Fatalf("expected context included, got %q", statusByCode["context"])
	}
	if statusByCode["doctor"] != "failed" {
		t.Fatalf("expected doctor failed, got %q", statusByCode["doctor"])
	}
	if statusByCode["runtime"] != "omitted" {
		t.Fatalf("expected runtime omitted, got %q", statusByCode["runtime"])
	}
	if statusByCode["latest_operation"] != "included" {
		t.Fatalf("expected latest_operation included, got %q", statusByCode["latest_operation"])
	}
	if statusByCode["backup"] != "included" {
		t.Fatalf("expected backup included, got %q", statusByCode["backup"])
	}
}
