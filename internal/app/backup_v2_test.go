package app_test

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	v2app "github.com/lazuale/espocrm-ops/internal/app"
	"github.com/lazuale/espocrm-ops/internal/model"
	v2runtime "github.com/lazuale/espocrm-ops/internal/runtime"
	"github.com/lazuale/espocrm-ops/internal/store"
)

func TestBackupV2FullBackupCreatesCompleteSet(t *testing.T) {
	req := testBackupRequest(t)
	createRetentionSet(t, req.BackupRoot, req.NamePrefix, "2026-04-01_00-00-00")
	createRetentionSet(t, req.BackupRoot, req.NamePrefix, "2026-04-14_00-00-00")

	rt := testRuntime(true)
	result, err := testService(rt, store.FileStore{}).ExecuteBackup(context.Background(), req)
	if err != nil {
		t.Fatalf("ExecuteBackup returned error: %v", err)
	}
	if !result.OK || result.ProcessExitCode != 0 || !result.Details.Ready {
		t.Fatalf("unexpected result: %+v", result)
	}
	assertSteps(t, result, []model.Step{
		{Code: model.StepArtifactAllocation, Status: model.StatusCompleted},
		{Code: model.StepRuntimePrepare, Status: model.StatusCompleted},
		{Code: model.StepDBBackup, Status: model.StatusCompleted},
		{Code: model.StepFilesBackup, Status: model.StatusCompleted},
		{Code: model.StepFinalize, Status: model.StatusCompleted},
		{Code: model.StepRetention, Status: model.StatusCompleted},
		{Code: model.StepRuntimeReturn, Status: model.StatusCompleted},
	})
	if err := rt.RequireCallOrder([]string{"running_services", "stop_services", "dump_database", "archive_files", "start_services"}); err != nil {
		t.Fatal(err)
	}
	if !result.Details.ConsistentSnapshot || !result.Details.AppServicesWereRunning {
		t.Fatalf("unexpected snapshot details: %+v", result.Details)
	}

	layout := model.NewBackupLayout(req.BackupRoot, req.NamePrefix, req.CreatedAt)
	assertFileExists(t, layout.DBArtifact)
	assertFileExists(t, layout.DBChecksum)
	assertFileExists(t, layout.FilesArtifact)
	assertFileExists(t, layout.FilesChecksum)
	assertFileExists(t, layout.ManifestJSON)
	assertFileExists(t, layout.ManifestText)
	assertNoTmpFiles(t, req.BackupRoot)
	assertGzipPayload(t, layout.DBArtifact, "select 1;\n")
	assertTarEntries(t, layout.FilesArtifact, []string{"espo/", "espo/file.txt"})
	assertChecksumSidecar(t, layout.DBArtifact, layout.DBChecksum)
	assertChecksumSidecar(t, layout.FilesArtifact, layout.FilesChecksum)
	assertManifestJSON(t, layout.ManifestJSON, req, layout)
	assertManifestText(t, layout.ManifestText, []string{
		"created_at=2026-04-15_11-00-00\n",
		"retention_days=7\n",
		"db_backup_created=1\n",
		"files_backup_created=1\n",
		"consistent_snapshot=1\n",
		"app_services_were_running=1\n",
	})
	assertBackupSetAbsent(t, req.BackupRoot, req.NamePrefix, "2026-04-01_00-00-00")
	assertBackupSetPresent(t, req.BackupRoot, req.NamePrefix, "2026-04-14_00-00-00")
}

func TestBackupV2FullBackupStoppedRuntimeDoesNotStart(t *testing.T) {
	req := testBackupRequest(t)
	rt := testRuntime(false)

	result, err := testService(rt, store.FileStore{}).ExecuteBackup(context.Background(), req)
	if err != nil {
		t.Fatalf("ExecuteBackup returned error: %v", err)
	}
	if !result.OK {
		t.Fatalf("backup failed: %+v", result)
	}
	if err := rt.RequireCallOrder([]string{"running_services", "dump_database", "archive_files"}); err != nil {
		t.Fatal(err)
	}
	if result.Details.AppServicesWereRunning {
		t.Fatalf("runtime был отмечен как running: %+v", result.Details)
	}
	assertSteps(t, result, []model.Step{
		{Code: model.StepArtifactAllocation, Status: model.StatusCompleted},
		{Code: model.StepRuntimePrepare, Status: model.StatusCompleted},
		{Code: model.StepDBBackup, Status: model.StatusCompleted},
		{Code: model.StepFilesBackup, Status: model.StatusCompleted},
		{Code: model.StepFinalize, Status: model.StatusCompleted},
		{Code: model.StepRetention, Status: model.StatusCompleted},
		{Code: model.StepRuntimeReturn, Status: model.StatusSkipped},
	})
}

