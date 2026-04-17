package restore

import (
	"errors"
	"os"

	"github.com/lazuale/espocrm-ops/internal/contract/apperr"
	"github.com/lazuale/espocrm-ops/internal/platform/backupstore"
	platformconfig "github.com/lazuale/espocrm-ops/internal/platform/config"
	platformdocker "github.com/lazuale/espocrm-ops/internal/platform/docker"
	platformfs "github.com/lazuale/espocrm-ops/internal/platform/fs"
)

type PreflightError struct {
	Err error
}

func (e PreflightError) Error() string {
	return e.Err.Error()
}

func (e PreflightError) Unwrap() error {
	return e.Err
}

func (e PreflightError) ErrorKind() apperr.Kind {
	return classifyPreflightError(e.Err).kind
}

func (e PreflightError) ErrorCode() string {
	if code, ok := apperr.CodeOf(e.Err); ok {
		return code
	}

	return classifyPreflightError(e.Err).code
}

type LockError struct {
	Err error
}

func (e LockError) Error() string {
	return e.Err.Error()
}

func (e LockError) Unwrap() error {
	return e.Err
}

func (e LockError) ErrorKind() apperr.Kind {
	return apperr.KindConflict
}

func (e LockError) ErrorCode() string {
	return "lock_acquire_failed"
}

type OperationError struct {
	Err          error
	FallbackCode string
}

func (e OperationError) Error() string {
	return e.Err.Error()
}

func (e OperationError) Unwrap() error {
	return e.Err
}

func (e OperationError) ErrorKind() apperr.Kind {
	return classifyOperationError(e.Err, e.FallbackCode).kind
}

func (e OperationError) ErrorCode() string {
	if code, ok := apperr.CodeOf(e.Err); ok {
		return code
	}

	return classifyOperationError(e.Err, e.FallbackCode).code
}

type preflightClassification struct {
	kind apperr.Kind
	code string
}

type operationClassification struct {
	kind apperr.Kind
	code string
}

func classifyOperationError(err error, fallbackCode string) operationClassification {
	var pathErr *os.PathError
	if errors.As(err, &pathErr) {
		return operationClassification{
			kind: apperr.KindIO,
			code: "filesystem_error",
		}
	}

	var linkErr *os.LinkError
	if errors.As(err, &linkErr) {
		return operationClassification{
			kind: apperr.KindIO,
			code: "filesystem_error",
		}
	}

	var ensureDirErr platformfs.EnsureDirError
	if errors.As(err, &ensureDirErr) {
		return operationClassification{
			kind: apperr.KindIO,
			code: "filesystem_error",
		}
	}

	var stageCreateRootErr platformfs.StageCreateRootError
	if errors.As(err, &stageCreateRootErr) {
		return operationClassification{
			kind: apperr.KindIO,
			code: "filesystem_error",
		}
	}

	var stagePrepareDirErr platformfs.StagePrepareDirError
	if errors.As(err, &stagePrepareDirErr) {
		return operationClassification{
			kind: apperr.KindIO,
			code: "filesystem_error",
		}
	}

	var stageReadErr platformfs.StageReadError
	if errors.As(err, &stageReadErr) {
		return operationClassification{
			kind: apperr.KindIO,
			code: "filesystem_error",
		}
	}

	var treeStatErr platformfs.TreeStatError
	if errors.As(err, &treeStatErr) {
		return operationClassification{
			kind: apperr.KindIO,
			code: "filesystem_error",
		}
	}

	var treeRenameErr platformfs.TreeRenameError
	if errors.As(err, &treeRenameErr) {
		return operationClassification{
			kind: apperr.KindIO,
			code: "filesystem_error",
		}
	}

	var dbClientErr platformdocker.DBClientDetectionError
	if errors.As(err, &dbClientErr) {
		return operationClassification{
			kind: apperr.KindExternal,
			code: dbClientErr.ErrorCode(),
		}
	}

	var dbExecErr platformdocker.DBExecutionError
	if errors.As(err, &dbExecErr) {
		return operationClassification{
			kind: apperr.KindExternal,
			code: dbExecErr.ErrorCode(),
		}
	}

	if fallbackCode == "" {
		fallbackCode = "restore_failed"
	}

	return operationClassification{
		kind: apperr.KindRestore,
		code: fallbackCode,
	}
}

