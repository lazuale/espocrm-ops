package fs

import (
	"os"
	"syscall"
)

func EnsureNonEmptyFile(label, path string) (int64, error) {
	fi, err := os.Stat(path)
	if err != nil {
		return 0, PathStatError{Label: label, Path: path, Err: err}
	}
	if fi.IsDir() {
		return 0, FileIsDirectoryError{Label: label, Path: path}
	}
	if fi.Size() == 0 {
		return 0, FileEmptyError{Label: label, Path: path}
	}

	return fi.Size(), nil
}

func EnsureWritableDir(path string) error {
	if err := os.MkdirAll(path, 0o755); err != nil {
		return EnsureDirError{Path: path, Err: err}
	}

	f, err := os.CreateTemp(path, ".espops-write-test-*")
	if err != nil {
		return DirCreateTempError{Path: path, Err: err}
	}
	name := f.Name()
	if _, err := f.Write([]byte("ok")); err != nil {
		_ = f.Close()
		_ = os.Remove(name)
		return DirWriteTestError{Path: path, Err: err}
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(name)
		return DirCloseTestError{Path: path, Err: err}
	}
	_ = os.Remove(name)

	return nil
}

func EnsureFreeSpace(path string, neededBytes uint64) error {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return FreeSpaceCheckError{Path: path, Err: err}
	}

	available := uint64(stat.Bavail) * uint64(stat.Bsize)
	if neededBytes > available {
		return InsufficientFreeSpaceError{Path: path, NeededBytes: neededBytes, AvailableBytes: available}
	}

	return nil
}
