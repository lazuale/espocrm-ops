package fs

import "fmt"

type PathStatError struct {
	Label string
	Path  string
	Err   error
}

func (e PathStatError) Error() string {
	return fmt.Sprintf("stat %s: %v", e.Label, e.Err)
}

func (e PathStatError) Unwrap() error {
	return e.Err
}

func (e PathStatError) ErrorCode() string {
	return "filesystem_error"
}

type FileIsDirectoryError struct {
	Label string
	Path  string
}

func (e FileIsDirectoryError) Error() string {
	return fmt.Sprintf("%s is a directory: %s", e.Label, e.Path)
}

func (e FileIsDirectoryError) ErrorCode() string {
	return "preflight_failed"
}

type FileEmptyError struct {
	Label string
	Path  string
}

func (e FileEmptyError) Error() string {
	return fmt.Sprintf("%s file is empty: %s", e.Label, e.Path)
}

func (e FileEmptyError) ErrorCode() string {
	return "preflight_failed"
}

type EnsureDirError struct {
	Path string
	Err  error
}

func (e EnsureDirError) Error() string {
	return fmt.Sprintf("ensure target parent dir: %v", e.Err)
}

func (e EnsureDirError) Unwrap() error {
	return e.Err
}

func (e EnsureDirError) ErrorCode() string {
	return "filesystem_error"
}

type DirCreateTempError struct {
	Path string
	Err  error
}

func (e DirCreateTempError) Error() string {
	return fmt.Sprintf("target parent dir is not writable: %v", e.Err)
}

func (e DirCreateTempError) Unwrap() error {
	return e.Err
}

func (e DirCreateTempError) ErrorCode() string {
	return "filesystem_error"
}

type DirWriteTestError struct {
	Path string
	Err  error
}

func (e DirWriteTestError) Error() string {
	return fmt.Sprintf("write target parent test file: %v", e.Err)
}

func (e DirWriteTestError) Unwrap() error {
	return e.Err
}

func (e DirWriteTestError) ErrorCode() string {
	return "filesystem_error"
}

type DirCloseTestError struct {
	Path string
	Err  error
}

func (e DirCloseTestError) Error() string {
	return fmt.Sprintf("close target parent test file: %v", e.Err)
}

func (e DirCloseTestError) Unwrap() error {
	return e.Err
}

func (e DirCloseTestError) ErrorCode() string {
	return "filesystem_error"
}

type FreeSpaceCheckError struct {
	Path string
	Err  error
}

func (e FreeSpaceCheckError) Error() string {
	return fmt.Sprintf("check free space: %v", e.Err)
}

func (e FreeSpaceCheckError) Unwrap() error {
	return e.Err
}

func (e FreeSpaceCheckError) ErrorCode() string {
	return "filesystem_error"
}

type InsufficientFreeSpaceError struct {
	Path           string
	NeededBytes    uint64
	AvailableBytes uint64
}

func (e InsufficientFreeSpaceError) Error() string {
	return fmt.Sprintf("not enough free space in %s: need at least %d bytes, available %d bytes", e.Path, e.NeededBytes, e.AvailableBytes)
}

func (e InsufficientFreeSpaceError) ErrorCode() string {
	return "preflight_failed"
}

type StageCreateRootError struct {
	Path string
	Err  error
}

func (e StageCreateRootError) Error() string {
	return fmt.Sprintf("create sibling stage root: %v", e.Err)
}

func (e StageCreateRootError) Unwrap() error {
	return e.Err
}

func (e StageCreateRootError) ErrorCode() string {
	return "filesystem_error"
}

type StagePrepareDirError struct {
	Path string
	Err  error
}

func (e StagePrepareDirError) Error() string {
	return fmt.Sprintf("create stage dir: %v", e.Err)
}

func (e StagePrepareDirError) Unwrap() error {
	return e.Err
}

func (e StagePrepareDirError) ErrorCode() string {
	return "filesystem_error"
}

type StageReadError struct {
	Path string
	Err  error
}