func classifyPreflightError(err error) preflightClassification {
	var manifestErr backupstore.ManifestError
	if errors.As(err, &manifestErr) {
		return preflightClassification{
			kind: apperr.KindManifest,
			code: "manifest_invalid",
		}
	}

	var verificationErr backupstore.ValidationError
	if errors.As(err, &verificationErr) {
		return preflightClassification{
			kind: apperr.KindValidation,
			code: "backup_verification_failed",
		}
	}

	var passwordFileReadErr platformconfig.PasswordFileReadError
	if errors.As(err, &passwordFileReadErr) {
		return preflightClassification{
			kind: apperr.KindIO,
			code: "filesystem_error",
		}
	}

	var passwordConflictErr platformconfig.PasswordSourceConflictError
	if errors.As(err, &passwordConflictErr) {
		return preflightClassification{
			kind: apperr.KindValidation,
			code: "preflight_failed",
		}
	}

	var passwordEmptyErr platformconfig.PasswordFileEmptyError
	if errors.As(err, &passwordEmptyErr) {
		return preflightClassification{
			kind: apperr.KindValidation,
			code: "preflight_failed",
		}
	}

	var passwordRequiredErr platformconfig.PasswordRequiredError
	if errors.As(err, &passwordRequiredErr) {
		return preflightClassification{
			kind: apperr.KindValidation,
			code: "preflight_failed",
		}
	}

	var fileStatErr platformfs.PathStatError
	if errors.As(err, &fileStatErr) {
		return preflightClassification{
			kind: apperr.KindIO,
			code: "filesystem_error",
		}
	}

	var ensureDirErr platformfs.EnsureDirError
	if errors.As(err, &ensureDirErr) {
		return preflightClassification{
			kind: apperr.KindIO,
			code: "filesystem_error",
		}
	}

	var dirCreateTempErr platformfs.DirCreateTempError
	if errors.As(err, &dirCreateTempErr) {
		return preflightClassification{
			kind: apperr.KindIO,
			code: "filesystem_error",
		}
	}

	var dirWriteTestErr platformfs.DirWriteTestError
	if errors.As(err, &dirWriteTestErr) {
		return preflightClassification{
			kind: apperr.KindIO,
			code: "filesystem_error",
		}
	}

	var dirCloseTestErr platformfs.DirCloseTestError
	if errors.As(err, &dirCloseTestErr) {
		return preflightClassification{
			kind: apperr.KindIO,
			code: "filesystem_error",
		}
	}

	var freeSpaceCheckErr platformfs.FreeSpaceCheckError
	if errors.As(err, &freeSpaceCheckErr) {
		return preflightClassification{
			kind: apperr.KindIO,
			code: "filesystem_error",
		}
	}

	var fileIsDirectoryErr platformfs.FileIsDirectoryError
	if errors.As(err, &fileIsDirectoryErr) {
		return preflightClassification{
			kind: apperr.KindValidation,
			code: "preflight_failed",
		}
	}

	var fileEmptyErr platformfs.FileEmptyError
	if errors.As(err, &fileEmptyErr) {
		return preflightClassification{
			kind: apperr.KindValidation,
			code: "preflight_failed",
		}
	}

	var insufficientSpaceErr platformfs.InsufficientFreeSpaceError
	if errors.As(err, &insufficientSpaceErr) {
		return preflightClassification{
			kind: apperr.KindValidation,
			code: "preflight_failed",
		}
	}

	var dockerUnavailable platformdocker.UnavailableError
	if errors.As(err, &dockerUnavailable) {
		return preflightClassification{
			kind: apperr.KindExternal,
			code: dockerUnavailable.ErrorCode(),
		}
	}

	var containerInspectErr platformdocker.ContainerInspectError
	if errors.As(err, &containerInspectErr) {
		return preflightClassification{
			kind: apperr.KindExternal,
			code: containerInspectErr.ErrorCode(),
		}
	}

	var containerNotRunningErr platformdocker.ContainerNotRunningError
	if errors.As(err, &containerNotRunningErr) {
		return preflightClassification{
			kind: apperr.KindExternal,
			code: containerNotRunningErr.ErrorCode(),
		}
	}

	return preflightClassification{
		kind: apperr.KindValidation,
		code: "preflight_failed",
	}
}
