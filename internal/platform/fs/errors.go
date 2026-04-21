package fs

import "fmt"

type pathStatError struct {
	Label string
	Path  string
	Err   error
}

func (e pathStatError) Error() string {
	return fmt.Sprintf("stat %s: %v", e.Label, e.Err)
}

func (e pathStatError) Unwrap() error {
	return e.Err
}

type fileIsDirectoryError struct {
	Label string
	Path  string
}

func (e fileIsDirectoryError) Error() string {
	return fmt.Sprintf("%s is a directory: %s", e.Label, e.Path)
}

type fileEmptyError struct {
	Label string
	Path  string
}

func (e fileEmptyError) Error() string {
	return fmt.Sprintf("%s file is empty: %s", e.Label, e.Path)
}

type ensureDirError struct {
	Path string
	Err  error
}

func (e ensureDirError) Error() string {
	return fmt.Sprintf("ensure target parent dir: %v", e.Err)
}

func (e ensureDirError) Unwrap() error {
	return e.Err
}

type dirCreateTempError struct {
	Path string
	Err  error
}

func (e dirCreateTempError) Error() string {
	return fmt.Sprintf("target parent dir is not writable: %v", e.Err)
}

func (e dirCreateTempError) Unwrap() error {
	return e.Err
}

type dirWriteTestError struct {
	Path string
	Err  error
}

func (e dirWriteTestError) Error() string {
	return fmt.Sprintf("write target parent test file: %v", e.Err)
}

func (e dirWriteTestError) Unwrap() error {
	return e.Err
}

type dirCloseTestError struct {
	Path string
	Err  error
}

func (e dirCloseTestError) Error() string {
	return fmt.Sprintf("close target parent test file: %v", e.Err)
}

func (e dirCloseTestError) Unwrap() error {
	return e.Err
}

type freeSpaceCheckError struct {
	Path string
	Err  error
}

func (e freeSpaceCheckError) Error() string {
	return fmt.Sprintf("check free space: %v", e.Err)
}

func (e freeSpaceCheckError) Unwrap() error {
	return e.Err
}

type insufficientFreeSpaceError struct {
	Path           string
	NeededBytes    uint64
	AvailableBytes uint64
}

func (e insufficientFreeSpaceError) Error() string {
	return fmt.Sprintf("not enough free space in %s: need at least %d bytes, available %d bytes", e.Path, e.NeededBytes, e.AvailableBytes)
}

type stageCreateRootError struct {
	Path string
	Err  error
}

func (e stageCreateRootError) Error() string {
	return fmt.Sprintf("create sibling stage root: %v", e.Err)
}

func (e stageCreateRootError) Unwrap() error {
	return e.Err
}

type stagePrepareDirError struct {
	Path string
	Err  error
}

func (e stagePrepareDirError) Error() string {
	return fmt.Sprintf("create stage dir: %v", e.Err)
}

func (e stagePrepareDirError) Unwrap() error {
	return e.Err
}

type stageReadError struct {
	Path string
	Err  error
}

func (e stageReadError) Error() string {
	return fmt.Sprintf("read stage dir: %v", e.Err)
}

func (e stageReadError) Unwrap() error {
	return e.Err
}

type stageEmptyError struct {
	Path string
}

func (e stageEmptyError) Error() string {
	return "archive is empty"
}

type stageMixedRootError struct {
	Path       string
	TargetBase string
}

func (e stageMixedRootError) Error() string {
	return fmt.Sprintf("archive mixes target root directory %q with sibling entries", e.TargetBase)
}

type stageRootMismatchError struct {
	Path       string
	TargetBase string
}

func (e stageRootMismatchError) Error() string {
	return fmt.Sprintf("archive root must be exactly %q", e.TargetBase)
}

type preparedDirNotDirectoryError struct {
	Path string
}

func (e preparedDirNotDirectoryError) Error() string {
	return fmt.Sprintf("prepared dir is not a directory: %s", e.Path)
}

type treeScratchPathExistsError struct {
	Path string
}

func (e treeScratchPathExistsError) Error() string {
	return fmt.Sprintf("replace-tree scratch path already exists: %s", e.Path)
}

type treeStatError struct {
	Path string
	Err  error
}

func (e treeStatError) Error() string {
	return fmt.Sprintf("stat target dir: %v", e.Err)
}

func (e treeStatError) Unwrap() error {
	return e.Err
}

type treeRenameError struct {
	Action string
	From   string
	To     string
	Err    error
}

func (e treeRenameError) Error() string {
	return fmt.Sprintf("%s: %v", e.Action, e.Err)
}

func (e treeRenameError) Unwrap() error {
	return e.Err
}

type archiveReadError struct {
	Path string
	Err  error
}

func (e archiveReadError) Error() string {
	return fmt.Sprintf("read archive %s: %v", e.Path, e.Err)
}

func (e archiveReadError) Unwrap() error {
	return e.Err
}

type archiveEntryEscapeError struct {
	ArchivePath string
	EntryName   string
}

func (e archiveEntryEscapeError) Error() string {
	return fmt.Sprintf("archive entry escapes destination: %s", e.EntryName)
}

type archiveUnexpectedEntryTypeError struct {
	ArchivePath string
	EntryName   string
	Typeflag    byte
}

func (e archiveUnexpectedEntryTypeError) Error() string {
	return fmt.Sprintf("unexpected tar entry type after validation: %d", e.Typeflag)
}

type archiveEntryConflictError struct {
	ArchivePath  string
	EntryName    string
	ConflictPath string
	Reason       string
}

func (e archiveEntryConflictError) Error() string {
	if e.Reason != "" {
		return fmt.Sprintf("archive entry conflicts with existing path %s: %s", e.EntryName, e.Reason)
	}

	return fmt.Sprintf("archive entry conflicts with existing path: %s", e.EntryName)
}

type archiveEmptyError struct {
	ArchivePath string
}

func (e archiveEmptyError) Error() string {
	return "archive is empty"
}
