package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func ValidateBackup(dir string) error {
	if dir == "" {
		return fmt.Errorf("backup directory is required")
	}
	if err := requireDir(dir, "backup directory"); err != nil {
		return err
	}

	dbPath := filepath.Join(dir, dbFileName)
	filesPath := filepath.Join(dir, filesFileName)
	manifestPath := filepath.Join(dir, manifestFileName)
	sumsPath := filepath.Join(dir, sumsFileName)

	dbInfo, err := requireRegularFile(dbPath, dbFileName)
	if err != nil {
		return err
	}
	filesInfo, err := requireRegularFile(filesPath, filesFileName)
	if err != nil {
		return err
	}
	if _, err := requireRegularFile(manifestPath, manifestFileName); err != nil {
		return err
	}
	if _, err := requireRegularFile(sumsPath, sumsFileName); err != nil {
		return err
	}

	manifest, err := readManifest(manifestPath)
	if err != nil {
		return err
	}
	sumsData, err := os.ReadFile(sumsPath)
	if err != nil {
		return fmt.Errorf("read SHA256SUMS: %w", err)
	}
	sums, err := parseSHA256SUMS(sumsData)
	if err != nil {
		return err
	}
	if sums[dbFileName] != strings.ToLower(manifest.DBSHA256) {
		return fmt.Errorf("manifest.json and SHA256SUMS disagree for %s", dbFileName)
	}
	if sums[filesFileName] != strings.ToLower(manifest.FilesSHA256) {
		return fmt.Errorf("manifest.json and SHA256SUMS disagree for %s", filesFileName)
	}

	dbHash, dbSize, err := fileSHA256(dbPath)
	if err != nil {
		return err
	}
	filesHash, filesSize, err := fileSHA256(filesPath)
	if err != nil {
		return err
	}
	if dbHash != strings.ToLower(manifest.DBSHA256) {
		return fmt.Errorf("%s sha256 mismatch", dbFileName)
	}
	if filesHash != strings.ToLower(manifest.FilesSHA256) {
		return fmt.Errorf("%s sha256 mismatch", filesFileName)
	}
	if dbSize != manifest.DBSize || dbSize != dbInfo.Size() {
		return fmt.Errorf("%s size mismatch", dbFileName)
	}
	if filesSize != manifest.FilesSize || filesSize != filesInfo.Size() {
		return fmt.Errorf("%s size mismatch", filesFileName)
	}
	if err := validateGzipReadable(dbPath); err != nil {
		return err
	}
	if err := validateFilesArchive(filesPath); err != nil {
		return err
	}
	return nil
}
