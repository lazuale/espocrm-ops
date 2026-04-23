package app_test

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"io"
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

const updateMigrateAcceptanceEnv = "UPDATE_ACCEPTANCE_MIGRATE"

type migrateBackupSet struct {
	Layout model.BackupLayout
}

type migrateAcceptanceFixture struct {
	root             string
	projectDir       string
	composeFile      string
	sourceEnvFile    string
	targetEnvFile    string
	storageDir       string
	sourceBackupRoot string
	targetBackupRoot string
	sourceBackupSet  migrateBackupSet
	runtime          *migrateRuntime
	snapshotRuntime  *v2runtime.Static
	service          v2app.MigrateService
	sourceSettings   model.MigrateCompatibilitySettings
	targetSettings   model.MigrateCompatibilitySettings
}

type failingMigrateStore struct {
	store.FileStore
	failRestoreFiles bool
}

func (s failingMigrateStore) RestoreFilesArtifact(ctx context.Context, filesBackupPath, targetDir string, requireExactRoot bool) error {
	if s.failRestoreFiles {
		return errors.New("files migrate failed")
	}
	return s.FileStore.RestoreFilesArtifact(ctx, filesBackupPath, targetDir, requireExactRoot)
}

type migrateRuntime struct {
	running           []string
	InspectErr        error
	StopErr           error
	StartErr          error
	RestoreDBErr      error
	PermissionErr     error
	PostCheckErr      error
	RestoredDBPath    string
	PostCheckServices []string
	Calls             []string
}

func newMigrateRuntime(running ...string) *migrateRuntime {
	return &migrateRuntime{running: append([]string(nil), running...)}
}

func (r *migrateRuntime) RunningServices(ctx context.Context, target model.RuntimeTarget) ([]string, error) {
	r.Calls = append(r.Calls, "running_services")
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if r.InspectErr != nil {
		return nil, r.InspectErr
	}
	return append([]string(nil), r.running...), nil
}

func (r *migrateRuntime) StopServices(ctx context.Context, target model.RuntimeTarget, services ...string) error {
	r.Calls = append(r.Calls, "stop_services")
	if err := ctx.Err(); err != nil {
		return err
	}
	if r.StopErr != nil {
		return r.StopErr
	}
	r.removeServices(services...)
	return nil
}

func (r *migrateRuntime) StartServices(ctx context.Context, target model.RuntimeTarget, services ...string) error {
	call := "start_services"
	if len(services) == 1 && services[0] == "db" {
		call = "start_db"
	}
	r.Calls = append(r.Calls, call)
	if err := ctx.Err(); err != nil {
		return err
	}
	if r.StartErr != nil {
		return r.StartErr
	}
	r.addServices(services...)
	return nil
}

func (r *migrateRuntime) RestoreDatabase(ctx context.Context, target model.RuntimeTarget, dbBackupPath string) error {
	r.Calls = append(r.Calls, "restore_database")
	if err := ctx.Err(); err != nil {
		return err
	}
	if r.RestoreDBErr != nil {
		return r.RestoreDBErr
	}
	r.RestoredDBPath = dbBackupPath
	return nil
}

func (r *migrateRuntime) ReconcileFilesPermissions(ctx context.Context, target model.RuntimeTarget) error {
	r.Calls = append(r.Calls, "reconcile_files_permissions")
	if err := ctx.Err(); err != nil {
		return err
	}
	return r.PermissionErr
}

func (r *migrateRuntime) PostRestoreCheck(ctx context.Context, target model.RuntimeTarget, services ...string) error {
	r.Calls = append(r.Calls, "post_migrate_check")
	if err := ctx.Err(); err != nil {
		return err
	}
	r.PostCheckServices = append([]string(nil), services...)
	return r.PostCheckErr
}

func (r *migrateRuntime) addServices(services ...string) {
	seen := map[string]struct{}{}
	for _, service := range r.running {
		service = strings.TrimSpace(service)
		if service == "" {
			continue
		}
		seen[service] = struct{}{}
	}
	for _, service := range services {
		service = strings.TrimSpace(service)
		if service == "" {
			continue
		}
		if _, ok := seen[service]; ok {
			continue
		}
		seen[service] = struct{}{}
		r.running = append(r.running, service)
	}
	sort.Strings(r.running)
}

