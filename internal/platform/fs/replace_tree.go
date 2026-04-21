package fs

import (
	"errors"
	"os"
	"path/filepath"
)

func ReplaceTree(targetDir, preparedDir string) error {
	parent := filepath.Dir(targetDir)
	base := filepath.Base(targetDir)

	if fi, err := os.Stat(preparedDir); err != nil {
		return pathStatError{Label: "prepared dir", Path: preparedDir, Err: err}
	} else if !fi.IsDir() {
		return preparedDirNotDirectoryError{Path: preparedDir}
	}

	if err := os.MkdirAll(parent, 0o755); err != nil {
		return ensureDirError{Path: parent, Err: err}
	}

	newDir := filepath.Join(parent, "."+base+".new")
	oldDir := filepath.Join(parent, "."+base+".old")

	if err := ensureScratchPathAbsent(newDir); err != nil {
		return err
	}
	if err := ensureScratchPathAbsent(oldDir); err != nil {
		return err
	}

	if err := os.Rename(preparedDir, newDir); err != nil {
		return treeRenameError{Action: "move prepared dir", From: preparedDir, To: newDir, Err: err}
	}

	if _, err := os.Stat(targetDir); err == nil {
		if err := os.Rename(targetDir, oldDir); err != nil {
			rollbackErr := restorePreparedDir(newDir, preparedDir)
			if rollbackErr != nil {
				err = errors.Join(err, rollbackErr)
			}
			return treeRenameError{Action: "move target to old dir", From: targetDir, To: oldDir, Err: err}
		}
	} else if !os.IsNotExist(err) {
		rollbackErr := restorePreparedDir(newDir, preparedDir)
		if rollbackErr != nil {
			err = errors.Join(err, rollbackErr)
		}
		return treeStatError{Path: targetDir, Err: err}
	}

	if err := os.Rename(newDir, targetDir); err != nil {
		restoreErr := restoreTargetDir(oldDir, targetDir)
		rollbackErr := restorePreparedDir(newDir, preparedDir)
		if restoreErr != nil {
			err = errors.Join(err, restoreErr)
		}
		if rollbackErr != nil {
			err = errors.Join(err, rollbackErr)
		}
		return treeRenameError{Action: "activate new dir", From: newDir, To: targetDir, Err: err}
	}

	_ = os.RemoveAll(oldDir)
	return nil
}

func ensureScratchPathAbsent(path string) error {
	_, err := os.Lstat(path)
	switch {
	case err == nil:
		return treeScratchPathExistsError{Path: path}
	case os.IsNotExist(err):
		return nil
	default:
		return pathStatError{Label: "replace-tree scratch path", Path: path, Err: err}
	}
}

func restorePreparedDir(from, to string) error {
	if err := os.Rename(from, to); err != nil && !os.IsNotExist(err) {
		return treeRenameError{Action: "restore prepared dir", From: from, To: to, Err: err}
	}

	return nil
}

func restoreTargetDir(from, to string) error {
	if _, err := os.Stat(from); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return treeStatError{Path: from, Err: err}
	}
	if err := os.Rename(from, to); err != nil {
		return treeRenameError{Action: "restore target dir", From: from, To: to, Err: err}
	}

	return nil
}
