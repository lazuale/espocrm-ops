package app_test

import (
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	v2app "github.com/lazuale/espocrm-ops/internal/app"
	"github.com/lazuale/espocrm-ops/internal/model"
	"github.com/lazuale/espocrm-ops/internal/store"
)

const updateBackupVerifyAcceptanceEnv = "UPDATE_ACCEPTANCE_BACKUP_VERIFY"

type verifyBackupSet struct {
	Layout model.BackupLayout
}

func TestBackupVerifyV2Acceptance_JSONAndDisk(t *testing.T) {
	cases := []struct {
		id      string
		setup   func(*testing.T) (model.BackupVerifyRequest, string)
		wantOK  bool
		wantErr string
	}{
		{
			id: "BKV-001",
			setup: func(t *testing.T) (model.BackupVerifyRequest, string) {
				root := t.TempDir()
				set := writeVerifyBackupSet(t, root, "espocrm-prod", "2026-04-07_01-00-00")
				return model.BackupVerifyRequest{ManifestPath: set.Layout.ManifestJSON}, root
			},
			wantOK: true,
		},
		{
			id: "BKV-002",
			setup: func(t *testing.T) (model.BackupVerifyRequest, string) {
				root := t.TempDir()
				writeVerifyBackupSet(t, root, "espocrm-prod", "2026-04-07_01-00-00")
				writePartialVerifyManifest(t, root, "espocrm-prod", "2026-04-07_02-00-00")
				return model.BackupVerifyRequest{BackupRoot: root}, root
			},
			wantOK: true,
		},
		{
			id: "BKV-101",
			setup: func(t *testing.T) (model.BackupVerifyRequest, string) {
				root := t.TempDir()
				set := writeVerifyBackupSet(t, root, "espocrm-prod", "2026-04-07_01-00-00")
				return model.BackupVerifyRequest{DBBackupPath: set.Layout.DBArtifact}, root
			},
			wantOK: true,
		},
		{
			id: "BKV-102",
			setup: func(t *testing.T) (model.BackupVerifyRequest, string) {
				root := t.TempDir()
				set := writeVerifyBackupSet(t, root, "espocrm-prod", "2026-04-07_01-00-00")
				return model.BackupVerifyRequest{FilesPath: set.Layout.FilesArtifact}, root
			},
			wantOK: true,
		},
		{
			id: "BKV-301",
			setup: func(t *testing.T) (model.BackupVerifyRequest, string) {
				root := t.TempDir()
				manifestPath := filepath.Join(root, "manifests", "espocrm-prod_2026-04-07_01-00-00.manifest.json")
				mustMkdirAll(t, filepath.Dir(manifestPath))
				mustWriteFile(t, manifestPath, "{")
				return model.BackupVerifyRequest{ManifestPath: manifestPath}, root
			},
			wantOK:  false,
			wantErr: model.ManifestInvalidCode,
		},
		{
			id: "BKV-302",
			setup: func(t *testing.T) (model.BackupVerifyRequest, string) {
				root := t.TempDir()
				set := writeVerifyBackupSet(t, root, "espocrm-prod", "2026-04-07_01-00-00")
				mustWriteFile(t, set.Layout.DBChecksum, strings.Repeat("0", 64)+"  "+filepath.Base(set.Layout.DBArtifact)+"\n")
				return model.BackupVerifyRequest{ManifestPath: set.Layout.ManifestJSON}, root
			},
			wantOK:  false,
			wantErr: model.BackupVerifyFailedCode,
		},
		{
			id: "BKV-303",
			setup: func(t *testing.T) (model.BackupVerifyRequest, string) {
				root := t.TempDir()
				set := writeVerifyBackupSet(t, root, "espocrm-prod", "2026-04-07_01-00-00")
				writeGzipFileForVerify(t, set.Layout.FilesArtifact, []byte("not a tar stream"))
				rewriteVerifyChecksums(t, set)
				return model.BackupVerifyRequest{ManifestPath: set.Layout.ManifestJSON}, root
			},
			wantOK:  false,
			wantErr: model.BackupVerifyFailedCode,
		},
		{
			id: "BKV-304",
			setup: func(t *testing.T) (model.BackupVerifyRequest, string) {
				root := t.TempDir()
				set := writeVerifyBackupSet(t, root, "espocrm-prod", "2026-04-07_01-00-00")
				if err := os.Remove(set.Layout.DBArtifact); err != nil {
					t.Fatal(err)
				}
				return model.BackupVerifyRequest{ManifestPath: set.Layout.ManifestJSON}, root
			},
			wantOK:  false,
			wantErr: model.BackupVerifyFailedCode,
		},
		{
			id: "BKV-305",
			setup: func(t *testing.T) (model.BackupVerifyRequest, string) {
				root := t.TempDir()
				manifestPath := writePartialVerifyManifest(t, root, "espocrm-prod", "2026-04-07_01-00-00")
				return model.BackupVerifyRequest{ManifestPath: manifestPath}, root
			},
			wantOK:  false,
			wantErr: model.ManifestInvalidCode,
		},
		{
			id: "BKV-401",
			setup: func(t *testing.T) (model.BackupVerifyRequest, string) {
				root := t.TempDir()
				writePartialVerifyManifest(t, root, "espocrm-prod", "2026-04-07_01-00-00")
				return model.BackupVerifyRequest{BackupRoot: root}, root
			},
			wantOK:  false,
			wantErr: model.BackupVerifyFailedCode,
		},
	}

	service := v2app.NewBackupVerifyService(v2app.BackupVerifyDependencies{Store: store.FileStore{}})
	for _, tc := range cases {
		tc := tc
		t.Run(tc.id, func(t *testing.T) {
			req, root := tc.setup(t)
			before := collectVerifyTreeEntries(t, root)

			result, err := service.VerifyBackup(context.Background(), req)
			if tc.wantOK && err != nil {
				t.Fatalf("VerifyBackup returned error: %v", err)
			}
			if !tc.wantOK && err == nil {
				t.Fatal("expected verification error")
			}
			if result.OK != tc.wantOK {
				t.Fatalf("unexpected ok: %+v", result)
			}
			if tc.wantErr != "" {
				if result.Error == nil || result.Error.Code != tc.wantErr {
					t.Fatalf("expected error code %s, got %+v", tc.wantErr, result.Error)
				}
			}

			normalizedJSON := normalizeBackupVerifyAcceptanceJSON(t, result)
			assertOrWriteBackupVerifyAcceptanceGoldenJSON(t, normalizedJSON, filepath.Join("golden", "json", "v2_"+tc.id+".json"))

			snapshot := marshalBackupVerifyAcceptanceJSON(t, map[string]any{
				"process_exit_code": result.ProcessExitCode,
				"before_entries":    before,
				"after_entries":     collectVerifyTreeEntries(t, root),
			})
			assertOrWriteBackupVerifyAcceptanceGoldenJSON(t, snapshot, filepath.Join("golden", "disk", "v2_"+tc.id+".json"))
		})
	}
}

