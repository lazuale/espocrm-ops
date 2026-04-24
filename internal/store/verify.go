package store

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/lazuale/espocrm-ops/internal/model"
)

func (FileStore) LoadBackupVerifyManifest(ctx context.Context, path string) (model.BackupVerifyManifest, error) {
	if err := ctx.Err(); err != nil {
		return model.BackupVerifyManifest{}, err
	}
	if strings.TrimSpace(path) == "" {
		return model.BackupVerifyManifest{}, model.BackupVerifyManifestError{Err: fmt.Errorf("manifest path обязателен")}
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		return model.BackupVerifyManifest{}, model.BackupVerifyManifestError{Err: fmt.Errorf("прочитать manifest: %w", err)}
	}

	var manifest model.BackupVerifyManifest
	if err := json.Unmarshal(raw, &manifest); err != nil {
		return model.BackupVerifyManifest{}, model.BackupVerifyManifestError{Err: fmt.Errorf("разобрать manifest json: %w", err)}
	}
	manifest = manifest.Normalized()
	if err := manifest.ValidateComplete(); err != nil {
		return model.BackupVerifyManifest{}, model.BackupVerifyManifestError{Err: fmt.Errorf("проверить complete manifest contract: %w", err)}
	}
	if err := model.ValidateBackupVerifyManifestCoherence(path, manifest); err != nil {
		return model.BackupVerifyManifest{}, model.BackupVerifyManifestError{Err: err}
	}

	return manifest, nil
}

func (FileStore) VerifyDBArtifact(ctx context.Context, path, expectedChecksum string) (model.Artifact, error) {
	return verifyArtifact(ctx, artifactVerifySpec{
		label:            "db backup",
		path:             path,
		expectedChecksum: expectedChecksum,
		requiredSuffix:   ".sql.gz",
		verifyReadable:   verifyGzipReadable,
	})
}

func (FileStore) VerifyFilesArtifact(ctx context.Context, path, expectedChecksum string) (model.Artifact, error) {
	return verifyArtifact(ctx, artifactVerifySpec{
		label:            "files backup",
		path:             path,
		expectedChecksum: expectedChecksum,
		requiredSuffix:   ".tar.gz",
		verifyReadable:   verifyTarGzReadable,
	})
}

type artifactVerifySpec struct {
	label            string
	path             string
	expectedChecksum string
	requiredSuffix   string
	verifyReadable   func(string) error
}

func verifyArtifact(ctx context.Context, spec artifactVerifySpec) (model.Artifact, error) {
	if err := ctx.Err(); err != nil {
		return model.Artifact{}, err
	}
	path := strings.TrimSpace(spec.path)
	if path == "" {
		return model.Artifact{}, model.BackupVerifyArtifactError{Label: spec.label, Err: fmt.Errorf("path обязателен")}
	}
	if !strings.HasSuffix(path, spec.requiredSuffix) {
		return model.Artifact{}, model.BackupVerifyArtifactError{Label: spec.label, Err: fmt.Errorf("ожидался suffix %s", spec.requiredSuffix)}
	}

	size, err := nonEmptyFile(path)
	if err != nil {
		return model.Artifact{}, model.BackupVerifyArtifactError{Label: spec.label, Err: err}
	}

	actual, err := sha256File(path)
	if err != nil {
		return model.Artifact{}, model.BackupVerifyArtifactError{Label: spec.label, Err: err}
	}
	if expected := strings.TrimSpace(spec.expectedChecksum); expected != "" {
		if err := model.ValidateChecksum("checksum", expected); err != nil {
			return model.Artifact{}, model.BackupVerifyArtifactError{Label: spec.label, Err: err}
		}
		if !strings.EqualFold(actual, expected) {
			return model.Artifact{}, model.BackupVerifyArtifactError{Label: spec.label, Err: fmt.Errorf("sha256 не совпадает с manifest")}
		}
	}

	sidecarChecksum, err := readSHA256Sidecar(path+".sha256", path)
	if err != nil {
		return model.Artifact{}, model.BackupVerifyArtifactError{Label: spec.label, Err: err}
	}
	if !strings.EqualFold(actual, sidecarChecksum) {
		return model.Artifact{}, model.BackupVerifyArtifactError{Label: spec.label, Err: fmt.Errorf("sha256 не совпадает с sidecar")}
	}
	if expected := strings.TrimSpace(spec.expectedChecksum); expected != "" && !strings.EqualFold(sidecarChecksum, expected) {
		return model.Artifact{}, model.BackupVerifyArtifactError{Label: spec.label, Err: fmt.Errorf("sidecar sha256 не совпадает с manifest")}
	}

	if err := spec.verifyReadable(path); err != nil {
		return model.Artifact{}, model.BackupVerifyArtifactError{Label: spec.label, Err: err}
	}

	return model.NewArtifactFromVerification(path, actual, size), nil
}

