package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const timestampFormat = "20060102-150405"

var nowUTC = func() time.Time {
	return time.Now().UTC()
}

func requireDir(path string, label string) error {
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("%s does not exist: %s", label, path)
		}
		return fmt.Errorf("stat %s %s: %w", label, path, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("%s is not a directory: %s", label, path)
	}
	return nil
}

func requireRegularFile(path string, label string) (os.FileInfo, error) {
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("missing %s: %s", label, path)
		}
		return nil, fmt.Errorf("stat %s %s: %w", label, path, err)
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("%s is not a regular file: %s", label, path)
	}
	return info, nil
}

func pathInside(base string, child string) (bool, error) {
	baseAbs, err := filepath.Abs(filepath.Clean(base))
	if err != nil {
		return false, err
	}
	childAbs, err := filepath.Abs(filepath.Clean(child))
	if err != nil {
		return false, err
	}
	rel, err := filepath.Rel(baseAbs, childAbs)
	if err != nil {
		return false, err
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))), nil
}

func safeRemoveTempDir(path string, allowedParent string) error {
	inside, err := pathInside(allowedParent, path)
	if err != nil {
		return fmt.Errorf("validate temp path: %w", err)
	}
	if !inside || filepath.Clean(path) == filepath.Clean(allowedParent) {
		return fmt.Errorf("refusing to remove unsafe temp path %s", path)
	}
	if err := os.RemoveAll(path); err != nil {
		return fmt.Errorf("remove temp dir %s: %w", path, err)
	}
	return nil
}