func (r *migrateRuntime) removeServices(services ...string) {
	remove := map[string]struct{}{}
	for _, service := range services {
		service = strings.TrimSpace(service)
		if service == "" {
			continue
		}
		remove[service] = struct{}{}
	}
	kept := make([]string, 0, len(r.running))
	for _, service := range r.running {
		if _, ok := remove[service]; ok {
			continue
		}
		kept = append(kept, service)
	}
	r.running = kept
}

func TestMigrateV2Acceptance_JSONDiskAndRuntime(t *testing.T) {
	cases := []struct {
		id       string
		setup    func(*testing.T) (migrateAcceptanceFixture, model.MigrateRequest)
		wantOK   bool
		wantCode string
	}{
		{
			id: "MIG-001",
			setup: func(t *testing.T) (migrateAcceptanceFixture, model.MigrateRequest) {
				fx := newMigrateAcceptanceFixture(t)
				req := fx.request()
				req = finalizeMigrateRequest(req)
				return fx, req
			},
			wantOK: true,
		},
		{
			id: "MIG-002",
			setup: func(t *testing.T) (migrateAcceptanceFixture, model.MigrateRequest) {
				fx := newMigrateAcceptanceFixture(t)
				req := fx.request()
				req.DBBackup = fx.sourceBackupSet.Layout.DBArtifact
				req.FilesBackup = fx.sourceBackupSet.Layout.FilesArtifact
				req = finalizeMigrateRequest(req)
				return fx, req
			},
			wantOK: true,
		},
		{
			id: "MIG-101",
			setup: func(t *testing.T) (migrateAcceptanceFixture, model.MigrateRequest) {
				fx := newMigrateAcceptanceFixture(t)
				req := fx.request()
				req.DBBackup = fx.sourceBackupSet.Layout.DBArtifact
				req.SkipFiles = true
				req = finalizeMigrateRequest(req)
				return fx, req
			},
			wantOK: true,
		},
		{
			id: "MIG-102",
			setup: func(t *testing.T) (migrateAcceptanceFixture, model.MigrateRequest) {
				fx := newMigrateAcceptanceFixture(t)
				req := fx.request()
				req.FilesBackup = fx.sourceBackupSet.Layout.FilesArtifact
				req.SkipDB = true
				req = finalizeMigrateRequest(req)
				return fx, req
			},
			wantOK: true,
		},
		{
			id: "MIG-205",
			setup: func(t *testing.T) (migrateAcceptanceFixture, model.MigrateRequest) {
				fx := newMigrateAcceptanceFixture(t)
				other := writeRestoreBackupSet(t, fx.sourceBackupRoot, "espocrm-dev", "2026-04-19_07-30-00", "dev", map[string]string{
					"espo/other.txt": "other\n",
				})
				writeJSONFileForVerify(t, fx.sourceBackupSet.Layout.ManifestJSON, map[string]any{
					"version":    1,
					"scope":      "dev",
					"created_at": "2026-04-19T08:00:00Z",
					"artifacts": map[string]any{
						"db_backup":    filepath.Base(fx.sourceBackupSet.Layout.DBArtifact),
						"files_backup": filepath.Base(other.Layout.FilesArtifact),
					},
					"checksums": map[string]any{
						"db_backup":    sha256OfPath(t, fx.sourceBackupSet.Layout.DBArtifact),
						"files_backup": sha256OfPath(t, other.Layout.FilesArtifact),
					},
				})
				req := fx.request()
				req = finalizeMigrateRequest(req)
				return fx, req
			},
			wantOK:   false,
			wantCode: model.MigrateFailedCode,
		},
		{
			id: "MIG-206",
			setup: func(t *testing.T) (migrateAcceptanceFixture, model.MigrateRequest) {
				fx := newMigrateAcceptanceFixture(t)
				other := writeRestoreBackupSet(t, fx.sourceBackupRoot, "espocrm-dev", "2026-04-19_08-30-00", "dev", map[string]string{
					"espo/other.txt": "other\n",
				})
				req := fx.request()
				req.DBBackup = fx.sourceBackupSet.Layout.DBArtifact
				req.FilesBackup = other.Layout.FilesArtifact
				req = finalizeMigrateRequest(req)
				return fx, req
			},
			wantOK:   false,
			wantCode: model.MigrateFailedCode,
		},
		{
			id: "MIG-301",
			setup: func(t *testing.T) (migrateAcceptanceFixture, model.MigrateRequest) {
				fx := newMigrateAcceptanceFixture(t)
				req := fx.request()
				req.TargetSettings.EspoCRMImage = "espocrm/espocrm:9.4.0-apache"
				req = finalizeMigrateRequest(req)
				return fx, req
			},
			wantOK:   false,
			wantCode: model.MigrateFailedCode,
		},
		{
			id: "MIG-207",
			setup: func(t *testing.T) (migrateAcceptanceFixture, model.MigrateRequest) {
				fx := newMigrateAcceptanceFixture(t)
				req := fx.request()
				req.DBBackup = fx.sourceBackupSet.Layout.DBArtifact
				req = finalizeMigrateRequest(req)
				return fx, req
			},
			wantOK:   false,
			wantCode: model.MigrateFailedCode,
		},
		{
			id: "MIG-208",
			setup: func(t *testing.T) (migrateAcceptanceFixture, model.MigrateRequest) {
				fx := newMigrateAcceptanceFixture(t)
				req := fx.request()
				req.FilesBackup = fx.sourceBackupSet.Layout.FilesArtifact
				req = finalizeMigrateRequest(req)
				return fx, req
			},
			wantOK:   false,
			wantCode: model.MigrateFailedCode,
		},
		{
			id: "MIG-401",
			setup: func(t *testing.T) (migrateAcceptanceFixture, model.MigrateRequest) {
				fx := newMigrateAcceptanceFixture(t)
				req := fx.request()
				req = finalizeMigrateRequest(req)
				return fx, req
			},
			wantOK: true,
		},
		{
			id: "MIG-402",
			setup: func(t *testing.T) (migrateAcceptanceFixture, model.MigrateRequest) {
				fx := newMigrateAcceptanceFixture(t)
				req := fx.request()
				req.NoStart = true
				req = finalizeMigrateRequest(req)
				return fx, req
			},
			wantOK: true,
		},
		{
			id: "MIG-403",
			setup: func(t *testing.T) (migrateAcceptanceFixture, model.MigrateRequest) {
				fx := newMigrateAcceptanceFixture(t)
				fx.runtime.PostCheckErr = errors.New("target health failed")
				req := fx.request()
				req = finalizeMigrateRequest(req)
				return fx, req
			},
			wantOK:   false,
			wantCode: model.MigrateFailedCode,
		},
		{
			id: "MIG-501",
			setup: func(t *testing.T) (migrateAcceptanceFixture, model.MigrateRequest) {
				fx := newMigrateAcceptanceFixture(t)
				fx.snapshotRuntime.DumpErr = errors.New("snapshot dump failed")
				req := fx.request()
				req = finalizeMigrateRequest(req)
				return fx, req
			},
			wantOK:   false,
			wantCode: model.MigrateFailedCode,
		},
		{
			id: "MIG-502",
			setup: func(t *testing.T) (migrateAcceptanceFixture, model.MigrateRequest) {
				fx := newMigrateAcceptanceFixture(t)
				fx.runtime.RestoreDBErr = errors.New("db migrate failed")
				req := fx.request()
				req = finalizeMigrateRequest(req)
				return fx, req
			},
			wantOK:   false,
			wantCode: model.MigrateFailedCode,
		},
		{
			id: "MIG-503",
			setup: func(t *testing.T) (migrateAcceptanceFixture, model.MigrateRequest) {
				fx := newMigrateAcceptanceFixtureWithStore(t, failingMigrateStore{FileStore: store.FileStore{}, failRestoreFiles: true})
				req := fx.request()
				req = finalizeMigrateRequest(req)
				return fx, req
			},
			wantOK:   false,
			wantCode: model.MigrateFailedCode,
		},
		{
			id: "MIG-504",
			setup: func(t *testing.T) (migrateAcceptanceFixture, model.MigrateRequest) {
				fx := newMigrateAcceptanceFixture(t)
				fx.runtime.PermissionErr = errors.New("permission reconcile failed")
				req := fx.request()
				req = finalizeMigrateRequest(req)
				return fx, req
			},
			wantOK:   false,
			wantCode: model.MigrateFailedCode,
		},
		{
			id: "MIG-505",
			setup: func(t *testing.T) (migrateAcceptanceFixture, model.MigrateRequest) {
				fx := newMigrateAcceptanceFixture(t)
				req := fx.request()
				req.DBBackup = filepath.Join(fx.sourceBackupRoot, "db", "missing.sql.gz")
				req.SkipFiles = true
				req = finalizeMigrateRequest(req)
				return fx, req
			},
			wantOK:   false,
			wantCode: model.MigrateFailedCode,
		},
		{
			id: "MIG-506",
			setup: func(t *testing.T) (migrateAcceptanceFixture, model.MigrateRequest) {
				fx := newMigrateAcceptanceFixture(t)
				if err := os.WriteFile(fx.sourceBackupSet.Layout.FilesArtifact, []byte("broken"), 0o644); err != nil {
					t.Fatal(err)
				}
				mustWriteFile(t, fx.sourceBackupSet.Layout.FilesChecksum, sha256OfPath(t, fx.sourceBackupSet.Layout.FilesArtifact)+"  "+filepath.Base(fx.sourceBackupSet.Layout.FilesArtifact)+"\n")
				req := fx.request()
				req = finalizeMigrateRequest(req)
				return fx, req
			},
			wantOK:   false,
			wantCode: model.MigrateFailedCode,
		},
		{
			id: "MIG-507",
			setup: func(t *testing.T) (migrateAcceptanceFixture, model.MigrateRequest) {
				fx := newMigrateAcceptanceFixture(t)
				mustWriteFile(t, fx.sourceBackupSet.Layout.DBChecksum, strings.Repeat("0", 64)+"  "+filepath.Base(fx.sourceBackupSet.Layout.DBArtifact)+"\n")
				req := fx.request()
				req.SkipFiles = true
				req.DBBackup = fx.sourceBackupSet.Layout.DBArtifact
				req = finalizeMigrateRequest(req)
				return fx, req
			},
			wantOK:   false,
			wantCode: model.MigrateFailedCode,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.id, func(t *testing.T) {
			fx, req := tc.setup(t)
			result, err := fx.service.ExecuteMigrate(context.Background(), req)
			if tc.wantOK {
				if err != nil {
					t.Fatalf("ExecuteMigrate failed: %v", err)
				}
				if !result.OK {
					t.Fatalf("expected ok result, got %#v", result.Error)
				}
			} else {
				if err == nil {
					t.Fatal("expected migrate failure")
				}
				if result.OK {
					t.Fatal("expected result.OK=false")
				}
				if result.Error == nil || result.Error.Code != tc.wantCode {
					t.Fatalf("unexpected error %#v", result.Error)
				}
			}

			assertOrWriteMigrateAcceptanceGoldenJSON(t, normalizeMigrateAcceptanceJSON(t, result), filepath.Join("golden", "json", "v2_"+tc.id+".json"))
			assertOrWriteMigrateAcceptanceGoldenJSON(t, normalizeMigrateDiskGolden(t, fx, result), filepath.Join("golden", "disk", "v2_"+tc.id+".json"))
			assertOrWriteMigrateAcceptanceGoldenJSON(t, normalizeMigrateRuntimeGolden(t, fx, result), filepath.Join("golden", "runtime", "v2_"+tc.id+".json"))
		})
	}
}

