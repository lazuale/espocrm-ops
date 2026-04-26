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
	"path"
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
	ErrorKindRuntime  = "runtime"
	ErrorKindIO       = "io"
)

const tarRegularTypeflagZero = byte(0)

const maxInt64 = int64(1<<63 - 1)

type VerifyResult struct {
	Manifest           string
	ManifestVersion    int
	Scope              string
	CreatedAt          string
	DBBackup           string
	FilesBackup        string
	FilesExpandedBytes int64
	Runtime            manifest.Runtime
	Warnings           []string
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

	paths, err := manifest.ResolveArtifacts(manifestPath, loadedManifest)
	if err != nil {
		return VerifyResult{}, manifestError("manifest is invalid", err)
	}

	_, dbWarnings, err := verifyArtifact(ctx, "db backup", paths.DBPath, paths.DBSidecarPath, loadedManifest.Checksums.DBBackup, ".sql.gz", verifyGzipReadableForArtifact)
	if err != nil {
		return VerifyResult{}, err
	}
	filesCheck, filesWarnings, err := verifyArtifact(ctx, "files backup", paths.FilesPath, paths.FilesSidecarPath, loadedManifest.Checksums.FilesBackup, ".tar.gz", verifyTarGzReadableForArtifact)
	if err != nil {
		return VerifyResult{}, err
	}

	return VerifyResult{
		Manifest:           manifestPath,
		ManifestVersion:    loadedManifest.Version,
		Scope:              loadedManifest.Scope,
		CreatedAt:          loadedManifest.CreatedAt,
		DBBackup:           paths.DBPath,
		FilesBackup:        paths.FilesPath,
		FilesExpandedBytes: filesCheck.FilesExpandedBytes,
		Runtime:            loadedManifest.Runtime,
		Warnings:           append(dbWarnings, filesWarnings...),
	}, nil
}

type artifactCheckResult struct {
	FilesExpandedBytes int64
}

func verifyArtifact(ctx context.Context, label, artifactPath, sidecarPath, manifestChecksum, requiredSuffix string, verifyReadable func(string) (artifactCheckResult, error)) (artifactCheckResult, []string, error) {
	if err := ctx.Err(); err != nil {
		return artifactCheckResult{}, nil, ioError("backup verify interrupted", err)
	}
	if !strings.HasSuffix(artifactPath, requiredSuffix) {
		return artifactCheckResult{}, nil, artifactError(label+" has an unexpected file name", nil)
	}
	if err := ensureNonEmptyFile(artifactPath); err != nil {
		return artifactCheckResult{}, nil, artifactError(label+" is unavailable", err)
	}

	actualChecksum, err := sha256File(artifactPath)
	if err != nil {
		return artifactCheckResult{}, nil, ioError("failed to read "+label+" checksum", err)
	}

	manifestChecksum = strings.ToLower(strings.TrimSpace(manifestChecksum))
	if !validChecksum(manifestChecksum) {
		return artifactCheckResult{}, nil, checksumError(label+" manifest checksum is invalid", nil)
	}
	if actualChecksum != manifestChecksum {
		return artifactCheckResult{}, nil, checksumError(label+" checksum does not match manifest", nil)
	}

	warnings := verifySidecarChecksum(label, sidecarPath, artifactPath, actualChecksum)
	result, err := verifyReadable(artifactPath)
	if err != nil {
		return artifactCheckResult{}, warnings, archiveError(label+" archive is unreadable", err)
	}

	return result, warnings, nil
}

func verifySidecarChecksum(label, sidecarPath, artifactPath, actualChecksum string) []string {
	if err := ensureNonEmptyFile(sidecarPath); err != nil {
		return []string{label + " checksum sidecar is unavailable: " + err.Error()}
	}
	sidecarChecksum, err := readSidecarChecksum(sidecarPath, artifactPath)
	if err != nil {
		return []string{label + " checksum sidecar is invalid: " + err.Error()}
	}
	if actualChecksum != sidecarChecksum {
		return []string{label + " checksum does not match sidecar"}
	}
	return nil
}

