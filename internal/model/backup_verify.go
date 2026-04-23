package model

import (
	"archive/tar"
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"
)

const (
	CommandBackupVerify      = "backup verify"
	BackupVerifyFailedCode   = "backup_verification_failed"
	ManifestInvalidCode      = "manifest_invalid"
	StepVerifySource         = "source_selection"
	StepVerifyManifest       = "manifest_contract"
	StepVerifyDBArtifact     = "db_artifact"
	StepVerifyFilesArtifact  = "files_artifact"
	VerifySourceManifest     = "manifest"
	VerifySourceBackupRoot   = "backup_root"
	VerifySourceDirectDB     = "direct_db"
	VerifySourceDirectFiles  = "direct_files"
	legacyTarRegularTypeflag = byte(0)
)

type BackupVerifyStore interface {
	LoadBackupVerifyManifest(ctx context.Context, path string) (BackupVerifyManifest, error)
	ListBackupVerifyManifestCandidates(ctx context.Context, backupRoot string) ([]string, error)
	VerifyDBArtifact(ctx context.Context, path, expectedChecksum string) (Artifact, error)
	VerifyFilesArtifact(ctx context.Context, path, expectedChecksum string) (Artifact, error)
}

type BackupVerifyRequest struct {
	ManifestPath string
	BackupRoot   string
	DBBackupPath string
	FilesPath    string
}

type BackupVerifyManifest struct {
	Version            int               `json:"version"`
	Scope              string            `json:"scope"`
	CreatedAt          string            `json:"created_at"`
	Artifacts          ManifestArtifacts `json:"artifacts"`
	Checksums          ManifestChecksums `json:"checksums"`
	DBBackupCreated    bool              `json:"db_backup_created"`
	FilesBackupCreated bool              `json:"files_backup_created"`
}

func (m BackupVerifyManifest) Normalized() BackupVerifyManifest {
	if m.DBBackupCreated || m.FilesBackupCreated {
		return m
	}
	m.DBBackupCreated = manifestArtifactPresent(m.Artifacts.DBBackup, m.Checksums.DBBackup)
	m.FilesBackupCreated = manifestArtifactPresent(m.Artifacts.FilesBackup, m.Checksums.FilesBackup)
	return m
}

func (m BackupVerifyManifest) ValidateComplete() error {
	m = m.Normalized()
	if m.Version != 1 {
		return fmt.Errorf("неподдерживаемая версия manifest: %d", m.Version)
	}
	if strings.TrimSpace(m.Scope) == "" {
		return fmt.Errorf("scope обязателен")
	}
	if strings.TrimSpace(m.CreatedAt) == "" {
		return fmt.Errorf("created_at обязателен")
	}
	if _, err := time.Parse(time.RFC3339, m.CreatedAt); err != nil {
		return fmt.Errorf("created_at не является RFC3339: %w", err)
	}
	if !m.DBBackupCreated || !m.FilesBackupCreated {
		return fmt.Errorf("manifest допустим только для полного backup-set")
	}
	if err := validateManifestName("artifacts.db_backup", m.Artifacts.DBBackup); err != nil {
		return err
	}
	if err := validateManifestName("artifacts.files_backup", m.Artifacts.FilesBackup); err != nil {
		return err
	}
	if err := ValidateChecksum("checksums.db_backup", m.Checksums.DBBackup); err != nil {
		return err
	}
	if err := ValidateChecksum("checksums.files_backup", m.Checksums.FilesBackup); err != nil {
		return err
	}
	return nil
}

func (m BackupVerifyManifest) DBArtifactPath(manifestPath string) string {
	return ResolveManifestArtifactPath(manifestPath, "db", m.Artifacts.DBBackup)
}

func (m BackupVerifyManifest) FilesArtifactPath(manifestPath string) string {
	return ResolveManifestArtifactPath(manifestPath, "files", m.Artifacts.FilesBackup)
}

