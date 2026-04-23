package app_test

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	v2app "github.com/lazuale/espocrm-ops/internal/app"
	"github.com/lazuale/espocrm-ops/internal/model"
	v2runtime "github.com/lazuale/espocrm-ops/internal/runtime"
	"github.com/lazuale/espocrm-ops/internal/store"
)

const updateRestoreAcceptanceEnv = "UPDATE_ACCEPTANCE_RESTORE"

type restoreBackupSet struct {
	Layout model.BackupLayout
}

type restoreAcceptanceFixture struct {
	root            string
	projectDir      string
	composeFile     string
	envFile         string
	storageDir      string
	backupRoot      string
	snapshotRoot    string
	backupSet       restoreBackupSet
	restoreRuntime  *v2runtime.Static
	snapshotRuntime *v2runtime.Static
	service         v2app.RestoreService
}

func TestRestoreV2Acceptance_JSONDiskAndRuntime(t *testing.T) {
	cases := []struct {
		id       string
		setup    func(*testing.T) (restoreAcceptanceFixture, model.RestoreRequest)
		wantOK   bool
		wantCode string
	}{
		{
			id: "RST-001",
			setup: func(t *testing.T) (restoreAcceptanceFixture, model.RestoreRequest) {
				fx := newRestoreAcceptanceFixture(t)
				req := fx.request()
				req.Manifest = fx.backupSet.Layout.ManifestJSON
				req = finalizeRestoreRequest(req)
				return fx, req
			},
			wantOK: true,
		},
		{
			id: "RST-002",
			setup: func(t *testing.T) (restoreAcceptanceFixture, model.RestoreRequest) {
				fx := newRestoreAcceptanceFixture(t)
				req := fx.request()
				req.DBBackup = fx.backupSet.Layout.DBArtifact
				req.FilesBackup = fx.backupSet.Layout.FilesArtifact
				req = finalizeRestoreRequest(req)
				return fx, req
			},
			wantOK: true,
		},
		{
			id: "RST-101",
			setup: func(t *testing.T) (restoreAcceptanceFixture, model.RestoreRequest) {
				fx := newRestoreAcceptanceFixture(t)
				req := fx.request()
				req.DBBackup = fx.backupSet.Layout.DBArtifact
				req.SkipFiles = true
				req.NoSnapshot = true
				req = finalizeRestoreRequest(req)
				return fx, req
			},
			wantOK: true,
		},
		{
			id: "RST-102",
			setup: func(t *testing.T) (restoreAcceptanceFixture, model.RestoreRequest) {
				fx := newRestoreAcceptanceFixture(t)
				req := fx.request()
				req.FilesBackup = fx.backupSet.Layout.FilesArtifact
				req.SkipDB = true
				req.NoSnapshot = true
				req.NoStart = true
				req = finalizeRestoreRequest(req)
				return fx, req
			},
			wantOK: true,
		},
		{
			id: "RST-204",
			setup: func(t *testing.T) (restoreAcceptanceFixture, model.RestoreRequest) {
				fx := newRestoreAcceptanceFixture(t)
				other := writeRestoreBackupSet(t, fx.backupRoot, "espocrm-prod", "2026-04-19_08-30-00", "prod", map[string]string{
					"espo/file.txt": "other\n",
				})
				req := fx.request()
				req.DBBackup = fx.backupSet.Layout.DBArtifact
				req.FilesBackup = other.Layout.FilesArtifact
				req.NoSnapshot = true
				req = finalizeRestoreRequest(req)
				return fx, req
			},
			wantOK:   false,
			wantCode: model.RestoreFailedCode,
		},
		{
			id: "RST-205",
			setup: func(t *testing.T) (restoreAcceptanceFixture, model.RestoreRequest) {
				fx := newRestoreAcceptanceFixture(t)
				req := fx.request()
				req.Manifest = fx.backupSet.Layout.ManifestJSON
				req.SkipFiles = true
				req.NoSnapshot = true
				req = finalizeRestoreRequest(req)
				return fx, req
			},
			wantOK:   false,
			wantCode: model.RestoreFailedCode,
		},
		{
			id: "RST-301",
			setup: func(t *testing.T) (restoreAcceptanceFixture, model.RestoreRequest) {
				fx := newRestoreAcceptanceFixture(t)
				req := fx.request()
				req.Manifest = fx.backupSet.Layout.ManifestJSON
				req = finalizeRestoreRequest(req)
				return fx, req
			},
			wantOK: true,
		},
		{
			id: "RST-302",
			setup: func(t *testing.T) (restoreAcceptanceFixture, model.RestoreRequest) {
				fx := newRestoreAcceptanceFixture(t)
				req := fx.request()
				req.Manifest = fx.backupSet.Layout.ManifestJSON
				req.NoSnapshot = true
				req = finalizeRestoreRequest(req)
				return fx, req
			},
			wantOK: true,
		},
		{
			id: "RST-303",
			setup: func(t *testing.T) (restoreAcceptanceFixture, model.RestoreRequest) {
				fx := newRestoreAcceptanceFixture(t)
				fx.snapshotRuntime.DumpErr = errors.New("snapshot dump failed")
				req := fx.request()
				req.Manifest = fx.backupSet.Layout.ManifestJSON
				req = finalizeRestoreRequest(req)
				return fx, req
			},
			wantOK:   false,
			wantCode: model.RestoreFailedCode,
		},
		{
			id: "RST-402",
			setup: func(t *testing.T) (restoreAcceptanceFixture, model.RestoreRequest) {
				fx := newRestoreAcceptanceFixture(t)
				req := fx.request()
				req.Manifest = fx.backupSet.Layout.ManifestJSON
				req.NoSnapshot = true
				req.NoStop = true
				req = finalizeRestoreRequest(req)
				return fx, req
			},
			wantOK: true,
		},
		{
			id: "RST-401",
			setup: func(t *testing.T) (restoreAcceptanceFixture, model.RestoreRequest) {
				fx := newRestoreAcceptanceFixture(t)
				req := fx.request()
				req.Manifest = fx.backupSet.Layout.ManifestJSON
				req.NoSnapshot = true
				req = finalizeRestoreRequest(req)
				return fx, req
			},
			wantOK: true,
		},
		{
			id: "RST-403",
			setup: func(t *testing.T) (restoreAcceptanceFixture, model.RestoreRequest) {
				fx := newRestoreAcceptanceFixture(t)
				req := fx.request()
				req.Manifest = fx.backupSet.Layout.ManifestJSON
				req.NoSnapshot = true
				req.NoStart = true
				req = finalizeRestoreRequest(req)
				return fx, req
			},
			wantOK: true,
		},
		{
			id: "RST-404",
			setup: func(t *testing.T) (restoreAcceptanceFixture, model.RestoreRequest) {
				fx := newRestoreAcceptanceFixture(t)
				fx.restoreRuntime.StartErr = errors.New("runtime return failed")
				req := fx.request()
				req.Manifest = fx.backupSet.Layout.ManifestJSON
				req.NoSnapshot = true
				req = finalizeRestoreRequest(req)
				return fx, req
			},
			wantOK:   false,
			wantCode: model.RestoreFailedCode,
		},
		{
			id: "RST-501",
			setup: func(t *testing.T) (restoreAcceptanceFixture, model.RestoreRequest) {
				fx := newRestoreAcceptanceFixture(t)
				fx.restoreRuntime.RestoreDBErr = errors.New("db restore failed")
				req := fx.request()
				req.Manifest = fx.backupSet.Layout.ManifestJSON
				req.NoSnapshot = true
				req = finalizeRestoreRequest(req)
				return fx, req
			},
			wantOK:   false,
			wantCode: model.RestoreFailedCode,
		},
		{
			id: "RST-502",
			setup: func(t *testing.T) (restoreAcceptanceFixture, model.RestoreRequest) {
				fx := newRestoreAcceptanceFixtureWithStore(t, failingRestoreStore{FileStore: store.FileStore{}, failRestoreFiles: true})
				req := fx.request()
				req.Manifest = fx.backupSet.Layout.ManifestJSON
				req.NoSnapshot = true
				req = finalizeRestoreRequest(req)
				return fx, req
			},
			wantOK:   false,
			wantCode: model.RestoreFailedCode,
		},
		{
			id: "RST-503",
			setup: func(t *testing.T) (restoreAcceptanceFixture, model.RestoreRequest) {
				fx := newRestoreAcceptanceFixture(t)
				fx.restoreRuntime.PermissionErr = errors.New("permission reconciliation failed")
				req := fx.request()
				req.Manifest = fx.backupSet.Layout.ManifestJSON
				req.NoSnapshot = true
				req = finalizeRestoreRequest(req)
				return fx, req
			},
			wantOK:   false,
			wantCode: model.RestoreFailedCode,
		},
		{
			id: "RST-504",
			setup: func(t *testing.T) (restoreAcceptanceFixture, model.RestoreRequest) {
				fx := newRestoreAcceptanceFixture(t)
				mustWriteFile(t, fx.backupSet.Layout.ManifestJSON, "{")
				req := fx.request()
				req.Manifest = fx.backupSet.Layout.ManifestJSON
				req.NoSnapshot = true
				req = finalizeRestoreRequest(req)
				return fx, req
			},
			wantOK:   false,
			wantCode: model.ManifestInvalidCode,
		},
		{
			id: "RST-505",
			setup: func(t *testing.T) (restoreAcceptanceFixture, model.RestoreRequest) {
				fx := newRestoreAcceptanceFixture(t)
				if err := os.Remove(fx.backupSet.Layout.DBArtifact); err != nil {
					t.Fatal(err)
				}
				req := fx.request()
				req.Manifest = fx.backupSet.Layout.ManifestJSON
				req.NoSnapshot = true
				req = finalizeRestoreRequest(req)
				return fx, req
			},
			wantOK:   false,
			wantCode: model.RestoreFailedCode,
		},
		{
			id: "RST-506",
			setup: func(t *testing.T) (restoreAcceptanceFixture, model.RestoreRequest) {
				fx := newRestoreAcceptanceFixture(t)
				mustWriteFile(t, fx.backupSet.Layout.FilesArtifact, "not archive")
				mustWriteFile(t, fx.backupSet.Layout.FilesChecksum, sha256OfPath(t, fx.backupSet.Layout.FilesArtifact)+"  "+filepath.Base(fx.backupSet.Layout.FilesArtifact)+"\n")
				req := fx.request()
				req.Manifest = fx.backupSet.Layout.ManifestJSON
				req.NoSnapshot = true
				req = finalizeRestoreRequest(req)
				return fx, req
			},
			wantOK:   false,
			wantCode: model.RestoreFailedCode,
		},
		{
			id: "RST-507",
			setup: func(t *testing.T) (restoreAcceptanceFixture, model.RestoreRequest) {
				fx := newRestoreAcceptanceFixture(t)
				mustWriteFile(t, fx.backupSet.Layout.DBChecksum, strings.Repeat("0", 64)+"  "+filepath.Base(fx.backupSet.Layout.DBArtifact)+"\n")
				req := fx.request()
				req.Manifest = fx.backupSet.Layout.ManifestJSON
				req.NoSnapshot = true
				req = finalizeRestoreRequest(req)
				return fx, req
			},
			wantOK:   false,
			wantCode: model.RestoreFailedCode,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.id, func(t *testing.T) {
			fx, req := tc.setup(t)
			beforeStorage := collectRestoreTreeEntries(t, fx.storageDir)

			result, err := fx.service.ExecuteRestore(context.Background(), req)
			if tc.wantOK && err != nil {
				t.Fatalf("ExecuteRestore returned error: %v", err)
			}
			if !tc.wantOK && err == nil {
				t.Fatal("expected restore error")
			}
			if result.OK != tc.wantOK {
				t.Fatalf("unexpected ok: %+v", result)
			}
			if tc.wantCode != "" {
				if result.Error == nil || result.Error.Code != tc.wantCode {
					t.Fatalf("expected error code %s, got %+v", tc.wantCode, result.Error)
				}
			}

			assertOrWriteRestoreAcceptanceGoldenJSON(t, normalizeRestoreAcceptanceJSON(t, result), filepath.Join("golden", "json", "v2_"+tc.id+".json"))
			assertOrWriteRestoreAcceptanceGoldenJSON(t, marshalRestoreAcceptanceJSON(t, map[string]any{
				"process_exit_code": result.ProcessExitCode,
				"before_storage":    beforeStorage,
				"after_storage":     collectRestoreTreeEntries(t, fx.storageDir),
				"snapshot_entries":  collectRestoreTreeEntries(t, fx.snapshotRoot),
			}), filepath.Join("golden", "disk", "v2_"+tc.id+".json"))
			assertOrWriteRestoreAcceptanceGoldenJSON(t, normalizeRestoreRuntimeGolden(t, fx, result), filepath.Join("golden", "runtime", "v2_"+tc.id+".json"))
		})
	}
}