func ensureNonEmptyFile(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("expected regular file, got symlink")
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

func readSidecarChecksum(sidecarPath, artifactPath string) (string, error) {
	raw, err := os.ReadFile(sidecarPath)
	if err != nil {
		return "", err
	}
	fields := strings.Fields(string(raw))
	if len(fields) < 2 {
		return "", fmt.Errorf("sidecar must contain digest and filename")
	}

	digest := strings.ToLower(strings.TrimSpace(fields[0]))
	if !validChecksum(digest) {
		return "", fmt.Errorf("sidecar checksum is invalid")
	}

	name := strings.TrimPrefix(fields[1], "*")
	if filepath.Base(name) != filepath.Base(artifactPath) {
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

func verifyGzipReadableForArtifact(path string) (artifactCheckResult, error) {
	return artifactCheckResult{}, verifyGzipReadable(path)
}

func verifyTarGzReadableForArtifact(path string) (artifactCheckResult, error) {
	info, err := inspectFilesArchive(path)
	if err != nil {
		return artifactCheckResult{}, err
	}
	return artifactCheckResult{FilesExpandedBytes: info.expandedBytes}, nil
}

type filesArchiveInfo struct {
	expandedBytes int64
}

func inspectFilesArchive(path string) (info filesArchiveInfo, err error) {
	file, err := os.Open(path)
	if err != nil {
		return filesArchiveInfo{}, err
	}
	defer closeResource(file, &err)

	gzipReader, err := gzip.NewReader(file)
	if err != nil {
		return filesArchiveInfo{}, err
	}
	defer closeResource(gzipReader, &err)

	tarReader := tar.NewReader(gzipReader)
	validator := newFilesArchiveValidator()
	var found bool
	for {
		header, nextErr := tarReader.Next()
		if nextErr == io.EOF {
			break
		}
		if nextErr != nil {
			return filesArchiveInfo{}, nextErr
		}
		if err := validator.validate(header); err != nil {
			return filesArchiveInfo{}, err
		}
		found = true
		if _, err := io.Copy(io.Discard, tarReader); err != nil {
			return filesArchiveInfo{}, err
		}
	}
	if !found {
		return filesArchiveInfo{}, fmt.Errorf("files archive is empty")
	}

	return filesArchiveInfo{expandedBytes: validator.expandedBytes}, nil
}

type tarEntryKind int

const (
	tarEntryKindDir tarEntryKind = iota
	tarEntryKindRegular
)

type filesArchiveValidator struct {
	expandedBytes int64
	seen          map[string]tarEntryKind
	files         map[string]struct{}
	dirs          map[string]struct{}
	implicitDirs  map[string]struct{}
}

func newFilesArchiveValidator() *filesArchiveValidator {
	return &filesArchiveValidator{
		seen:         make(map[string]tarEntryKind),
		files:        make(map[string]struct{}),
		dirs:         make(map[string]struct{}),
		implicitDirs: make(map[string]struct{}),
	}
}

func (v *filesArchiveValidator) validate(header *tar.Header) error {
	name, kind, err := validateTarHeaderEntry(header)
	if err != nil {
		return err
	}
	if _, ok := v.seen[name]; ok {
		return fmt.Errorf("tar entry is duplicated: %s", name)
	}
	if err := v.validatePathShape(name, kind); err != nil {
		return err
	}
	if kind == tarEntryKindRegular {
		if header.Size > maxInt64-v.expandedBytes {
			return fmt.Errorf("files archive expanded size is too large")
		}
		v.expandedBytes += header.Size
		v.files[name] = struct{}{}
	} else {
		v.dirs[name] = struct{}{}
	}
	v.seen[name] = kind
	v.addImplicitParents(name)
	return nil
}

func (v *filesArchiveValidator) validatePathShape(name string, kind tarEntryKind) error {
	for parent := path.Dir(name); parent != "."; parent = path.Dir(parent) {
		if _, ok := v.files[parent]; ok {
			return fmt.Errorf("tar entry parent is a file: %s", parent)
		}
	}

	switch kind {
	case tarEntryKindDir:
		if _, ok := v.files[name]; ok {
			return fmt.Errorf("tar entry collides with file: %s", name)
		}
	case tarEntryKindRegular:
		if _, ok := v.dirs[name]; ok {
			return fmt.Errorf("tar entry collides with directory: %s", name)
		}
		if _, ok := v.implicitDirs[name]; ok {
			return fmt.Errorf("tar entry collides with directory: %s", name)
		}
	}
	return nil
}

func (v *filesArchiveValidator) addImplicitParents(name string) {
	for parent := path.Dir(name); parent != "."; parent = path.Dir(parent) {
		v.implicitDirs[parent] = struct{}{}
	}
}

func validateTarHeaderEntry(header *tar.Header) (string, tarEntryKind, error) {
	if header == nil {
		return "", tarEntryKindRegular, fmt.Errorf("tar header is required")
	}
	name := strings.TrimSpace(header.Name)
	if name == "" {
		return "", tarEntryKindRegular, fmt.Errorf("tar entry has empty name")
	}
	clean, err := cleanTarEntryName(name)
	if err != nil {
		return "", tarEntryKindRegular, err
	}
	if err := validateTarHeaderMode(header.Mode); err != nil {
		return "", tarEntryKindRegular, err
	}
	switch header.Typeflag {
	case tar.TypeDir:
		if header.Size != 0 {
			return "", tarEntryKindRegular, fmt.Errorf("tar directory entry has data: %s", name)
		}
		return clean, tarEntryKindDir, nil
	case tar.TypeReg, tarRegularTypeflagZero:
		if header.Size < 0 {
			return "", tarEntryKindRegular, fmt.Errorf("tar entry size is invalid: %s", name)
		}
		return clean, tarEntryKindRegular, nil
	default:
		return "", tarEntryKindRegular, fmt.Errorf("tar entry type is not allowed: %d", header.Typeflag)
	}
}

func cleanTarEntryName(name string) (string, error) {
	if strings.IndexByte(name, 0) >= 0 {
		return "", fmt.Errorf("tar entry has invalid name")
	}
	if path.IsAbs(name) || filepath.IsAbs(name) || hasParentPathSegment(name) {
		return "", fmt.Errorf("tar entry escapes archive root: %s", name)
	}
	clean := path.Clean(name)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") || hasParentPathSegment(clean) {
		return "", fmt.Errorf("tar entry escapes archive root: %s", name)
	}
	return clean, nil
}

func hasParentPathSegment(name string) bool {
	for _, segment := range strings.Split(name, "/") {
		if segment == ".." {
			return true
		}
	}
	return false
}

func validateTarHeaderMode(mode int64) error {
	if mode < 0 {
		return fmt.Errorf("tar entry mode is invalid: %o", mode)
	}
	return nil
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