func TestBackupV2NoStopDoesNotTouchRuntimeState(t *testing.T) {
	req := testBackupRequest(t)
	req.NoStop = true
	rt := testRuntime(true)

	result, err := testService(rt, store.FileStore{}).ExecuteBackup(context.Background(), req)
	if err != nil {
		t.Fatalf("ExecuteBackup returned error: %v", err)
	}
	if !result.OK {
		t.Fatalf("backup failed: %+v", result)
	}
	if err := rt.RequireCallOrder([]string{"dump_database", "archive_files"}); err != nil {
		t.Fatal(err)
	}
	if !rt.AppServicesRunning {
		t.Fatalf("--no-stop изменил runtime state")
	}
	if result.Details.ConsistentSnapshot || result.Details.AppServicesWereRunning {
		t.Fatalf("unexpected no-stop details: %+v", result.Details)
	}
	layout := model.NewBackupLayout(req.BackupRoot, req.NamePrefix, req.CreatedAt)
	assertFileExists(t, layout.DBArtifact)
	assertFileExists(t, layout.DBChecksum)
	assertFileExists(t, layout.FilesArtifact)
	assertFileExists(t, layout.FilesChecksum)
	assertFileExists(t, layout.ManifestJSON)
	assertFileExists(t, layout.ManifestText)
}

func TestBackupV2PartialBackupsDoNotCreateManifest(t *testing.T) {
	tests := []struct {
		name          string
		mutate        func(*model.BackupRequest)
		wantArtifacts func(model.BackupLayout) []string
		wantMissing   func(model.BackupLayout) []string
		wantCalls     []string
	}{
		{
			name: "files-only",
			mutate: func(req *model.BackupRequest) {
				req.SkipDB = true
				req.NoStop = true
			},
			wantArtifacts: func(layout model.BackupLayout) []string {
				return []string{layout.FilesArtifact, layout.FilesChecksum}
			},
			wantMissing: func(layout model.BackupLayout) []string {
				return []string{layout.DBArtifact, layout.DBChecksum, layout.ManifestJSON, layout.ManifestText}
			},
			wantCalls: []string{"archive_files"},
		},
		{
			name: "db-only",
			mutate: func(req *model.BackupRequest) {
				req.SkipFiles = true
				req.NoStop = true
			},
			wantArtifacts: func(layout model.BackupLayout) []string {
				return []string{layout.DBArtifact, layout.DBChecksum}
			},
			wantMissing: func(layout model.BackupLayout) []string {
				return []string{layout.FilesArtifact, layout.FilesChecksum, layout.ManifestJSON, layout.ManifestText}
			},
			wantCalls: []string{"dump_database"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := testBackupRequest(t)
			tt.mutate(&req)
			rt := testRuntime(true)

			result, err := testService(rt, store.FileStore{}).ExecuteBackup(context.Background(), req)
			if err != nil {
				t.Fatalf("ExecuteBackup returned error: %v", err)
			}
			if !result.OK {
				t.Fatalf("backup failed: %+v", result)
			}
			if result.Artifacts.ManifestJSON != "" || result.Artifacts.ManifestText != "" {
				t.Fatalf("partial backup exposed manifest artifacts: %+v", result.Artifacts)
			}
			layout := model.NewBackupLayout(req.BackupRoot, req.NamePrefix, req.CreatedAt)
			for _, path := range tt.wantArtifacts(layout) {
				assertFileExists(t, path)
			}
			for _, path := range tt.wantMissing(layout) {
				assertFileMissing(t, path)
			}
			if req.SkipDB {
				assertChecksumSidecar(t, layout.FilesArtifact, layout.FilesChecksum)
			}
			if req.SkipFiles {
				assertChecksumSidecar(t, layout.DBArtifact, layout.DBChecksum)
			}
			if err := rt.RequireCallOrder(tt.wantCalls); err != nil {
				t.Fatal(err)
			}
			assertNoTmpFiles(t, req.BackupRoot)
		})
	}
}

