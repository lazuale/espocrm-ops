package backupstore

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	domainbackup "github.com/lazuale/espocrm-ops/internal/domain/backup"
)

type ManifestError struct {
	Err error
}

func (e ManifestError) Error() string {
	return e.Err.Error()
}

func (e ManifestError) Unwrap() error {
	return e.Err
}

func LoadManifest(path string) (domainbackup.Manifest, error) {
	var manifest domainbackup.Manifest

	raw, err := os.ReadFile(path)
	if err != nil {
		return manifest, ManifestError{Err: fmt.Errorf("read manifest: %w", err)}
	}

	if err := json.Unmarshal(raw, &manifest); err != nil {
		return manifest, ManifestError{Err: fmt.Errorf("parse manifest json: %w", err)}
	}

	if err := manifest.Validate(); err != nil {
		return manifest, ManifestError{Err: fmt.Errorf("validate manifest: %w", err)}
	}

	return manifest, nil
}

func WriteManifest(path string, manifest domainbackup.Manifest) error {
	if err := manifest.Validate(); err != nil {
		return err
	}

	raw, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal manifest: %w", err)
	}
	raw = append(raw, '\n')

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("ensure manifest dir: %w", err)
	}
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		return fmt.Errorf("write manifest: %w", err)
	}

	return nil
}

func WriteSHA256Sidecar(filePath, checksum, sidecarPath string) error {
	if err := domainbackup.ValidateChecksum("checksum", checksum); err != nil {
		return err
	}
	if strings.TrimSpace(sidecarPath) == "" {
		sidecarPath = filePath + ".sha256"
	}

	body := fmt.Sprintf("%s  %s\n", strings.ToLower(strings.TrimSpace(checksum)), filepath.Base(filePath))
	if err := os.MkdirAll(filepath.Dir(sidecarPath), 0o755); err != nil {
		return fmt.Errorf("ensure sidecar dir: %w", err)
	}
	if err := os.WriteFile(sidecarPath, []byte(body), 0o644); err != nil {
		return fmt.Errorf("write sha256 sidecar: %w", err)
	}

	return nil
}