func TestMigrateV2RejectsIncoherentRequest(t *testing.T) {
	cases := []struct {
		name        string
		mutate      func(*model.MigrateRequest)
		messagePart string
	}{
		{
			name: "snapshot_scope_mismatch",
			mutate: func(req *model.MigrateRequest) {
				req.Snapshot.Scope = "dev"
			},
			messagePart: "snapshot scope не согласован",
		},
		{
			name: "missing_target_db_root_password",
			mutate: func(req *model.MigrateRequest) {
				req.Target.DBRootPassword = ""
			},
			messagePart: "target DB root password обязателен",
		},
		{
			name: "snapshot_flags_mismatch",
			mutate: func(req *model.MigrateRequest) {
				req.Snapshot.SkipDB = !req.SkipDB
			},
			messagePart: "target snapshot flags должны совпадать",
		},
		{
			name: "empty_source_settings",
			mutate: func(req *model.MigrateRequest) {
				req.SourceSettings = model.MigrateCompatibilitySettings{}
			},
			messagePart: "source ESPOCRM_IMAGE обязателен",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			fx := newMigrateAcceptanceFixture(t)
			req := finalizeMigrateRequest(fx.request())
			tc.mutate(&req)

			result, err := fx.service.ExecuteMigrate(context.Background(), req)
			if err == nil {
				t.Fatal("expected validation failure")
			}
			if result.OK {
				t.Fatal("expected result.OK=false")
			}
			if result.Error == nil {
				t.Fatal("expected result.Error")
			}
			if !strings.Contains(result.Error.Message, tc.messagePart) {
				t.Fatalf("expected error message to contain %q, got %q", tc.messagePart, result.Error.Message)
			}
			if len(result.Items) != 0 {
				t.Fatalf("expected no execution steps, got %#v", result.Items)
			}
			if len(fx.runtime.Calls) != 0 {
				t.Fatalf("expected no migrate runtime calls, got %v", fx.runtime.Calls)
			}
			if len(fx.snapshotRuntime.Calls) != 0 {
				t.Fatalf("expected no snapshot runtime calls, got %v", fx.snapshotRuntime.Calls)
			}
			if strings.TrimSpace(result.Artifacts.SnapshotManifestJSON) != "" {
				t.Fatalf("expected no snapshot artifacts, got %#v", result.Artifacts)
			}
		})
	}
}