func TestBackupV2ValidationErrorsDoNotEnterMutatingPath(t *testing.T) {
	t.Run("both skip flags", func(t *testing.T) {
		req := testBackupRequest(t)
		req.SkipDB = true
		req.SkipFiles = true
		rt := testRuntime(true)

		result, err := testService(rt, store.FileStore{}).ExecuteBackup(context.Background(), req)
		if err == nil {
			t.Fatal("expected error")
		}
		if result.OK || result.ProcessExitCode != model.ExitUsageError {
			t.Fatalf("unexpected result: %+v", result)
		}
		if len(rt.Calls) != 0 {
			t.Fatalf("runtime was called: %v", rt.Calls)
		}
		assertDirEmpty(t, req.BackupRoot)
	})

	t.Run("missing backup root", func(t *testing.T) {
		req := testBackupRequest(t)
		req.BackupRoot = ""
		rt := testRuntime(true)

		result, err := testService(rt, store.FileStore{}).ExecuteBackup(context.Background(), req)
		if err == nil {
			t.Fatal("expected error")
		}
		if result.OK || result.ProcessExitCode != model.ExitValidationError {
			t.Fatalf("unexpected result: %+v", result)
		}
		if len(rt.Calls) != 0 {
			t.Fatalf("runtime was called: %v", rt.Calls)
		}
	})
}

func TestBackupV2RuntimePrepareFailureDoesNotCreateArtifacts(t *testing.T) {
	req := testBackupRequest(t)
	rt := testRuntime(true)
	rt.InspectErr = errors.New("prepare failed")

	result, err := testService(rt, store.FileStore{}).ExecuteBackup(context.Background(), req)
	if err == nil {
		t.Fatal("expected error")
	}
	if result.OK || result.ProcessExitCode != model.ExitExternalError {
		t.Fatalf("unexpected result: %+v", result)
	}
	if err := rt.RequireCallOrder([]string{"running_services"}); err != nil {
		t.Fatal(err)
	}
	layout := model.NewBackupLayout(req.BackupRoot, req.NamePrefix, req.CreatedAt)
	for _, path := range layout.CompleteSetPaths() {
		assertFileMissing(t, path)
	}
}

func TestBackupV2DatabaseFailureReturnsRuntimeAndDoesNotFinalize(t *testing.T) {
	req := testBackupRequest(t)
	rt := testRuntime(true)
	rt.DumpErr = errors.New("dump failed")

	result, err := testService(rt, store.FileStore{}).ExecuteBackup(context.Background(), req)
	if err == nil {
		t.Fatal("expected error")
	}
	if result.OK || result.ProcessExitCode != model.ExitExternalError {
		t.Fatalf("unexpected result: %+v", result)
	}
	if err := rt.RequireCallOrder([]string{"running_services", "stop_services", "dump_database", "start_services"}); err != nil {
		t.Fatal(err)
	}
	if !rt.AppServicesRunning {
		t.Fatalf("runtime was not returned")
	}
	layout := model.NewBackupLayout(req.BackupRoot, req.NamePrefix, req.CreatedAt)
	for _, path := range layout.CompleteSetPaths() {
		assertFileMissing(t, path)
	}
}

func TestBackupV2FilesFailureRemovesIncompleteFullSet(t *testing.T) {
	req := testBackupRequest(t)
	req.NoStop = true
	rt := testRuntime(true)
	rt.ArchiveErr = errors.New("archive failed")

	result, err := testService(rt, store.FileStore{}).ExecuteBackup(context.Background(), req)
	if err == nil {
		t.Fatal("expected error")
	}
	if result.OK {
		t.Fatalf("backup reported success: %+v", result)
	}
	if !strings.Contains(err.Error(), "контракт helper fallback не задан") {
		t.Fatalf("expected explicit missing helper contract error, got %v", err)
	}
	if err := rt.RequireCallOrder([]string{"dump_database", "archive_files"}); err != nil {
		t.Fatal(err)
	}
	layout := model.NewBackupLayout(req.BackupRoot, req.NamePrefix, req.CreatedAt)
	for _, path := range layout.CompleteSetPaths() {
		assertFileMissing(t, path)
	}
	assertNoTmpFiles(t, req.BackupRoot)
}