func ValidateBackupVerifyManifestCoherence(manifestPath string, manifest BackupVerifyManifest) error {
	dbGroup, dbOK := ParseBackupGroupName(manifest.Artifacts.DBBackup)
	filesGroup, filesOK := ParseBackupGroupName(manifest.Artifacts.FilesBackup)
	if dbOK && filesOK && dbGroup != filesGroup {
		return fmt.Errorf("manifest ссылается на artifacts из разных backup-set")
	}

	manifestGroup, manifestOK := ParseBackupGroupName(manifestPath)
	if !manifestOK {
		return nil
	}
	if !dbOK {
		return fmt.Errorf("db artifact не соответствует canonical имени backup")
	}
	if !filesOK {
		return fmt.Errorf("files artifact не соответствует canonical имени backup")
	}
	if dbGroup != manifestGroup || filesGroup != manifestGroup {
		return fmt.Errorf("manifest и artifacts относятся к разным backup-set")
	}
	return nil
}

func ResolveManifestArtifactPath(manifestPath, artifactDir, artifactName string) string {
	root := filepath.Dir(filepath.Dir(manifestPath))
	return filepath.Join(root, artifactDir, filepath.Base(artifactName))
}

func ValidateFilesArchiveHeader(header *tar.Header) error {
	if header == nil {
		return fmt.Errorf("tar header обязателен")
	}
	name := strings.TrimSpace(header.Name)
	if name == "" {
		return fmt.Errorf("tar entry без имени")
	}
	clean := filepath.ToSlash(filepath.Clean(name))
	if clean == "." || strings.HasPrefix(clean, "../") || clean == ".." || filepath.IsAbs(name) {
		return fmt.Errorf("tar entry выходит за пределы archive root: %s", name)
	}
	switch header.Typeflag {
	case tar.TypeDir, tar.TypeReg, legacyTarRegularTypeflag:
		return nil
	default:
		return fmt.Errorf("tar entry type не разрешён для backup files archive: %d", header.Typeflag)
	}
}

type BackupVerifyDetails struct {
	Ready      bool   `json:"ready"`
	SourceKind string `json:"source_kind"`
	Scope      string `json:"scope,omitempty"`
	CreatedAt  string `json:"created_at,omitempty"`
	Steps      int    `json:"steps"`
	Completed  int    `json:"completed,omitempty"`
	Skipped    int    `json:"skipped,omitempty"`
	Blocked    int    `json:"blocked,omitempty"`
	Failed     int    `json:"failed,omitempty"`
}

type BackupVerifyArtifacts struct {
	BackupRoot    string `json:"backup_root,omitempty"`
	Manifest      string `json:"manifest,omitempty"`
	DBBackup      string `json:"db_backup,omitempty"`
	DBChecksum    string `json:"db_checksum,omitempty"`
	FilesBackup   string `json:"files_backup,omitempty"`
	FilesChecksum string `json:"files_checksum,omitempty"`
}

type BackupVerifyResult struct {
	Command         string                `json:"command"`
	OK              bool                  `json:"ok"`
	ProcessExitCode int                   `json:"process_exit_code"`
	Details         BackupVerifyDetails   `json:"details"`
	Artifacts       BackupVerifyArtifacts `json:"artifacts,omitempty"`
	Items           []Step                `json:"items"`
	Error           *BackupError          `json:"error,omitempty"`
}

func NewBackupVerifyResult(req BackupVerifyRequest) BackupVerifyResult {
	return BackupVerifyResult{
		Command:         CommandBackupVerify,
		ProcessExitCode: ExitIOError,
		Details: BackupVerifyDetails{
			SourceKind: verifySourceKind(req),
		},
		Artifacts: BackupVerifyArtifacts{
			BackupRoot:    strings.TrimSpace(req.BackupRoot),
			Manifest:      strings.TrimSpace(req.ManifestPath),
			DBBackup:      strings.TrimSpace(req.DBBackupPath),
			DBChecksum:    checksumPath(req.DBBackupPath),
			FilesBackup:   strings.TrimSpace(req.FilesPath),
			FilesChecksum: checksumPath(req.FilesPath),
		},
	}
}

func (r *BackupVerifyResult) AddStep(code, status string) {
	r.Items = append(r.Items, Step{Code: code, Status: status})
	r.recount()
}