type failingRestoreStore struct {
	store.FileStore
	failRestoreFiles bool
}

func (s failingRestoreStore) RestoreFilesArtifact(ctx context.Context, filesBackupPath, targetDir string, requireExactRoot bool) error {
	if s.failRestoreFiles {
		return errors.New("files restore failed")
	}
	return s.FileStore.RestoreFilesArtifact(ctx, filesBackupPath, targetDir, requireExactRoot)
}

func newRestoreAcceptanceFixture(t *testing.T) restoreAcceptanceFixture {
	t.Helper()
	return newRestoreAcceptanceFixtureWithStore(t, store.FileStore{})
}

func newRestoreAcceptanceFixtureWithStore(t *testing.T, restoreStore model.RestoreStore) restoreAcceptanceFixture {
	t.Helper()

	root := t.TempDir()
	projectDir := filepath.Join(root, "project")
	storageDir := filepath.Join(projectDir, "runtime", "prod", "espo")
	backupRoot := filepath.Join(projectDir, "backups", "prod")
	snapshotRoot := filepath.Join(projectDir, "snapshots", "prod")
	mustMkdirAll(t, storageDir)
	mustWriteFile(t, filepath.Join(storageDir, "stale.txt"), "stale\n")

	backupSet := writeRestoreBackupSet(t, backupRoot, "espocrm-prod", "2026-04-19_08-00-00", "prod", map[string]string{
		"espo/file.txt": "restored\n",
	})
	restoreRuntime := &v2runtime.Static{AppServicesRunning: true}
	snapshotRuntime := &v2runtime.Static{
		AppServicesRunning: false,
		DBDump:             gzipBytes("snapshot;\n"),
		FilesArchive:       tarGzBytes(map[string]string{"espo/snapshot.txt": "snapshot\n"}),
	}
	snapshotService := v2app.NewBackupService(v2app.BackupDependencies{
		Runtime: snapshotRuntime,
		Store:   store.FileStore{},
	})
	service := v2app.NewRestoreService(v2app.RestoreDependencies{
		Runtime:  restoreRuntime,
		Store:    restoreStore,
		Snapshot: snapshotService,
	})

	return restoreAcceptanceFixture{
		root:            root,
		projectDir:      projectDir,
		composeFile:     filepath.Join(projectDir, "compose.yaml"),
		envFile:         filepath.Join(projectDir, ".env.prod"),
		storageDir:      storageDir,
		backupRoot:      backupRoot,
		snapshotRoot:    snapshotRoot,
		backupSet:       backupSet,
		restoreRuntime:  restoreRuntime,
		snapshotRuntime: snapshotRuntime,
		service:         service,
	}
}