func TestBackupV2FilesArchiveFallsBackToExplicitHelperContract(t *testing.T) {
	req := testBackupRequest(t)
	req.NoStop = true
	req.HelperArchive = model.HelperArchiveContract{Image: "registry.example.com/espops-helper:1.0"}
	rt := testRuntime(true)
	rt.ArchiveErr = errors.New("local archive failed")
	rt.HelperFilesArchive = tarGzBytes(map[string]string{"espo/helper.txt": "helper\n"})

	result, err := testService(rt, store.FileStore{}).ExecuteBackup(context.Background(), req)
	if err != nil {
		t.Fatalf("ExecuteBackup returned error: %v", err)
	}
	if !result.OK || !result.Details.Ready {
		t.Fatalf("backup failed: %+v", result)
	}
	if err := rt.RequireCallOrder([]string{"dump_database", "archive_files", "archive_files_helper"}); err != nil {
		t.Fatal(err)
	}
	if result.Details.Warnings != 1 || len(result.Warnings) != 1 {
		t.Fatalf("expected one helper warning, got details=%+v warnings=%v", result.Details, result.Warnings)
	}
	if !strings.Contains(result.Warnings[0], "helper fallback") {
		t.Fatalf("unexpected warning: %v", result.Warnings)
	}

	layout := model.NewBackupLayout(req.BackupRoot, req.NamePrefix, req.CreatedAt)
	assertFileExists(t, layout.DBArtifact)
	assertFileExists(t, layout.DBChecksum)
	assertFileExists(t, layout.FilesArtifact)
	assertFileExists(t, layout.FilesChecksum)
	assertFileExists(t, layout.ManifestJSON)
	assertFileExists(t, layout.ManifestText)
	assertTarEntries(t, layout.FilesArtifact, []string{"espo/", "espo/helper.txt"})
	assertChecksumSidecar(t, layout.FilesArtifact, layout.FilesChecksum)
	assertManifestJSON(t, layout.ManifestJSON, req, layout)
}

func TestBackupV2FilesArchiveFailsClosedWhenHelperPathFails(t *testing.T) {
	req := testBackupRequest(t)
	req.NoStop = true
	req.HelperArchive = model.HelperArchiveContract{Image: "registry.example.com/espops-helper:1.0"}
	rt := testRuntime(true)
	rt.ArchiveErr = errors.New("local archive failed")
	rt.HelperArchiveErr = errors.New("helper archive failed")

	result, err := testService(rt, store.FileStore{}).ExecuteBackup(context.Background(), req)
	if err == nil {
		t.Fatal("expected error")
	}
	if result.OK {
		t.Fatalf("backup reported success: %+v", result)
	}
	if err := rt.RequireCallOrder([]string{"dump_database", "archive_files", "archive_files_helper"}); err != nil {
		t.Fatal(err)
	}
	layout := model.NewBackupLayout(req.BackupRoot, req.NamePrefix, req.CreatedAt)
	for _, path := range layout.CompleteSetPaths() {
		assertFileMissing(t, path)
	}
	assertNoTmpFiles(t, req.BackupRoot)
}

func TestBackupV2FinalizeFailureBlocksRetentionAndRemovesIncompleteSet(t *testing.T) {
	req := testBackupRequest(t)
	req.NoStop = true
	rt := testRuntime(true)
	base := store.FileStore{}
	fs := &failingStore{Store: base, failManifestJSON: true}

	result, err := testService(rt, fs).ExecuteBackup(context.Background(), req)
	if err == nil {
		t.Fatal("expected error")
	}
	if result.OK || result.ProcessExitCode != model.ExitIOError {
		t.Fatalf("unexpected result: %+v", result)
	}
	if fs.retentionListed {
		t.Fatalf("retention ran after finalize failure")
	}
	layout := model.NewBackupLayout(req.BackupRoot, req.NamePrefix, req.CreatedAt)
	for _, path := range layout.CompleteSetPaths() {
		assertFileMissing(t, path)
	}
}