func newMigrateAcceptanceFixture(t *testing.T) migrateAcceptanceFixture {
	t.Helper()
	return newMigrateAcceptanceFixtureWithStoreAndFiles(t, store.FileStore{}, map[string]string{
		"espo/file.txt": "restored\n",
	})
}

func newMigrateAcceptanceFixtureWithStore(t *testing.T, migrateStore model.MigrateStore) migrateAcceptanceFixture {
	t.Helper()
	return newMigrateAcceptanceFixtureWithStoreAndFiles(t, migrateStore, map[string]string{
		"espo/file.txt": "restored\n",
	})
}

func newMigrateAcceptanceFixtureWithStoreAndFiles(t *testing.T, migrateStore model.MigrateStore, files map[string]string) migrateAcceptanceFixture {
	t.Helper()

	root := t.TempDir()
	projectDir := filepath.Join(root, "project")
	storageDir := filepath.Join(projectDir, "runtime", "prod", "espo")
	sourceBackupRoot := filepath.Join(projectDir, "backups", "dev")
	targetBackupRoot := filepath.Join(projectDir, "backups", "prod")
	mustMkdirAll(t, storageDir)
	mustWriteFile(t, filepath.Join(storageDir, "stale.txt"), "stale\n")

	sourceBackup := writeRestoreBackupSet(t, sourceBackupRoot, "espocrm-dev", "2026-04-19_08-00-00", "dev", files)
	sourceSettings := model.MigrateCompatibilitySettings{
		EspoCRMImage:    "espocrm/espocrm:9.3.4-apache",
		MariaDBTag:      "10.11",
		DefaultLanguage: "en_US",
		TimeZone:        "UTC",
	}
	targetSettings := sourceSettings
	runtime := newMigrateRuntime("espocrm", "espocrm-daemon", "espocrm-websocket")
	snapshotRuntime := &v2runtime.Static{
		AppServicesRunning: true,
		DBDump:             gzipBytes("snapshot;\n"),
		FilesArchive:       tarGzBytes(map[string]string{"espo/snapshot.txt": "snapshot\n"}),
	}
	snapshotService := v2app.NewBackupService(v2app.BackupDependencies{
		Runtime: snapshotRuntime,
		Store:   store.FileStore{},
	})
	service := v2app.NewMigrateService(v2app.MigrateDependencies{
		Runtime:  runtime,
		Store:    migrateStore,
		Snapshot: snapshotService,
	})

	return migrateAcceptanceFixture{
		root:             root,
		projectDir:       projectDir,
		composeFile:      filepath.Join(projectDir, "compose.yaml"),
		sourceEnvFile:    filepath.Join(projectDir, ".env.dev"),
		targetEnvFile:    filepath.Join(projectDir, ".env.prod"),
		storageDir:       storageDir,
		sourceBackupRoot: sourceBackupRoot,
		targetBackupRoot: targetBackupRoot,
		sourceBackupSet:  migrateBackupSet(sourceBackup),
		runtime:          runtime,
		snapshotRuntime:  snapshotRuntime,
		service:          service,
		sourceSettings:   sourceSettings,
		targetSettings:   targetSettings,
	}
}

