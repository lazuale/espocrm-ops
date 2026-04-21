package fs

import (
	"os"
	"syscall"
)

func EnsureNonEmptyFile(label, path string) (int64, error) {
	fi, err := os.Stat(path)
	if err != nil {
		return 0, pathStatError{Label: label, Path: path, Err: err}
	}
	if fi.IsDir() {
		return 0, fileIsDirectoryError{Label: label, Path: path}
	}
	if fi.Size() == 0 {
		return 0, fileEmptyError{Label: label, Path: path}
	}

	return fi.Size(), nil
}

func EnsureWritableDir(path string) error {
	if err := os.MkdirAll(path, 0o755); err != nil {
		return ensureDirError{Path: path, Err: err}
	}

	f, err := os.CreateTemp(path, ".espops-write-test-*")
	if err != nil {
		return dirCreateTempError{Path: path, Err: err}
	}
	name := f.Name()
	if _, err := f.Write([]byte("ok")); err != nil {
		_ = f.Close()
		_ = os.Remove(name)
		return dirWriteTestError{Path: path, Err: err}
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(name)
		return dirCloseTestError{Path: path, Err: err}
	}
	_ = os.Remove(name)

	return nil
}

func EnsureFreeSpace(path string, neededBytes uint64) error {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return freeSpaceCheckError{Path: path, Err: err}
	}

	available := uint64(stat.Bavail) * uint64(stat.Bsize)
	if neededBytes > available {
		return insufficientFreeSpaceError{Path: path, NeededBytes: neededBytes, AvailableBytes: available}
	}

	return nil
}
