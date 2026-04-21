package config

import (
	"fmt"
	"io/fs"
	"os"
	"syscall"
)

func validateEnvFileForLoading(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return MissingEnvFileError{Path: path}
		}
		return fmt.Errorf("stat env file %s: %w", path, err)
	}

	if err := validateEnvFileMetadata(path, info, os.Getuid()); err != nil {
		return err
	}

	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open env file %s: %w", path, err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close env file %s: %w", path, err)
	}

	return nil
}

func validateEnvFileMetadata(path string, info fs.FileInfo, currentUID int) error {
	if err := validateEnvFileType(path, info.Mode()); err != nil {
		return err
	}

	ownerUID, err := envFileOwnerUID(info)
	if err != nil {
		return fmt.Errorf("determine env file owner %s: %w", path, err)
	}
	if err := validateEnvFileOwnership(path, ownerUID, currentUID); err != nil {
		return err
	}
	if err := validateEnvFilePermissions(path, info.Mode().Perm()); err != nil {
		return err
	}

	return nil
}

func validateEnvFileType(path string, mode fs.FileMode) error {
	if mode&os.ModeSymlink != 0 {
		return InvalidEnvFileError{Path: path, Message: "env file must not be a symlink"}
	}
	if !mode.IsRegular() {
		return InvalidEnvFileError{Path: path, Message: "env file must be a regular file"}
	}

	return nil
}

func envFileOwnerUID(info fs.FileInfo) (uint32, error) {
	statT, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, fmt.Errorf("unsupported stat payload")
	}

	return statT.Uid, nil
}

func validateEnvFileOwnership(path string, ownerUID uint32, currentUID int) error {
	if ownerUID != uint32(currentUID) && ownerUID != 0 {
		return InvalidEnvFileError{Path: path, Message: "env file must belong to the current user or root"}
	}

	return nil
}

func validateEnvFilePermissions(path string, perm fs.FileMode) error {
	if perm&0o137 != 0 {
		return InvalidEnvFileError{Path: path, Message: fmt.Sprintf("env file must not be broader than 640 and must not have execute bits: current %03o", perm)}
	}

	return nil
}