func (fx restoreAcceptanceFixture) request() model.RestoreRequest {
	createdAt := time.Date(2026, 4, 19, 9, 0, 0, 0, time.UTC)
	target := model.RuntimeTarget{
		ProjectDir:       fx.projectDir,
		ComposeFile:      fx.composeFile,
		EnvFile:          fx.envFile,
		StorageDir:       fx.storageDir,
		DBService:        "db",
		DBUser:           "espocrm",
		DBRootPassword:   "root-secret",
		DBName:           "espocrm",
		HelperImage:      "alpine:3.20",
		RuntimeUID:       1000,
		RuntimeGID:       1000,
		ReadinessTimeout: 1,
	}
	return model.RestoreRequest{
		Scope:       "prod",
		ProjectDir:  fx.projectDir,
		ComposeFile: fx.composeFile,
		EnvFile:     fx.envFile,
		BackupRoot:  fx.backupRoot,
		StorageDir:  fx.storageDir,
		Target:      target,
		Snapshot: model.BackupRequest{
			Scope:         "prod",
			ProjectDir:    fx.projectDir,
			ComposeFile:   fx.composeFile,
			EnvFile:       fx.envFile,
			BackupRoot:    fx.snapshotRoot,
			StorageDir:    fx.storageDir,
			NamePrefix:    "espocrm-prod",
			RetentionDays: 7,
			CreatedAt:     createdAt,
			DBService:     "db",
			DBUser:        "espocrm",
			DBPassword:    "secret",
			DBName:        "espocrm",
			Metadata: model.BackupMetadata{
				ComposeProject: "espocrm-prod",
				EnvFileName:    ".env.prod",
				EspoCRMImage:   "espocrm/espocrm:9.3.4-apache",
				MariaDBTag:     "10.11",
			},
		},
	}
}

