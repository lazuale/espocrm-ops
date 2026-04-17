package fs

import (
	"os"
	"path/filepath"
)

func ReplaceTree(targetDir, preparedDir string) error {
	parent := filepath.Dir(targetDir)
	base := filepath.Base(targetDir)

	if fi, err := os.Stat(preparedDir); err != nil {
		return PathStatError{Label: "prepared dir", Path: preparedDir, Err: err}
	} else if !fi.IsDir() {
		return PreparedDirNotDirectoryError{Path: preparedDir}
	}

	if err := os.MkdirAll(parent, 0o755); err != nil {
		return EnsureDirError{Path: parent, Err: err}
	}

	newDir := filepath.Join(parent, "."+base+".new")
	oldDir := filepath.Join(parent, "."+base+".old")

	_ = os.RemoveAll(newDir)
	_ = os.RemoveAll(oldDir)

	if err := os.Rename(preparedDir, newDir); err != nil {
		return TreeRenameError{Action: "move prepared dir", From: preparedDir, To: newDir, Err: err}
	}

	if _, err := os.Stat(targetDir); err == nil {
		if err := os.Rename(targetDir, oldDir); err != nil {
			if renameErr := os.Rename(newDir, preparedDir); renameErr != nil {
				_ = os.RemoveAll(newDir)
			}
			return TreeRenameError{Action: "move target to old dir", From: targetDir, To: oldDir, Err: err}
		}
	} else if !os.IsNotExist(err) {
		if renameErr := os.Rename(newDir, preparedDir); renameErr != nil {
			_ = os.RemoveAll(newDir)
		}
		return TreeStatError{Path: targetDir, Err: err}
	}

	if err := os.Rename(newDir, targetDir); err != nil {
		if _, statErr := os.Stat(oldDir); statErr == nil {
			_ = os.Rename(oldDir, targetDir)
		}
		if renameErr := os.Rename(newDir, preparedDir); renameErr != nil {
			_ = os.RemoveAll(newDir)
		}
		return TreeRenameError{Action: "activate new dir", From: newDir, To: targetDir, Err: err}
	}

	_ = os.RemoveAll(oldDir)
	return nil
}
