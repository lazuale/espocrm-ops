package manifest

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const Version = 1

const (
	DBFileName    = "db.sql.gz"
	FilesFileName = "files.tar.gz"
	ManifestName  = "manifest.json"
)

type Manifest struct {
	Version   int      `json:"version"`
	Scope     string   `json:"scope"`
	CreatedAt string   `json:"created_at"`
	DB        Artifact `json:"db"`
	Files     Artifact `json:"files"`
	DBName    string   `json:"db_name"`
}

type Artifact struct {
	File   string `json:"file"`
	SHA256 string `json:"sha256"`
}

type ArtifactPaths struct {
	DBPath    string
	FilesPath string
}

func Load(path string) (Manifest, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return Manifest{}, fmt.Errorf("read manifest: %w", err)
	}

	var manifest Manifest
	if err := json.Unmarshal(raw, &manifest); err != nil {
		return Manifest{}, fmt.Errorf("decode manifest json: %w", err)
	}
	return manifest, nil
}

func Write(path string, manifest Manifest) error {
	raw, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(raw, '\n'), 0o644)
}

func Validate(path string, manifest Manifest) error {
	if strings.TrimSpace(path) == "" {
		return fmt.Errorf("manifest path is required")
	}
	if filepath.Base(path) != ManifestName {
		return fmt.Errorf("manifest file must be named %s", ManifestName)
	}
	if manifest.Version != Version {
		return fmt.Errorf("manifest version must be %d", Version)
	}
	if strings.TrimSpace(manifest.Scope) == "" {
		return fmt.Errorf("manifest scope is required")
	}
	if strings.TrimSpace(manifest.CreatedAt) == "" {
		return fmt.Errorf("manifest created_at is required")
	}
	if _, err := time.Parse(time.RFC3339Nano, manifest.CreatedAt); err != nil {
		return fmt.Errorf("manifest created_at must be RFC3339: %w", err)
	}
	if err := validateArtifact("db", manifest.DB, DBFileName); err != nil {
		return err
	}
	if err := validateArtifact("files", manifest.Files, FilesFileName); err != nil {
		return err
	}
	if strings.TrimSpace(manifest.DBName) == "" {
		return fmt.Errorf("manifest db_name is required")
	}
	return nil
}

func ResolveArtifacts(path string, manifest Manifest) ArtifactPaths {
	dir := filepath.Dir(path)
	return ArtifactPaths{
		DBPath:    filepath.Join(dir, filepath.Base(manifest.DB.File)),
		FilesPath: filepath.Join(dir, filepath.Base(manifest.Files.File)),
	}
}

func validateArtifact(label string, artifact Artifact, fileName string) error {
	if artifact.File != fileName {
		return fmt.Errorf("manifest %s.file must be %q", label, fileName)
	}
	if !validSHA256(artifact.SHA256) {
		return fmt.Errorf("manifest %s.sha256 must be a 64-char sha256 hex digest", label)
	}
	return nil
}

func validSHA256(value string) bool {
	value = strings.ToLower(strings.TrimSpace(value))
	if len(value) != 64 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}
