package main

import (
	"encoding/json"
	"fmt"
	"os"
	"time"
)

const (
	dbFileName       = "db.sql.gz"
	filesFileName    = "files.tar.gz"
	manifestFileName = "manifest.json"
	sumsFileName     = "SHA256SUMS"
)

type Manifest struct {
	Version      int    `json:"version"`
	CreatedAtUTC string `json:"created_at_utc"`
	DBFile       string `json:"db_file"`
	FilesFile    string `json:"files_file"`
	DBSHA256     string `json:"db_sha256"`
	FilesSHA256  string `json:"files_sha256"`
	DBSize       int64  `json:"db_size"`
	FilesSize    int64  `json:"files_size"`
}

func newManifest(created time.Time, db checksumFile, files checksumFile) Manifest {
	return Manifest{
		Version:      1,
		CreatedAtUTC: created.UTC().Format(time.RFC3339),
		DBFile:       dbFileName,
		FilesFile:    filesFileName,
		DBSHA256:     db.Hash,
		FilesSHA256:  files.Hash,
		DBSize:       db.Size,
		FilesSize:    files.Size,
	}
}

func readManifest(path string) (Manifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Manifest{}, fmt.Errorf("read manifest.json: %w", err)
	}
	var manifest Manifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return Manifest{}, fmt.Errorf("parse manifest.json: %w", err)
	}
	if err := validateManifestShape(manifest); err != nil {
		return Manifest{}, err
	}
	return manifest, nil
}

func writeManifest(path string, manifest Manifest) error {
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return fmt.Errorf("encode manifest.json: %w", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("write manifest.json: %w", err)
	}
	return nil
}

func validateManifestShape(manifest Manifest) error {
	if manifest.Version != 1 {
		return fmt.Errorf("manifest.json version must be 1")
	}
	if _, err := time.Parse(time.RFC3339, manifest.CreatedAtUTC); err != nil {
		return fmt.Errorf("manifest.json created_at_utc must be RFC3339: %w", err)
	}
	if manifest.DBFile != dbFileName {
		return fmt.Errorf("manifest.json db_file must be %s", dbFileName)
	}
	if manifest.FilesFile != filesFileName {
		return fmt.Errorf("manifest.json files_file must be %s", filesFileName)
	}
	if manifest.DBSHA256 == "" || manifest.FilesSHA256 == "" {
		return fmt.Errorf("manifest.json hashes must not be empty")
	}
	if manifest.DBSize < 0 || manifest.FilesSize < 0 {
		return fmt.Errorf("manifest.json sizes must not be negative")
	}
	return nil
}