func (r *BackupVerifyResult) Succeed() {
	r.OK = true
	r.ProcessExitCode = ExitOK
	r.Error = nil
	r.recount()
}

func (r *BackupVerifyResult) Fail(f BackupFailure) {
	r.OK = false
	r.ProcessExitCode = f.BackupError.ExitCode
	errCopy := f.BackupError
	r.Error = &errCopy
	r.recount()
}

func (r *BackupVerifyResult) recount() {
	var completed, skipped, blocked, failed int
	for _, step := range r.Items {
		switch step.Status {
		case StatusCompleted:
			completed++
		case StatusSkipped:
			skipped++
		case StatusBlocked:
			blocked++
		case StatusFailed:
			failed++
		}
	}
	r.Details.Steps = len(r.Items)
	r.Details.Completed = completed
	r.Details.Skipped = skipped
	r.Details.Blocked = blocked
	r.Details.Failed = failed
	r.Details.Ready = failed == 0 && blocked == 0 && len(r.Items) > 0
}

type BackupVerifyReport struct {
	ManifestPath string
	BackupRoot   string
	Scope        string
	CreatedAt    string
	DBBackup     Artifact
	FilesBackup  Artifact
	SourceKind   string
}

func (r *BackupVerifyResult) ApplyReport(report BackupVerifyReport) {
	r.Details.SourceKind = report.SourceKind
	r.Details.Scope = report.Scope
	r.Details.CreatedAt = report.CreatedAt
	r.Artifacts.BackupRoot = report.BackupRoot
	r.Artifacts.Manifest = report.ManifestPath
	if report.DBBackup.Path != "" {
		r.Artifacts.DBBackup = report.DBBackup.Path
		r.Artifacts.DBChecksum = report.DBBackup.ChecksumPath
	}
	if report.FilesBackup.Path != "" {
		r.Artifacts.FilesBackup = report.FilesBackup.Path
		r.Artifacts.FilesChecksum = report.FilesBackup.ChecksumPath
	}
}

func NewBackupVerifyFailure(kind ErrorKind, code, message string, cause error) BackupFailure {
	return NewCommandFailure(kind, code, message, cause)
}

type BackupVerifyManifestError struct {
	Err error
}

func (e BackupVerifyManifestError) Error() string {
	return e.Err.Error()
}

func (e BackupVerifyManifestError) Unwrap() error {
	return e.Err
}

type BackupVerifyArtifactError struct {
	Label string
	Err   error
}

func (e BackupVerifyArtifactError) Error() string {
	if strings.TrimSpace(e.Label) == "" {
		return e.Err.Error()
	}
	return e.Label + ": " + e.Err.Error()
}

func (e BackupVerifyArtifactError) Unwrap() error {
	return e.Err
}

type BackupVerifySelectionError struct {
	Err error
}

func (e BackupVerifySelectionError) Error() string {
	return e.Err.Error()
}

func (e BackupVerifySelectionError) Unwrap() error {
	return e.Err
}

func NewArtifactFromVerification(path, checksum string, sizeBytes int64) Artifact {
	return Artifact{
		Path:         path,
		Name:         filepath.Base(path),
		ChecksumPath: checksumPath(path),
		Checksum:     strings.ToLower(strings.TrimSpace(checksum)),
		SizeBytes:    sizeBytes,
	}
}

func manifestArtifactPresent(name, checksum string) bool {
	return strings.TrimSpace(name) != "" || strings.TrimSpace(checksum) != ""
}

func verifySourceKind(req BackupVerifyRequest) string {
	switch {
	case strings.TrimSpace(req.ManifestPath) != "":
		return VerifySourceManifest
	case strings.TrimSpace(req.BackupRoot) != "":
		return VerifySourceBackupRoot
	case strings.TrimSpace(req.DBBackupPath) != "":
		return VerifySourceDirectDB
	case strings.TrimSpace(req.FilesPath) != "":
		return VerifySourceDirectFiles
	default:
		return ""
	}
}

func checksumPath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	return path + ".sha256"
}
