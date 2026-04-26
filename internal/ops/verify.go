package ops

import (
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/lazuale/espocrm-ops/internal/manifest"
	"github.com/lazuale/espocrm-ops/internal/runtime"
)

const (
	ErrorKindUsage    = "usage"
	ErrorKindManifest = "manifest"
	ErrorKindArtifact = "artifact"
	ErrorKindChecksum = "checksum"
	ErrorKindArchive  = "archive"
	ErrorKindRuntime  = "runtime"
	ErrorKindIO       = "io"
)

type VerifyResult struct {
	Manifest    string
	Scope       string
	CreatedAt   string
	DBBackup    string
	FilesBackup string
	DBName      string
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
	manifestPath = strings.TrimSpace(manifestPath)
	if manifestPath == "" {
		return VerifyResult{}, usageError("manifest path is required")
	}
	if err := ctx.Err(); err != nil {
		return VerifyResult{}, ioError("backup verify interrupted", err)
	}
	if err := ensureNonEmptyFile(manifestPath); err != nil {
		return VerifyResult{}, manifestError("manifest is invalid", err)
	}

	loadedManifest, err := manifest.Load(manifestPath)
	if err != nil {
		return VerifyResult{}, manifestError("manifest is invalid", err)
	}
	if err := manifest.Validate(manifestPath, loadedManifest); err != nil {
		return VerifyResult{}, manifestError("manifest is invalid", err)
	}

	paths := manifest.ResolveArtifacts(manifestPath, loadedManifest)
	if err := verifyArtifact(ctx, "db backup", paths.DBPath, loadedManifest.DB.SHA256, testGzip); err != nil {
		return VerifyResult{}, err
	}
	if err := verifyArtifact(ctx, "files backup", paths.FilesPath, loadedManifest.Files.SHA256, runtime.TestTarGz); err != nil {
		return VerifyResult{}, err
	}

	return VerifyResult{
		Manifest:    manifestPath,
		Scope:       loadedManifest.Scope,
		CreatedAt:   loadedManifest.CreatedAt,
		DBBackup:    paths.DBPath,
		FilesBackup: paths.FilesPath,
		DBName:      loadedManifest.DBName,
	}, nil
}

func testGzip(ctx context.Context, path string) (err error) {
	if err := ctx.Err(); err != nil {
		return err
	}

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

	if _, err := io.Copy(io.Discard, reader); err != nil {
		return err
	}
	return ctx.Err()
}

func verifyArtifact(ctx context.Context, label, path, manifestChecksum string, verifyReadable func(context.Context, string) error) error {
	if err := ctx.Err(); err != nil {
		return ioError("backup verify interrupted", err)
	}
	if err := ensureNonEmptyFile(path); err != nil {
		return artifactError(label+" is unavailable", err)
	}

	actualChecksum, err := sha256File(path)
	if err != nil {
		return ioError("failed to read "+label+" checksum", err)
	}
	if actualChecksum != strings.ToLower(strings.TrimSpace(manifestChecksum)) {
		return checksumError(label+" checksum does not match manifest", nil)
	}
	if err := verifyReadable(ctx, path); err != nil {
		return archiveError(label+" archive is unreadable", err)
	}
	return nil
}

func ensureNonEmptyFile(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if info.IsDir() {
		return fmt.Errorf("expected file, got directory")
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("expected regular file")
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

func usageError(message string) error {
	return &VerifyError{Kind: ErrorKindUsage, Message: message}
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

func runtimeError(message string, err error) error {
	return &VerifyError{Kind: ErrorKindRuntime, Message: message, Err: err}
}

func ioError(message string, err error) error {
	return &VerifyError{Kind: ErrorKindIO, Message: message, Err: err}
}

func closeResource(closer io.Closer, errp *error) {
	if closer == nil {
		return
	}
	if closeErr := closer.Close(); closeErr != nil && *errp == nil {
		*errp = closeErr
	}
}