func (fx migrateAcceptanceFixture) request() model.MigrateRequest {
	createdAt := time.Date(2026, 4, 19, 9, 0, 0, 0, time.UTC)
	target := model.RuntimeTarget{
		ProjectDir:       fx.projectDir,
		ComposeFile:      fx.composeFile,
		EnvFile:          fx.targetEnvFile,
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
	return model.MigrateRequest{
		SourceScope:      "dev",
		TargetScope:      "prod",
		ProjectDir:       fx.projectDir,
		ComposeFile:      fx.composeFile,
		SourceEnvFile:    fx.sourceEnvFile,
		TargetEnvFile:    fx.targetEnvFile,
		SourceBackupRoot: fx.sourceBackupRoot,
		TargetBackupRoot: fx.targetBackupRoot,
		StorageDir:       fx.storageDir,
		Target:           target,
		SourceSettings:   fx.sourceSettings,
		TargetSettings:   fx.targetSettings,
		Snapshot: model.BackupRequest{
			Scope:         "prod",
			ProjectDir:    fx.projectDir,
			ComposeFile:   fx.composeFile,
			EnvFile:       fx.targetEnvFile,
			BackupRoot:    fx.targetBackupRoot,
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
				EspoCRMImage:   fx.targetSettings.EspoCRMImage,
				MariaDBTag:     fx.targetSettings.MariaDBTag,
			},
		},
	}
}

