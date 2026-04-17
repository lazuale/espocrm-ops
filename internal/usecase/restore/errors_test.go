package restore

import (
	"errors"
	"os"
	"testing"

	"github.com/lazuale/espocrm-ops/internal/contract/apperr"
	"github.com/lazuale/espocrm-ops/internal/platform/backupstore"
	platformconfig "github.com/lazuale/espocrm-ops/internal/platform/config"
	platformdocker "github.com/lazuale/espocrm-ops/internal/platform/docker"
	platformfs "github.com/lazuale/espocrm-ops/internal/platform/fs"
)

func TestPreflightError_ClassifiesTypedContract(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		wantKind apperr.Kind
		wantCode string
	}{
		{
			name:     "manifest",
			err:      PreflightError{Err: backupstore.ManifestError{Err: errors.New("bad manifest")}},
			wantKind: apperr.KindManifest,
			wantCode: "manifest_invalid",
		},
		{
			name:     "backup verification",
			err:      PreflightError{Err: backupstore.ValidationError{Err: errors.New("checksum mismatch")}},
			wantKind: apperr.KindValidation,
			wantCode: "backup_verification_failed",
		},
		{
			name:     "docker unavailable",
			err:      PreflightError{Err: platformdocker.UnavailableError{Err: errors.New("docker missing")}},
			wantKind: apperr.KindExternal,
			wantCode: "docker_unavailable",
		},
		{
			name:     "password file read",
			err:      PreflightError{Err: platformconfig.PasswordFileReadError{Path: "/run/secrets/db", Err: errors.New("permission denied")}},
			wantKind: apperr.KindIO,
			wantCode: "filesystem_error",
		},
		{
			name:     "password source conflict",
			err:      PreflightError{Err: platformconfig.PasswordSourceConflictError{Label: "db password"}},
			wantKind: apperr.KindValidation,
			wantCode: "preflight_failed",
		},
		{
			name:     "password file empty",
			err:      PreflightError{Err: platformconfig.PasswordFileEmptyError{Path: "/run/secrets/db"}},
			wantKind: apperr.KindValidation,
			wantCode: "preflight_failed",
		},
		{
			name:     "password required",
			err:      PreflightError{Err: platformconfig.PasswordRequiredError{Label: "db password"}},
			wantKind: apperr.KindValidation,
			wantCode: "preflight_failed",
		},
		{
			name:     "backup stat failure",
			err:      PreflightError{Err: platformfs.PathStatError{Label: "db backup", Path: "/backups/db.sql.gz", Err: errors.New("permission denied")}},
			wantKind: apperr.KindIO,
			wantCode: "filesystem_error",
		},
		{
			name:     "target parent ensure failure",
			err:      PreflightError{Err: platformfs.EnsureDirError{Path: "/srv/storage", Err: errors.New("not a directory")}},
			wantKind: apperr.KindIO,
			wantCode: "filesystem_error",
		},
		{
			name:     "target parent not writable",
			err:      PreflightError{Err: platformfs.DirCreateTempError{Path: "/srv/storage", Err: errors.New("permission denied")}},
			wantKind: apperr.KindIO,
			wantCode: "filesystem_error",
		},
		{
			name:     "file is directory",
			err:      PreflightError{Err: platformfs.FileIsDirectoryError{Label: "files backup", Path: "/backups/files.tar.gz"}},
			wantKind: apperr.KindValidation,
			wantCode: "preflight_failed",
		},
		{
			name:     "file is empty",
			err:      PreflightError{Err: platformfs.FileEmptyError{Label: "files backup", Path: "/backups/files.tar.gz"}},
			wantKind: apperr.KindValidation,
			wantCode: "preflight_failed",
		},
		{
			name:     "free space check failure",
			err:      PreflightError{Err: platformfs.FreeSpaceCheckError{Path: "/srv/storage", Err: errors.New("statfs failed")}},
			wantKind: apperr.KindIO,
			wantCode: "filesystem_error",
		},
		{
			name:     "insufficient free space",
			err:      PreflightError{Err: platformfs.InsufficientFreeSpaceError{Path: "/srv/storage", NeededBytes: 100, AvailableBytes: 50}},
			wantKind: apperr.KindValidation,
			wantCode: "preflight_failed",
		},
		{
			name:     "container inspect failed",
			err:      PreflightError{Err: platformdocker.ContainerInspectError{Container: "db", Err: errors.New("inspect failed")}},
			wantKind: apperr.KindExternal,
			wantCode: "container_inspect_failed",
		},
		{
			name:     "container not running",
			err:      PreflightError{Err: platformdocker.ContainerNotRunningError{Container: "db"}},
			wantKind: apperr.KindExternal,
			wantCode: "container_not_running",
		},
		{
			name:     "generic preflight",
			err:      PreflightError{Err: errors.New("generic preflight failure")},
			wantKind: apperr.KindValidation,
			wantCode: "preflight_failed",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if kind := tc.err.(interface{ ErrorKind() apperr.Kind }).ErrorKind(); kind != tc.wantKind {
				t.Fatalf("expected kind %q, got %q", tc.wantKind, kind)
			}
			if code := tc.err.(interface{ ErrorCode() string }).ErrorCode(); code != tc.wantCode {
				t.Fatalf("expected code %q, got %q", tc.wantCode, code)
			}
		})
	}
}