func writeVerifyBackupSet(t *testing.T, root, prefix, stamp string) verifyBackupSet {
	t.Helper()

	layout := model.NewBackupLayoutForStamp(root, prefix, stamp)
	mustMkdirAll(t, filepath.Dir(layout.DBArtifact), filepath.Dir(layout.FilesArtifact), filepath.Dir(layout.ManifestJSON))

	if err := os.WriteFile(layout.DBArtifact, gzipBytes("select 1;\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(layout.FilesArtifact, tarGzBytes(map[string]string{"espo/file.txt": "hello\n"}), 0o644); err != nil {
		t.Fatal(err)
	}
	rewriteVerifyChecksums(t, verifyBackupSet{Layout: layout})

	return verifyBackupSet{Layout: layout}
}

func mustMkdirAll(t *testing.T, paths ...string) {
	t.Helper()

	for _, path := range paths {
		if err := os.MkdirAll(path, 0o755); err != nil {
			t.Fatal(err)
		}
	}
}

func mustWriteFile(t *testing.T, path, body string) {
	t.Helper()

	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func rewriteVerifyChecksums(t *testing.T, set verifyBackupSet) {
	t.Helper()

	dbChecksum := sha256OfPath(t, set.Layout.DBArtifact)
	filesChecksum := sha256OfPath(t, set.Layout.FilesArtifact)
	mustWriteFile(t, set.Layout.DBChecksum, dbChecksum+"  "+filepath.Base(set.Layout.DBArtifact)+"\n")
	mustWriteFile(t, set.Layout.FilesChecksum, filesChecksum+"  "+filepath.Base(set.Layout.FilesArtifact)+"\n")

	createdAt, err := model.ParseStamp(set.Layout.Stamp)
	if err != nil {
		t.Fatal(err)
	}
	manifest := model.CompleteManifest{
		Version:    1,
		Scope:      "prod",
		CreatedAt:  createdAt.UTC().Format(time.RFC3339),
		Contour:    "prod",
		EnvFile:    "prod.env",
		MariaDBTag: "10.11",
		Artifacts: model.ManifestArtifacts{
			DBBackup:    filepath.Base(set.Layout.DBArtifact),
			FilesBackup: filepath.Base(set.Layout.FilesArtifact),
		},
		Checksums: model.ManifestChecksums{
			DBBackup:    dbChecksum,
			FilesBackup: filesChecksum,
		},
		DBBackupCreated:        true,
		FilesBackupCreated:     true,
		ComposeProject:         "espocrm-prod",
		EspoCRMImage:           "espocrm:8",
		RetentionDays:          7,
		ConsistentSnapshot:     true,
		AppServicesWereRunning: true,
	}
	raw, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(set.Layout.ManifestJSON, append(raw, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
}

func writePartialVerifyManifest(t *testing.T, root, prefix, stamp string) string {
	t.Helper()

	layout := model.NewBackupLayoutForStamp(root, prefix, stamp)
	mustMkdirAll(t, filepath.Dir(layout.ManifestJSON))
	writeJSONFileForVerify(t, layout.ManifestJSON, map[string]any{
		"version":              1,
		"scope":                "prod",
		"created_at":           "2026-04-07T01:00:00Z",
		"db_backup_created":    true,
		"files_backup_created": false,
		"artifacts": map[string]any{
			"db_backup":    filepath.Base(layout.DBArtifact),
			"files_backup": "",
		},
		"checksums": map[string]any{
			"db_backup":    strings.Repeat("a", 64),
			"files_backup": "",
		},
	})
	return layout.ManifestJSON
}

func writeGzipFileForVerify(t *testing.T, path string, body []byte) {
	t.Helper()

	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer closeVerifyTestResource(t, "verify gzip file", file)
	writer := gzip.NewWriter(file)
	if _, err := writer.Write(body); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
}

func closeVerifyTestResource(t *testing.T, label string, closer interface{ Close() error }) {
	t.Helper()

	if err := closer.Close(); err != nil {
		t.Fatalf("close %s: %v", label, err)
	}
}

func writeJSONFileForVerify(t *testing.T, path string, value any) {
	t.Helper()

	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(raw, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
}

func normalizeBackupVerifyAcceptanceJSON(t *testing.T, result model.BackupVerifyResult) []byte {
	t.Helper()

	raw := marshalBackupVerifyAcceptanceJSON(t, result)
	obj := map[string]any{}
	if err := json.Unmarshal(raw, &obj); err != nil {
		t.Fatal(err)
	}
	artifacts, _ := obj["artifacts"].(map[string]any)
	replacements := map[string]string{}
	for _, key := range []string{"backup_root", "manifest", "db_backup", "db_checksum", "files_backup", "files_checksum"} {
		value, _ := artifacts[key].(string)
		if strings.TrimSpace(value) == "" {
			continue
		}
		placeholder := "REPLACE_" + strings.ToUpper(key)
		artifacts[key] = placeholder
		replacements[value] = placeholder
	}
	if errObj, ok := obj["error"].(map[string]any); ok {
		delete(errObj, "message")
	}
	if items, ok := obj["items"].([]any); ok {
		normalized := make([]any, 0, len(items))
		for _, rawItem := range items {
			item, ok := rawItem.(map[string]any)
			if !ok {
				continue
			}
			normalized = append(normalized, map[string]any{
				"code":   item["code"],
				"status": item["status"],
			})
		}
		obj["items"] = normalized
	}
	for path, placeholder := range replacements {
		replaceStringValues(obj, path, placeholder)
	}
	return marshalBackupVerifyAcceptanceJSON(t, obj)
}

func replaceStringValues(value any, old, new string) {
	switch typed := value.(type) {
	case map[string]any:
		for key, nested := range typed {
			if text, ok := nested.(string); ok {
				typed[key] = strings.ReplaceAll(text, old, new)
				continue
			}
			replaceStringValues(nested, old, new)
		}
	case []any:
		for idx, nested := range typed {
			if text, ok := nested.(string); ok {
				typed[idx] = strings.ReplaceAll(text, old, new)
				continue
			}
			replaceStringValues(nested, old, new)
		}
	}
}

func collectVerifyTreeEntries(t *testing.T, root string) []string {
	t.Helper()

	entries := []string{}
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == root {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if entry.IsDir() {
			entries = append(entries, rel+"/")
			return nil
		}
		entries = append(entries, rel)
		return nil
	})
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(entries)
	return entries
}

func sha256OfPath(t *testing.T, path string) string {
	t.Helper()

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func marshalBackupVerifyAcceptanceJSON(t *testing.T, value any) []byte {
	t.Helper()

	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func assertOrWriteBackupVerifyAcceptanceGoldenJSON(t *testing.T, got []byte, relPath string) {
	t.Helper()

	path := filepath.Join("..", "..", "acceptance", "v2", "backup_verify", relPath)
	gotNorm := normalizeJSONBytesForVerify(t, got)
	if os.Getenv(updateBackupVerifyAcceptanceEnv) == "1" {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, append(gotNorm, '\n'), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}

	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden %s: %v", path, err)
	}
	wantNorm := normalizeJSONBytesForVerify(t, want)
	if string(gotNorm) != string(wantNorm) {
		t.Fatalf("golden mismatch for %s\nGOT:\n%s\n\nWANT:\n%s", relPath, gotNorm, wantNorm)
	}
}

func normalizeJSONBytesForVerify(t *testing.T, raw []byte) []byte {
	t.Helper()

	var obj any
	if err := json.Unmarshal(raw, &obj); err != nil {
		t.Fatalf("invalid json: %v\n%s", err, string(raw))
	}
	out, err := json.MarshalIndent(obj, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	return out
}