func nonEmptyFile(path string) (int64, error) {
	info, err := os.Stat(path)
	if err != nil {
		return 0, fmt.Errorf("проверить файл: %w", err)
	}
	if info.IsDir() {
		return 0, fmt.Errorf("ожидался файл, получен каталог")
	}
	if info.Size() == 0 {
		return 0, fmt.Errorf("файл пустой")
	}
	return info.Size(), nil
}

func sha256File(path string) (checksum string, err error) {
	file, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("открыть файл для sha256: %w", err)
	}
	defer closeVerifyResource(file, &err)

	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", fmt.Errorf("посчитать sha256: %w", err)
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func readSHA256Sidecar(sidecarPath, artifactPath string) (string, error) {
	if _, err := nonEmptyFile(sidecarPath); err != nil {
		return "", fmt.Errorf("checksum sidecar недоступен: %w", err)
	}
	raw, err := os.ReadFile(sidecarPath)
	if err != nil {
		return "", fmt.Errorf("прочитать checksum sidecar: %w", err)
	}
	fields := strings.Fields(string(raw))
	if len(fields) == 0 {
		return "", fmt.Errorf("checksum sidecar пустой")
	}
	if err := model.ValidateChecksum("checksum", fields[0]); err != nil {
		return "", err
	}
	if len(fields) > 1 && filepath.Base(fields[1]) != filepath.Base(artifactPath) {
		return "", fmt.Errorf("checksum sidecar указывает на другой artifact")
	}
	return strings.ToLower(fields[0]), nil
}

func verifyGzipReadable(path string) (err error) {
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("открыть gzip: %w", err)
	}
	defer closeVerifyResource(file, &err)

	reader, err := gzip.NewReader(file)
	if err != nil {
		return fmt.Errorf("создать gzip reader: %w", err)
	}
	defer closeVerifyResource(reader, &err)

	if _, err := io.Copy(io.Discard, reader); err != nil {
		return fmt.Errorf("прочитать gzip: %w", err)
	}
	return nil
}

func verifyTarGzReadable(path string) (err error) {
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("открыть tar.gz: %w", err)
	}
	defer closeVerifyResource(file, &err)

	gzipReader, err := gzip.NewReader(file)
	if err != nil {
		return fmt.Errorf("создать gzip reader: %w", err)
	}
	defer closeVerifyResource(gzipReader, &err)

	tarReader := tar.NewReader(gzipReader)
	var found bool
	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("прочитать tar entry: %w", err)
		}
		if err := model.ValidateFilesArchiveHeader(header); err != nil {
			return err
		}
		cleanName := filepath.ToSlash(filepath.Clean(header.Name))
		if cleanName != "." && cleanName != "/" {
			found = true
		}
		if _, err := io.Copy(io.Discard, tarReader); err != nil {
			return fmt.Errorf("прочитать tar payload: %w", err)
		}
	}
	if !found {
		return fmt.Errorf("files archive пустой")
	}
	return nil
}

func closeVerifyResource(closer io.Closer, errp *error) {
	if closer == nil {
		return
	}
	if closeErr := closer.Close(); closeErr != nil && *errp == nil {
		*errp = closeErr
	}
}