func TestBackupV2RetentionFailureDoesNotReportSuccess(t *testing.T) {
	req := testBackupRequest(t)
	req.NoStop = true
	rt := testRuntime(true)
	old := model.NewBackupLayoutForStamp(req.BackupRoot, req.NamePrefix, "2026-04-01_00-00-00")
	if err := os.MkdirAll(old.DBArtifact, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(old.DBArtifact, "blocker.txt"), []byte("block"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{old.DBChecksum, old.FilesArtifact, old.FilesChecksum, old.ManifestJSON, old.ManifestText} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("old"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	result, err := testService(rt, store.FileStore{}).ExecuteBackup(context.Background(), req)
	if err == nil {
		t.Fatal("expected error")
	}
	if result.OK || result.ProcessExitCode != model.ExitIOError {
		t.Fatalf("unexpected result: %+v", result)
	}
	layout := model.NewBackupLayout(req.BackupRoot, req.NamePrefix, req.CreatedAt)
	assertFileExists(t, layout.ManifestJSON)
	assertFileExists(t, layout.DBChecksum)
	assertFileExists(t, layout.FilesChecksum)
	assertFileExists(t, filepath.Join(old.DBArtifact, "blocker.txt"))
}

func TestBackupV2RuntimeReturnFailureFailsAfterArtifactsRemain(t *testing.T) {
	req := testBackupRequest(t)
	rt := testRuntime(true)
	rt.StartErr = errors.New("start failed")

	result, err := testService(rt, store.FileStore{}).ExecuteBackup(context.Background(), req)
	if err == nil {
		t.Fatal("expected error")
	}
	if result.OK || result.ProcessExitCode != model.ExitExternalError {
		t.Fatalf("unexpected result: %+v", result)
	}
	if err := rt.RequireCallOrder([]string{"running_services", "stop_services", "dump_database", "archive_files", "start_services"}); err != nil {
		t.Fatal(err)
	}
	layout := model.NewBackupLayout(req.BackupRoot, req.NamePrefix, req.CreatedAt)
	assertFileExists(t, layout.ManifestJSON)
	assertFileExists(t, layout.DBChecksum)
	assertFileExists(t, layout.FilesChecksum)
}

type failingStore struct {
	model.Store
	failManifestJSON bool
	retentionListed  bool
}

func (s *failingStore) WriteManifestJSON(ctx context.Context, path string, manifest model.CompleteManifest) error {
	if s.failManifestJSON {
		return errors.New("manifest json failed")
	}
	return s.Store.WriteManifestJSON(ctx, path, manifest)
}

func (s *failingStore) ListBackupGroups(ctx context.Context, root string) ([]model.BackupGroup, error) {
	s.retentionListed = true
	return s.Store.ListBackupGroups(ctx, root)
}

func testService(rt model.Runtime, st model.Store) v2app.BackupService {
	return v2app.NewBackupService(v2app.BackupDependencies{Runtime: rt, Store: st})
}

func testBackupRequest(t *testing.T) model.BackupRequest {
	t.Helper()
	root := t.TempDir()
	return model.BackupRequest{
		Scope:         "dev",
		ProjectDir:    filepath.Join(root, "project"),
		ComposeFile:   filepath.Join(root, "project", "compose.yaml"),
		EnvFile:       filepath.Join(root, "project", ".env.dev"),
		BackupRoot:    filepath.Join(root, "backup"),
		StorageDir:    filepath.Join(root, "project", "runtime", "dev", "espo"),
		NamePrefix:    "espocrm-test-dev",
		RetentionDays: 7,
		CreatedAt:     time.Date(2026, 4, 15, 11, 0, 0, 0, time.UTC),
		DBService:     "db",
		DBUser:        "espocrm",
		DBPassword:    "secret",
		DBName:        "espocrm",
		Metadata: model.BackupMetadata{
			ComposeProject: "espocrm-test",
			EnvFileName:    ".env.dev",
			EspoCRMImage:   "espocrm/espocrm:9.3.4-apache",
			MariaDBTag:     "10.11",
		},
	}
}

func testRuntime(running bool) *v2runtime.Static {
	return &v2runtime.Static{
		AppServicesRunning: running,
		DBDump:             gzipBytes("select 1;\n"),
		FilesArchive:       tarGzBytes(map[string]string{"espo/file.txt": "content\n"}),
	}
}

func gzipBytes(body string) []byte {
	var buf bytes.Buffer
	w := gzip.NewWriter(&buf)
	_, _ = w.Write([]byte(body))
	_ = w.Close()
	return buf.Bytes()
}

func tarGzBytes(files map[string]string) []byte {
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	dirs := map[string]struct{}{}
	for name := range files {
		dir := filepath.ToSlash(filepath.Dir(name)) + "/"
		dirs[dir] = struct{}{}
	}
	var dirNames []string
	for dir := range dirs {
		dirNames = append(dirNames, dir)
	}
	sort.Strings(dirNames)
	for _, dir := range dirNames {
		_ = tw.WriteHeader(&tar.Header{Name: dir, Typeflag: tar.TypeDir, Mode: 0o755})
	}
	var names []string
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		body := []byte(files[name])
		_ = tw.WriteHeader(&tar.Header{Name: filepath.ToSlash(name), Size: int64(len(body)), Mode: 0o644})
		_, _ = tw.Write(body)
	}
	_ = tw.Close()
	_ = gz.Close()
	return buf.Bytes()
}

func assertSteps(t *testing.T, result model.BackupResult, want []model.Step) {
	t.Helper()
	if !reflect.DeepEqual(result.Items, want) {
		t.Fatalf("steps mismatch:\n got: %+v\nwant: %+v", result.Items, want)
	}
}

func assertFileExists(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected file %s: %v", path, err)
	}
}

func assertFileMissing(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); err == nil {
		t.Fatalf("expected %s to be absent", path)
	} else if !os.IsNotExist(err) {
		t.Fatalf("stat %s: %v", path, err)
	}
}

