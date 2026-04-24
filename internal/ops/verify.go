package ops

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/lazuale/espocrm-ops/internal/manifest"
)

const (
	ErrorKindUsage    = "usage"
	ErrorKindManifest = "manifest"
	ErrorKindArtifact = "artifact"
	ErrorKindChecksum = "checksum"
	ErrorKindArchive  = "archive"
	ErrorKindIO       = "io"
)

const legacyTarRegularTypeflag = byte(0)

type VerifyResult struct {
	Manifest    string
	Scope       string
	CreatedAt   string
	DBBackup    string
	FilesBackup string
}

type VerifyError struct {
	Kind    string
	Message string
	Err     error
}

func (e *VerifyError) Error() string {
	if e == nil {
		return ""
	}
	if e.Err == nil {
		return e.Message
	}
	if strings.TrimSpace(e.Message) == "" {
		return e.Err.Error()
	}
	return e.Message + ": " + e.Err.Error()
}

func (e *VerifyError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

func VerifyBackup(ctx context.Context, manifestPath string) (VerifyResult, error) {
	if strings.TrimSpace(manifestPath) == "" {
		return VerifyResult{}, &VerifyError{Kind: ErrorKindUsage, Message: "manifest path is required"}
	}
	if err := ctx.Err(); err != nil {
		return VerifyResult{}, ioError("backup verify interrupted", err)
	}

	loadedManifest, err := manifest.Load(manifestPath)
	if err != nil {
		return VerifyResult{}, manifestError("manifest is invalid", err)
	}
	if err := manifest.Validate(manifestPath, loadedManifest); err != nil {
		return VerifyResult{}, manifestError("manifest is invalid", err)
	}

	paths := manifest.ResolveArtifacts(manifestPath, loadedManifest)
	if err := verifyArtifact(ctx, "db backup", paths.DBPath, paths.DBSidecarPath, loadedManifest.Checksums.DBBackup, ".sql.gz", verifyGzipReadable); err != nil {
		return VerifyResult{}, err
	}
	if err := verifyArtifact(ctx, "files backup", paths.FilesPath, paths.FilesSidecarPath, loadedManifest.Checksums.FilesBackup, ".tar.gz", verifyTarGzReadable); err != nil {
		return VerifyResult{}, err
	}

	return VerifyResult{
		Manifest:    manifestPath,
		Scope:       loadedManifest.Scope,
		CreatedAt:   loadedManifest.CreatedAt,
		DBBackup:    paths.DBPath,
		FilesBackup: paths.FilesPath,
	}, nil
}

func verifyArtifact(ctx context.Context, label, path, sidecarPath, expectedChecksum, requiredSuffix string, verifyReadable func(string) error) error {
	if err := ctx.Err(); err != nil {
		return ioError("backup verify interrupted", err)
	}
	if !strings.HasSuffix(path, requiredSuffix) {
		return artifactError(label+" has an unexpected file name", nil)
	}
	if err := ensureNonEmptyFile(path); err != nil {
		return artifactError(label+" is unavailable", err)
	}

	actualChecksum, err := sha256File(path)
	if err != nil {
		return ioError("failed to read "+label+" checksum", err)
	}

	expectedChecksum = strings.ToLower(strings.TrimSpace(expectedChecksum))
	if !validChecksum(expectedChecksum) {
		return checksumError(label+" manifest checksum is invalid", nil)
	}
	if actualChecksum != expectedChecksum {
		return checksumError(label+" checksum does not match manifest", nil)
	}

	sidecarChecksum, err := readSidecarChecksum(sidecarPath, path)
	if err != nil {
		return checksumError(label+" checksum sidecar is invalid", err)
	}
	if actualChecksum != sidecarChecksum {
		return checksumError(label+" checksum does not match sidecar", nil)
	}

	if err := verifyReadable(path); err != nil {
		return archiveError(label+" archive is unreadable", err)
	}

	return nil
}

func ensureNonEmptyFile(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if info.IsDir() {
		return fmt.Errorf("expected file, got directory")
	}
	if info.Size() == 0 {
		return fmt.Errorf("file is empty")
	}

	return nil
}

func sha256File(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer closeResource(file, &err)

	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}

	return hex.EncodeToString(hash.Sum(nil)), nil
}

func readSidecarChecksum(sidecarPath, artifactPath string) (string, error) {
	if err := ensureNonEmptyFile(sidecarPath); err != nil {
		return "", err
	}
	raw, err := os.ReadFile(sidecarPath)
	if err != nil {
		return "", err
	}

	fields := strings.Fields(string(raw))
	if len(fields) == 0 {
		return "", fmt.Errorf("sidecar is empty")
	}
	digest := strings.ToLower(strings.TrimSpace(fields[0]))
	if !validChecksum(digest) {
		return "", fmt.Errorf("sidecar checksum is invalid")
	}
	if len(fields) > 1 && filepath.Base(fields[1]) != filepath.Base(artifactPath) {
		return "", fmt.Errorf("sidecar points to a different artifact")
	}

	return digest, nil
}

func verifyGzipReadable(path string) (err error) {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer closeResource(file, &err)

	reader, err := gzip.NewReader(file)
	if err != nil {
		return err
	}
	defer closeResource(reader, &err)

	_, err = io.Copy(io.Discard, reader)
	return err
}

func verifyTarGzReadable(path string) (err error) {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer closeResource(file, &err)

	gzipReader, err := gzip.NewReader(file)
	if err != nil {
		return err
	}
	defer closeResource(gzipReader, &err)

	tarReader := tar.NewReader(gzipReader)
	var found bool
	for {
		header, nextErr := tarReader.Next()
		if nextErr == io.EOF {
			break
		}
		if nextErr != nil {
			return nextErr
		}
		if err := validateTarHeader(header); err != nil {
			return err
		}
		found = true
		if _, err := io.Copy(io.Discard, tarReader); err != nil {
			return err
		}
	}
	if !found {
		return fmt.Errorf("files archive is empty")
	}

	return nil
}

func validateTarHeader(header *tar.Header) error {
	if header == nil {
		return fmt.Errorf("tar header is required")
	}
	name := strings.TrimSpace(header.Name)
	if name == "" {
		return fmt.Errorf("tar entry has empty name")
	}
	clean := filepath.ToSlash(filepath.Clean(name))
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") || filepath.IsAbs(name) {
		return fmt.Errorf("tar entry escapes archive root: %s", name)
	}
	switch header.Typeflag {
	case tar.TypeDir, tar.TypeReg, legacyTarRegularTypeflag:
		return nil
	default:
		return fmt.Errorf("tar entry type is not allowed: %d", header.Typeflag)
	}
}

func validChecksum(value string) bool {
	if len(value) != 64 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func closeResource(closer io.Closer, errp *error) {
	if closer == nil {
		return
	}
	if closeErr := closer.Close(); closeErr != nil && *errp == nil {
		*errp = closeErr
	}
}

func manifestError(message string, err error) error {
	return &VerifyError{Kind: ErrorKindManifest, Message: message, Err: err}
}

func artifactError(message string, err error) error {
	return &VerifyError{Kind: ErrorKindArtifact, Message: message, Err: err}
}

func checksumError(message string, err error) error {
	return &VerifyError{Kind: ErrorKindChecksum, Message: message, Err: err}
}

func archiveError(message string, err error) error {
	return &VerifyError{Kind: ErrorKindArchive, Message: message, Err: err}
}

func ioError(message string, err error) error {
	return &VerifyError{Kind: ErrorKindIO, Message: message, Err: err}
}