func finalizeRestoreRequest(req model.RestoreRequest) model.RestoreRequest {
	req.Snapshot.SkipDB = req.SkipDB
	req.Snapshot.SkipFiles = req.SkipFiles
	req.Snapshot.NoStop = req.NoStop
	return req
}

func writeRestoreBackupSet(t *testing.T, root, prefix, stamp, scope string, files map[string]string) restoreBackupSet {
	t.Helper()

	layout := model.NewBackupLayoutForStamp(root, prefix, stamp)
	mustMkdirAll(t, filepath.Dir(layout.DBArtifact), filepath.Dir(layout.FilesArtifact), filepath.Dir(layout.ManifestJSON))
	if err := os.WriteFile(layout.DBArtifact, gzipBytes("select 1;\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(layout.FilesArtifact, tarGzBytes(files), 0o644); err != nil {
		t.Fatal(err)
	}

	dbChecksum := sha256OfPath(t, layout.DBArtifact)
	filesChecksum := sha256OfPath(t, layout.FilesArtifact)
	mustWriteFile(t, layout.DBChecksum, dbChecksum+"  "+filepath.Base(layout.DBArtifact)+"\n")
	mustWriteFile(t, layout.FilesChecksum, filesChecksum+"  "+filepath.Base(layout.FilesArtifact)+"\n")

	createdAt, err := model.ParseStamp(stamp)
	if err != nil {
		t.Fatal(err)
	}
	manifest := model.CompleteManifest{
		Version:                1,
		Scope:                  scope,
		CreatedAt:              createdAt.UTC().Format(time.RFC3339),
		Contour:                scope,
		ComposeProject:         prefix,
		EnvFile:                ".env." + scope,
		EspoCRMImage:           "espocrm/espocrm:9.3.4-apache",
		MariaDBTag:             "10.11",
		RetentionDays:          7,
		ConsistentSnapshot:     true,
		AppServicesWereRunning: true,
		Artifacts: model.ManifestArtifacts{
			DBBackup:    filepath.Base(layout.DBArtifact),
			FilesBackup: filepath.Base(layout.FilesArtifact),
		},
		Checksums: model.ManifestChecksums{
			DBBackup:    dbChecksum,
			FilesBackup: filesChecksum,
		},
		DBBackupCreated:    true,
		FilesBackupCreated: true,
	}
	raw, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(layout.ManifestJSON, append(raw, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
	mustWriteFile(t, layout.ManifestText, "created_at="+createdAt.UTC().Format(time.RFC3339)+"\n")
	return restoreBackupSet{Layout: layout}
}

func normalizeRestoreAcceptanceJSON(t *testing.T, result model.RestoreResult) []byte {
	t.Helper()

	raw := marshalRestoreAcceptanceJSON(t, result)
	obj := map[string]any{}
	if err := json.Unmarshal(raw, &obj); err != nil {
		t.Fatal(err)
	}
	replacements := restorePathReplacements(obj)
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
	return marshalRestoreAcceptanceJSON(t, obj)
}

func normalizeRestoreRuntimeGolden(t *testing.T, fx restoreAcceptanceFixture, result model.RestoreResult) []byte {
	t.Helper()
	obj := map[string]any{
		"process_exit_code":   result.ProcessExitCode,
		"restore_calls":       fx.restoreRuntime.Calls,
		"snapshot_calls":      fx.snapshotRuntime.Calls,
		"post_check_services": fx.restoreRuntime.PostCheckServices,
		"restored_db_path":    fx.restoreRuntime.RestoredDBPath,
	}
	for _, replacement := range []struct {
		path        string
		placeholder string
	}{
		{fx.backupSet.Layout.DBArtifact, "REPLACE_DB_BACKUP"},
		{fx.snapshotRoot, "REPLACE_SNAPSHOT_ROOT"},
		{fx.backupRoot, "REPLACE_BACKUP_ROOT"},
		{fx.projectDir, "REPLACE_PROJECT_DIR"},
	} {
		replaceStringValues(obj, replacement.path, replacement.placeholder)
	}
	return marshalRestoreAcceptanceJSON(t, obj)
}

func restorePathReplacements(obj map[string]any) map[string]string {
	replacements := map[string]string{}
	artifacts, _ := obj["artifacts"].(map[string]any)
	for _, key := range []string{
		"project_dir",
		"compose_file",
		"env_file",
		"backup_root",
		"manifest_json",
		"db_backup",
		"files_backup",
		"snapshot_manifest_json",
		"snapshot_db_backup",
		"snapshot_files_backup",
		"snapshot_db_checksum",
		"snapshot_files_checksum",
	} {
		value, _ := artifacts[key].(string)
		if strings.TrimSpace(value) == "" {
			continue
		}
		placeholder := "REPLACE_" + strings.ToUpper(key)
		artifacts[key] = placeholder
		replacements[value] = placeholder
	}
	return replacements
}

func collectRestoreTreeEntries(t *testing.T, root string) []string {
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

func marshalRestoreAcceptanceJSON(t *testing.T, value any) []byte {
	t.Helper()

	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func assertOrWriteRestoreAcceptanceGoldenJSON(t *testing.T, got []byte, relPath string) {
	t.Helper()

	path := filepath.Join("..", "..", "acceptance", "v2", "restore", relPath)
	gotNorm := normalizeJSONBytesForVerify(t, got)
	if os.Getenv(updateRestoreAcceptanceEnv) == "1" {
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
