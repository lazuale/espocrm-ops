package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/lazuale/espocrm-ops/internal/contract/exitcode"
)

func TestSchema_SupportBundle_JSON_Success(t *testing.T) {
	fixture := prepareSupportBundleFixture(t, "dev")
	prependSupportBundleFakeDocker(t)

	outputPath := filepath.Join(t.TempDir(), "support-bundle.tar.gz")
	outcome := executeCLIWithOptions(
		[]testAppOption{withFixedTestRuntime(fixture.fixedNow, "op-support-bundle-1")},
		"--journal-dir", fixture.journalDir,
		"--json",
		"support-bundle",
		"--scope", "dev",
		"--project-dir", fixture.projectDir,
		"--output", outputPath,
		"--tail", "42",
	)
	if outcome.ExitCode != 0 {
		t.Fatalf("command failed\nstdout=%s\nstderr=%s", outcome.Stdout, outcome.Stderr)
	}

	var obj map[string]any
	if err := json.Unmarshal([]byte(outcome.Stdout), &obj); err != nil {
		t.Fatal(err)
	}

	requireJSONPath(t, obj, "command")
	requireJSONPath(t, obj, "ok")
	requireJSONPath(t, obj, "message")
	requireJSONPath(t, obj, "details", "scope")
	requireJSONPath(t, obj, "details", "bundle_kind")
	requireJSONPath(t, obj, "details", "included_sections")
	requireJSONPath(t, obj, "artifacts", "bundle_path")
	requireJSONPath(t, obj, "items")

	if obj["command"] != "support-bundle" {
		t.Fatalf("unexpected command: %v", obj["command"])
	}
	if ok, _ := obj["ok"].(bool); !ok {
		t.Fatalf("expected ok=true, got %v", obj["ok"])
	}
	if bundleKind := requireJSONPath(t, obj, "details", "bundle_kind"); bundleKind != "support_bundle" {
		t.Fatalf("unexpected bundle kind: %v", bundleKind)
	}
	if tail, _ := requireJSONPath(t, obj, "details", "tail_lines").(float64); int(tail) != 42 {
		t.Fatalf("unexpected tail lines: %v", tail)
	}
	if bundlePath := requireJSONPath(t, obj, "artifacts", "bundle_path"); bundlePath != outputPath {
		t.Fatalf("unexpected bundle path: %v", bundlePath)
	}

	for _, code := range []string{
		"env_redacted",
		"compose_config",
		"compose_ps",
		"compose_logs",
		"docker_version",
		"docker_compose_version",
		"doctor",
		"history",
		"backup_catalog",
		"latest_manifest_json",
		"latest_manifest_txt",
		"bundle_summary",
		"bundle_manifest",
	} {
		assertJSONListContains(t, obj, []string{"details", "included_sections"}, code)
	}

	unpackDir := unpackSupportBundle(t, outputPath)
	for _, base := range []string{
		"env.redacted",
		"compose.config.yaml",
		"compose.ps.txt",
		"compose.logs.txt",
		"docker.version.txt",
		"docker.compose.version.txt",
		"doctor.txt",
		"doctor.json",
		"history.txt",
		"history.json",
		"backup-catalog.txt",
		"backup-catalog.json",
		"latest.manifest.json",
		"latest.manifest.txt",
		"bundle.summary.txt",
		"bundle.manifest.json",
	} {
		if _, err := os.Stat(bundledFileByBaseName(t, unpackDir, base)); err != nil {
			t.Fatalf("expected bundled file %s: %v", base, err)
		}
	}

	for _, secret := range []string{"root-secret", "db-secret", "admin-secret", "token-secret"} {
		assertBundleDoesNotContain(t, unpackDir, secret)
	}
	if raw, err := os.ReadFile(bundledFileByBaseName(t, unpackDir, "compose.config.yaml")); err != nil {
		t.Fatal(err)
	} else if !strings.Contains(string(raw), "<redacted>") {
		t.Fatalf("expected redacted compose config, got:\n%s", string(raw))
	}

	manifestRaw, err := os.ReadFile(bundledFileByBaseName(t, unpackDir, "bundle.manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest map[string]any
	if err := json.Unmarshal(manifestRaw, &manifest); err != nil {
		t.Fatal(err)
	}
	if manifest["bundle_kind"] != "support_bundle" {
		t.Fatalf("unexpected bundled manifest kind: %v", manifest["bundle_kind"])
	}
	assertJSONListContains(t, manifest, []string{"included_sections"}, "history")
}

func TestSchema_SupportBundle_JSON_Success_WithOmittedSections(t *testing.T) {
	fixture := prepareSupportBundleFixture(t, "dev")
	prependSupportBundleUnavailableDocker(t)

	if err := os.Remove(fixture.backupSet.ManifestJSON); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(fixture.backupSet.ManifestTXT); err != nil {
		t.Fatal(err)
	}

	outputPath := filepath.Join(t.TempDir(), "support-bundle-omitted.tar.gz")
	outcome := executeCLIWithOptions(
		[]testAppOption{withFixedTestRuntime(fixture.fixedNow, "op-support-bundle-omit-1")},
		"--journal-dir", fixture.journalDir,
		"--json",
		"support-bundle",
		"--scope", "dev",
		"--project-dir", fixture.projectDir,
		"--output", outputPath,
	)
	if outcome.ExitCode != 0 {
		t.Fatalf("command failed\nstdout=%s\nstderr=%s", outcome.Stdout, outcome.Stderr)
	}

	var obj map[string]any
	if err := json.Unmarshal([]byte(outcome.Stdout), &obj); err != nil {
		t.Fatal(err)
	}
	if ok, _ := obj["ok"].(bool); !ok {
		t.Fatalf("expected ok=true with omitted sections, got %v", obj["ok"])
	}

	for _, code := range []string{
		"compose_config",
		"compose_ps",
		"compose_logs",
		"docker_version",
		"docker_compose_version",
		"latest_manifest_json",
		"latest_manifest_txt",
	} {
		assertJSONListContains(t, obj, []string{"details", "omitted_sections"}, code)
	}
	for _, code := range []string{"doctor", "history", "backup_catalog", "bundle_summary", "bundle_manifest"} {
		assertJSONListContains(t, obj, []string{"details", "included_sections"}, code)
	}

	unpackDir := unpackSupportBundle(t, outputPath)
	manifestRaw, err := os.ReadFile(bundledFileByBaseName(t, unpackDir, "bundle.manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest map[string]any
	if err := json.Unmarshal(manifestRaw, &manifest); err != nil {
		t.Fatal(err)
	}
	assertJSONListContains(t, manifest, []string{"omitted_sections"}, "compose_logs")
	assertJSONListContains(t, manifest, []string{"omitted_sections"}, "latest_manifest_json")
}

func TestSchema_SupportBundle_JSON_Failure_MissingEnv(t *testing.T) {
	tmp := t.TempDir()
	projectDir := filepath.Join(tmp, "project")
	journalDir := filepath.Join(tmp, "journal")
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, "compose.yaml"), []byte("services: {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	outcome := executeCLIWithOptions(
		[]testAppOption{withFixedTestRuntime(testTime(), "op-support-bundle-fail-1")},
		"--journal-dir", journalDir,
		"--json",
		"support-bundle",
		"--scope", "dev",
		"--project-dir", projectDir,
	)
	if outcome.ExitCode != exitcode.ValidationError {
		t.Fatalf("expected validation exit code %d, got %d\nstdout=%s\nstderr=%s", exitcode.ValidationError, outcome.ExitCode, outcome.Stdout, outcome.Stderr)
	}

	var obj map[string]any
	if err := json.Unmarshal([]byte(outcome.Stdout), &obj); err != nil {
		t.Fatal(err)
	}
	if ok, _ := obj["ok"].(bool); ok {
		t.Fatalf("expected ok=false, got %v", obj["ok"])
	}
	if code := requireJSONPath(t, obj, "error", "code"); code != "support_bundle_failed" {
		t.Fatalf("unexpected error code: %v", code)
	}
	items, ok := requireJSONPath(t, obj, "items").([]any)
	if !ok || len(items) != 2 {
		t.Fatalf("expected two failure items, got %#v", obj["items"])
	}
	first, _ := items[0].(map[string]any)
	if first["code"] != "operation_preflight" || first["status"] != "failed" {
		t.Fatalf("unexpected first failure item: %#v", first)
	}
}

func assertBundleDoesNotContain(t *testing.T, root, secret string) {
	t.Helper()

	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if strings.Contains(string(raw), secret) {
			t.Fatalf("bundle leaked secret %q in %s", secret, path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func assertJSONListContains(t *testing.T, obj map[string]any, path []string, want string) {
	t.Helper()

	raw := requireJSONPath(t, obj, path...).([]any)
	for _, item := range raw {
		if item == want {
			return
		}
	}
	t.Fatalf("expected %q in %v, got %#v", want, path, raw)
}

func testTime() time.Time {
	return time.Date(2026, 4, 19, 9, 0, 0, 0, time.UTC)
}