func (e StageReadError) Error() string {
	return fmt.Sprintf("read stage dir: %v", e.Err)
}

func (e StageReadError) Unwrap() error {
	return e.Err
}

func (e StageReadError) ErrorCode() string {
	return "filesystem_error"
}

type StageEmptyError struct {
	Path string
}

func (e StageEmptyError) Error() string {
	return "archive is empty"
}

func (e StageEmptyError) ErrorCode() string {
	return "restore_files_failed"
}

type StageMixedRootError struct {
	Path       string
	TargetBase string
}

func (e StageMixedRootError) Error() string {
	return fmt.Sprintf("archive mixes target root directory %q with sibling entries", e.TargetBase)
}

func (e StageMixedRootError) ErrorCode() string {
	return "restore_files_failed"
}

type StageRootMismatchError struct {
	Path       string
	TargetBase string
}

func (e StageRootMismatchError) Error() string {
	return fmt.Sprintf("archive root must be exactly %q", e.TargetBase)
}

func (e StageRootMismatchError) ErrorCode() string {
	return "restore_files_failed"
}

type PreparedDirNotDirectoryError struct {
	Path string
}

func (e PreparedDirNotDirectoryError) Error() string {
	return fmt.Sprintf("prepared dir is not a directory: %s", e.Path)
}

func (e PreparedDirNotDirectoryError) ErrorCode() string {
	return "restore_files_failed"
}

type TreeStatError struct {
	Path string
	Err  error
}

func (e TreeStatError) Error() string {
	return fmt.Sprintf("stat target dir: %v", e.Err)
}

func (e TreeStatError) Unwrap() error {
	return e.Err
}

func (e TreeStatError) ErrorCode() string {
	return "filesystem_error"
}

type TreeRenameError struct {
	Action string
	From   string
	To     string
	Err    error
}

func (e TreeRenameError) Error() string {
	return fmt.Sprintf("%s: %v", e.Action, e.Err)
}

func (e TreeRenameError) Unwrap() error {
	return e.Err
}

func (e TreeRenameError) ErrorCode() string {
	return "filesystem_error"
}

type ArchiveReadError struct {
	Path string
	Err  error
}

func (e ArchiveReadError) Error() string {
	return fmt.Sprintf("read archive %s: %v", e.Path, e.Err)
}

func (e ArchiveReadError) Unwrap() error {
	return e.Err
}

func (e ArchiveReadError) ErrorCode() string {
	return "restore_files_failed"
}

type ArchiveEntryEscapeError struct {
	ArchivePath string
	EntryName   string
}

func (e ArchiveEntryEscapeError) Error() string {
	return fmt.Sprintf("archive entry escapes destination: %s", e.EntryName)
}

func (e ArchiveEntryEscapeError) ErrorCode() string {
	return "restore_files_failed"
}

type ArchiveUnexpectedEntryTypeError struct {
	ArchivePath string
	EntryName   string
	Typeflag    byte
}

func (e ArchiveUnexpectedEntryTypeError) Error() string {
	return fmt.Sprintf("unexpected tar entry type after validation: %d", e.Typeflag)
}

func (e ArchiveUnexpectedEntryTypeError) ErrorCode() string {
	return "restore_files_failed"
}

type ArchiveEntryConflictError struct {
	ArchivePath  string
	EntryName    string
	ConflictPath string
	Reason       string
}

func (e ArchiveEntryConflictError) Error() string {
	if e.Reason != "" {
		return fmt.Sprintf("archive entry conflicts with existing path %s: %s", e.EntryName, e.Reason)
	}

	return fmt.Sprintf("archive entry conflicts with existing path: %s", e.EntryName)
}

func (e ArchiveEntryConflictError) ErrorCode() string {
	return "restore_files_failed"
}

type ArchiveEmptyError struct {
	ArchivePath string
}

func (e ArchiveEmptyError) Error() string {
	return "archive is empty"
}

func (e ArchiveEmptyError) ErrorCode() string {
	return "restore_files_failed"
}