func finalizeMigrateRequest(req model.MigrateRequest) model.MigrateRequest {
	req.Snapshot.SkipDB = req.SkipDB
	req.Snapshot.SkipFiles = req.SkipFiles
	req.Snapshot.HelperArchive.Image = req.Target.HelperImage
	req.Snapshot.Metadata.EspoCRMImage = req.TargetSettings.EspoCRMImage
	req.Snapshot.Metadata.MariaDBTag = req.TargetSettings.MariaDBTag
	return req
}

func normalizeMigrateAcceptanceJSON(t *testing.T, result model.MigrateResult) []byte {
	t.Helper()

	raw := marshalRestoreAcceptanceJSON(t, result)
	obj := map[string]any{}
	if err := json.Unmarshal(raw, &obj); err != nil {
		t.Fatal(err)
	}
	if details, ok := obj["details"].(map[string]any); ok {
		if _, ok := details["selection_mode"]; !ok {
			details["selection_mode"] = ""
		}
		if _, ok := details["source_kind"]; !ok {
			details["source_kind"] = ""
		}
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
	for path, placeholder := range migratePathReplacements(obj) {
		replaceStringValues(obj, path, placeholder)
	}
	return marshalRestoreAcceptanceJSON(t, obj)
}

func normalizeMigrateDiskGolden(t *testing.T, fx migrateAcceptanceFixture, result model.MigrateResult) []byte {
	t.Helper()

	obj := map[string]any{
		"process_exit_code":        result.ProcessExitCode,
		"storage_entries_after":    collectRestoreTreeEntries(t, fx.storageDir),
		"storage_modes_after":      collectRestoreTreeModes(t, fx.storageDir),
		"source_backup_root_after": collectRestoreTreeEntries(t, fx.sourceBackupRoot),
		"target_backup_root_after": collectRestoreTreeEntries(t, fx.targetBackupRoot),
	}

	if path := strings.TrimSpace(result.Artifacts.SnapshotManifestJSON); path != "" {
		if _, err := os.Stat(path); err == nil {
			obj["snapshot_manifest_json"] = mustReadJSONFile(t, path)
		}
	}
	if path := strings.TrimSpace(result.Artifacts.SnapshotDBBackup); path != "" {
		if _, err := os.Stat(path); err == nil {
			obj["snapshot_db_sql"] = mustReadGzipText(t, path)
		}
	}
	if path := strings.TrimSpace(result.Artifacts.SnapshotFilesBackup); path != "" {
		if _, err := os.Stat(path); err == nil {
			obj["snapshot_files_entries"] = mustReadTarEntries(t, path)
		}
	}

	for _, replacement := range []struct {
		path        string
		placeholder string
	}{
		{fx.projectDir, "REPLACE_PROJECT_DIR"},
		{fx.sourceBackupRoot, "REPLACE_SOURCE_BACKUP_ROOT"},
		{fx.targetBackupRoot, "REPLACE_TARGET_BACKUP_ROOT"},
		{fx.storageDir, "REPLACE_STORAGE_DIR"},
	} {
		replaceStringValues(obj, replacement.path, replacement.placeholder)
	}
	return marshalRestoreAcceptanceJSON(t, obj)
}

func normalizeMigrateRuntimeGolden(t *testing.T, fx migrateAcceptanceFixture, result model.MigrateResult) []byte {
	t.Helper()

	obj := map[string]any{
		"process_exit_code":   result.ProcessExitCode,
		"migrate_calls":       fx.runtime.Calls,
		"snapshot_calls":      fx.snapshotRuntime.Calls,
		"post_check_services": fx.runtime.PostCheckServices,
		"restored_db_path":    fx.runtime.RestoredDBPath,
		"running_after":       append([]string(nil), fx.runtime.running...),
	}
	for _, replacement := range []struct {
		path        string
		placeholder string
	}{
		{fx.projectDir, "REPLACE_PROJECT_DIR"},
		{fx.sourceBackupRoot, "REPLACE_SOURCE_BACKUP_ROOT"},
		{fx.targetBackupRoot, "REPLACE_TARGET_BACKUP_ROOT"},
		{fx.sourceBackupSet.Layout.DBArtifact, "REPLACE_DB_BACKUP"},
		{fx.sourceBackupSet.Layout.FilesArtifact, "REPLACE_FILES_BACKUP"},
	} {
		replaceStringValues(obj, replacement.path, replacement.placeholder)
	}
	return marshalRestoreAcceptanceJSON(t, obj)
}

func migratePathReplacements(obj map[string]any) map[string]string {
	replacements := map[string]string{}
	artifacts, _ := obj["artifacts"].(map[string]any)
	for _, key := range []string{
		"project_dir",
		"compose_file",
		"source_env_file",
		"target_env_file",
		"source_backup_root",
		"target_backup_root",
		"manifest_txt",
		"manifest_json",
		"db_backup",
		"files_backup",
		"snapshot_manifest_txt",
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

func assertOrWriteMigrateAcceptanceGoldenJSON(t *testing.T, got []byte, relPath string) {
	t.Helper()

	path := filepath.Join("..", "..", "acceptance", "v2", "migrate", relPath)
	gotNorm := normalizeJSONBytesForVerify(t, got)
	if os.Getenv(updateMigrateAcceptanceEnv) == "1" {
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

func mustReadJSONFile(t *testing.T, path string) any {
	t.Helper()

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		t.Fatal(err)
	}
	return value
}

func mustReadGzipText(t *testing.T, path string) string {
	t.Helper()

	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := file.Close(); err != nil {
			t.Fatal(err)
		}
	}()
	reader, err := gzip.NewReader(file)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := reader.Close(); err != nil {
			t.Fatal(err)
		}
	}()
	raw, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func mustReadTarEntries(t *testing.T, path string) []string {
	t.Helper()

	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := file.Close(); err != nil {
			t.Fatal(err)
		}
	}()
	gz, err := gzip.NewReader(file)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := gz.Close(); err != nil {
			t.Fatal(err)
		}
	}()
	tr := tar.NewReader(gz)
	entries := []string{}
	for {
		header, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		entries = append(entries, header.Name)
	}
	return entries
}