func assertDirEmpty(t *testing.T, path string) {
	t.Helper()
	entries, err := os.ReadDir(path)
	if err != nil && !os.IsNotExist(err) {
		t.Fatalf("read dir %s: %v", path, err)
	}
	if len(entries) != 0 {
		t.Fatalf("expected empty dir %s, got %d entries", path, len(entries))
	}
}

func assertNoTmpFiles(t *testing.T, root string) {
	t.Helper()
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".tmp") {
			t.Fatalf("tmp file remains: %s", path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func assertGzipPayload(t *testing.T, path, want string) {
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
	if string(raw) != want {
		t.Fatalf("gzip payload mismatch: got %q want %q", string(raw), want)
	}
}

func assertTarEntries(t *testing.T, path string, want []string) {
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
	var got []string
	for {
		header, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		got = append(got, header.Name)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("tar entries mismatch: got %v want %v", got, want)
	}
}

func assertChecksumSidecar(t *testing.T, artifactPath, sidecarPath string) {
	t.Helper()
	raw, err := os.ReadFile(artifactPath)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(raw)
	want := hex.EncodeToString(sum[:]) + "  " + filepath.Base(artifactPath) + "\n"
	got, err := os.ReadFile(sidecarPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != want {
		t.Fatalf("checksum sidecar mismatch: got %q want %q", string(got), want)
	}
}

func assertManifestJSON(t *testing.T, path string, req model.BackupRequest, layout model.BackupLayout) {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var manifest model.CompleteManifest
	if err := json.Unmarshal(raw, &manifest); err != nil {
		t.Fatal(err)
	}
	if err := manifest.ValidateComplete(); err != nil {
		t.Fatal(err)
	}
	if manifest.Version != 1 || manifest.Scope != req.Scope || manifest.CreatedAt != req.CreatedAt.UTC().Format(time.RFC3339) {
		t.Fatalf("unexpected manifest identity: %+v", manifest)
	}
	if manifest.Artifacts.DBBackup != filepath.Base(layout.DBArtifact) || manifest.Artifacts.FilesBackup != filepath.Base(layout.FilesArtifact) {
		t.Fatalf("manifest artifact names are not canonical basenames: %+v", manifest.Artifacts)
	}
}

func assertManifestText(t *testing.T, path string, wantFragments []string) {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	for _, fragment := range wantFragments {
		if !strings.Contains(text, fragment) {
			t.Fatalf("manifest text missing %q in:\n%s", fragment, text)
		}
	}
}

func createRetentionSet(t *testing.T, root, prefix, stamp string) {
	t.Helper()
	layout := model.NewBackupLayoutForStamp(root, prefix, stamp)
	for _, path := range layout.CompleteSetPaths() {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("old"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func assertBackupSetAbsent(t *testing.T, root, prefix, stamp string) {
	t.Helper()
	layout := model.NewBackupLayoutForStamp(root, prefix, stamp)
	for _, path := range layout.CompleteSetPaths() {
		assertFileMissing(t, path)
	}
}

func assertBackupSetPresent(t *testing.T, root, prefix, stamp string) {
	t.Helper()
	layout := model.NewBackupLayoutForStamp(root, prefix, stamp)
	for _, path := range layout.CompleteSetPaths() {
		assertFileExists(t, path)
	}
}