func TestOperationError_ClassifiesTypedContract(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		wantKind apperr.Kind
		wantCode string
	}{
		{
			name:     "docker db client detection",
			err:      OperationError{Err: platformdocker.DBClientDetectionError{Container: "db", Err: errors.New("mysql version failed")}},
			wantKind: apperr.KindExternal,
			wantCode: "restore_db_failed",
		},
		{
			name:     "docker db execution",
			err:      OperationError{Err: platformdocker.DBExecutionError{Action: "restore mysql dump", Container: "db", Err: errors.New("permission denied")}},
			wantKind: apperr.KindExternal,
			wantCode: "restore_db_failed",
		},
		{
			name:     "filesystem path error",
			err:      OperationError{Err: &os.PathError{Op: "open", Path: "/backups/db.sql.gz", Err: errors.New("permission denied")}},
			wantKind: apperr.KindIO,
			wantCode: "filesystem_error",
		},
		{
			name:     "filesystem link error",
			err:      OperationError{Err: &os.LinkError{Op: "rename", Old: "/tmp/new", New: "/srv/storage", Err: errors.New("permission denied")}, FallbackCode: "restore_files_failed"},
			wantKind: apperr.KindIO,
			wantCode: "filesystem_error",
		},
		{
			name:     "typed stage read error",
			err:      OperationError{Err: platformfs.StageReadError{Path: "/tmp/stage", Err: errors.New("permission denied")}, FallbackCode: "restore_files_failed"},
			wantKind: apperr.KindIO,
			wantCode: "filesystem_error",
		},
		{
			name:     "typed archive read error",
			err:      OperationError{Err: platformfs.ArchiveReadError{Path: "/backups/files.tar.gz", Err: errors.New("gzip: invalid header")}, FallbackCode: "restore_files_failed"},
			wantKind: apperr.KindRestore,
			wantCode: "restore_files_failed",
		},
		{
			name:     "typed archive semantic error",
			err:      OperationError{Err: platformfs.ArchiveEntryEscapeError{ArchivePath: "/backups/files.tar.gz", EntryName: "../escape.txt"}, FallbackCode: "restore_files_failed"},
			wantKind: apperr.KindRestore,
			wantCode: "restore_files_failed",
		},
		{
			name:     "typed archive conflict error",
			err:      OperationError{Err: platformfs.ArchiveEntryConflictError{ArchivePath: "/backups/files.tar.gz", EntryName: "storage/a.txt/b.txt", ConflictPath: "/tmp/stage/storage/a.txt", Reason: "parent path resolves through a file"}, FallbackCode: "restore_files_failed"},
			wantKind: apperr.KindRestore,
			wantCode: "restore_files_failed",
		},
		{
			name:     "files runtime fallback",
			err:      OperationError{Err: platformfs.StageRootMismatchError{Path: "/tmp/stage", TargetBase: "storage"}, FallbackCode: "restore_files_failed"},
			wantKind: apperr.KindRestore,
			wantCode: "restore_files_failed",
		},
		{
			name:     "typed files runtime code without fallback",
			err:      OperationError{Err: platformfs.StageRootMismatchError{Path: "/tmp/stage", TargetBase: "storage"}},
			wantKind: apperr.KindRestore,
			wantCode: "restore_files_failed",
		},
		{
			name:     "generic restore",
			err:      OperationError{Err: errors.New("generic restore failure")},
			wantKind: apperr.KindRestore,
			wantCode: "restore_failed",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if kind := tc.err.(interface{ ErrorKind() apperr.Kind }).ErrorKind(); kind != tc.wantKind {
				t.Fatalf("expected kind %q, got %q", tc.wantKind, kind)
			}
			if code := tc.err.(interface{ ErrorCode() string }).ErrorCode(); code != tc.wantCode {
				t.Fatalf("expected code %q, got %q", tc.wantCode, code)
			}
		})
	}
}
