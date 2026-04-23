package model

import (
	"context"
	"encoding/hex"
	"fmt"
	"io"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

const (
	CommandBackup          = "backup"
	BackupFailedCode       = "backup_failed"
	ExitOK                 = 0
	ExitUsageError         = 2
	ExitManifestError      = 3
	ExitValidationError    = 4
	ExitExternalError      = 5
	ExitIOError            = 6
	StatusPlanned          = "planned"
	StatusCompleted        = "completed"
	StatusSkipped          = "skipped"
	StatusBlocked          = "blocked"
	StatusFailed           = "failed"
	StepArtifactAllocation = "artifact_allocation"
	StepRuntimePrepare     = "runtime_prepare"
	StepDBBackup           = "db_backup"
	StepFilesBackup        = "files_backup"
	StepFinalize           = "finalize"
	StepRetention          = "retention"
	StepRuntimeReturn      = "runtime_return"
)

type ErrorKind string

const (
	KindUsage      ErrorKind = "usage"
	KindManifest   ErrorKind = "manifest"
	KindValidation ErrorKind = "validation"
	KindExternal   ErrorKind = "external"
	KindIO         ErrorKind = "io"
)

type BackupFailure struct {
	BackupError
	Cause error `json:"-"`
}

type BackupError struct {
	Kind     ErrorKind `json:"kind"`
	Code     string    `json:"code"`
	ExitCode int       `json:"exit_code"`
	Message  string    `json:"message,omitempty"`
}

func NewBackupFailure(kind ErrorKind, message string, cause error) BackupFailure {
	return NewCommandFailure(kind, BackupFailedCode, message, cause)
}

func NewCommandFailure(kind ErrorKind, code, message string, cause error) BackupFailure {
	code = strings.TrimSpace(code)
	if code == "" {
		code = BackupFailedCode
	}
	return BackupFailure{
		BackupError: BackupError{
			Kind:     kind,
			Code:     code,
			ExitCode: exitCodeForKind(kind),
			Message:  strings.TrimSpace(message),
		},
		Cause: cause,
	}
}

func (f BackupFailure) Error() string {
	if f.Cause == nil {
		return f.Message
	}
	if strings.TrimSpace(f.Message) == "" {
		return f.Cause.Error()
	}
	return f.Message + ": " + f.Cause.Error()
}

func (f BackupFailure) Unwrap() error {
	return f.Cause
}

func exitCodeForKind(kind ErrorKind) int {
	switch kind {
	case KindUsage:
		return ExitUsageError
	case KindManifest:
		return ExitManifestError
	case KindValidation:
		return ExitValidationError
	case KindExternal:
		return ExitExternalError
	case KindIO:
		return ExitIOError
	default:
		return ExitIOError
	}
}

type Runtime interface {
	RunningServices(ctx context.Context, target RuntimeTarget) ([]string, error)
	StopServices(ctx context.Context, target RuntimeTarget, services ...string) error
	StartServices(ctx context.Context, target RuntimeTarget, services ...string) error
	DumpDatabase(ctx context.Context, target RuntimeTarget) (io.ReadCloser, error)
	ArchiveFiles(ctx context.Context, target RuntimeTarget) (io.ReadCloser, error)
	ArchiveFilesWithHelper(ctx context.Context, target RuntimeTarget, contract HelperArchiveContract) (io.ReadCloser, error)
}

type RuntimeTarget struct {
	ProjectDir       string
	ComposeFile      string
	EnvFile          string
	StorageDir       string
	DBService        string
	DBUser           string
	DBPassword       string
	DBRootPassword   string
	DBName           string
	HelperImage      string
	RuntimeUID       int
	RuntimeGID       int
	ReadinessTimeout int
}

type Store interface {
	EnsureLayout(ctx context.Context, layout BackupLayout) error
	AnyExists(ctx context.Context, paths []string) (bool, error)
	WriteArtifact(ctx context.Context, path string, body io.Reader) (Artifact, error)
	WriteChecksum(ctx context.Context, artifact Artifact) error
	WriteManifestJSON(ctx context.Context, path string, manifest CompleteManifest) error
	WriteManifestText(ctx context.Context, path string, manifest CompleteManifest) error
	ListBackupGroups(ctx context.Context, root string) ([]BackupGroup, error)
	RemoveBackupSet(ctx context.Context, layout BackupLayout) error
	CleanupLayoutTemps(ctx context.Context, layout BackupLayout) error
}

type BackupRequest struct {
	Scope         string
	ProjectDir    string
	ComposeFile   string
	EnvFile       string
	BackupRoot    string
	StorageDir    string
	NamePrefix    string
	RetentionDays int
	CreatedAt     time.Time
	DBService     string
	DBUser        string
	DBPassword    string
	DBName        string
	SkipDB        bool
	SkipFiles     bool
	NoStop        bool
	HelperArchive HelperArchiveContract
	Metadata      BackupMetadata
}

type HelperArchiveContract struct {
	Image string
}

type BackupMetadata struct {
	ComposeProject string
	EnvFileName    string
	EspoCRMImage   string
	MariaDBTag     string
}

type BackupLayout struct {
	Root          string
	Prefix        string
	Stamp         string
	DBArtifact    string
	DBChecksum    string
	FilesArtifact string
	FilesChecksum string
	ManifestJSON  string
	ManifestText  string
}

func NewBackupLayout(root, prefix string, createdAt time.Time) BackupLayout {
	stamp := FormatStamp(createdAt)
	dbArtifact := filepath.Join(root, "db", fmt.Sprintf("%s_%s.sql.gz", prefix, stamp))
	filesArtifact := filepath.Join(root, "files", fmt.Sprintf("%s_files_%s.tar.gz", prefix, stamp))
	return BackupLayout{
		Root:          root,
		Prefix:        prefix,
		Stamp:         stamp,
		DBArtifact:    dbArtifact,
		DBChecksum:    dbArtifact + ".sha256",
		FilesArtifact: filesArtifact,
		FilesChecksum: filesArtifact + ".sha256",
		ManifestJSON:  filepath.Join(root, "manifests", fmt.Sprintf("%s_%s.manifest.json", prefix, stamp)),
		ManifestText:  filepath.Join(root, "manifests", fmt.Sprintf("%s_%s.manifest.txt", prefix, stamp)),
	}
}

func NewBackupLayoutForStamp(root, prefix, stamp string) BackupLayout {
	dbArtifact := filepath.Join(root, "db", fmt.Sprintf("%s_%s.sql.gz", prefix, stamp))
	filesArtifact := filepath.Join(root, "files", fmt.Sprintf("%s_files_%s.tar.gz", prefix, stamp))
	return BackupLayout{
		Root:          root,
		Prefix:        prefix,
		Stamp:         stamp,
		DBArtifact:    dbArtifact,
		DBChecksum:    dbArtifact + ".sha256",
		FilesArtifact: filesArtifact,
		FilesChecksum: filesArtifact + ".sha256",
		ManifestJSON:  filepath.Join(root, "manifests", fmt.Sprintf("%s_%s.manifest.json", prefix, stamp)),
		ManifestText:  filepath.Join(root, "manifests", fmt.Sprintf("%s_%s.manifest.txt", prefix, stamp)),
	}
}

func FormatStamp(createdAt time.Time) string {
	return createdAt.UTC().Format("2006-01-02_15-04-05")
}

func ParseStamp(stamp string) (time.Time, error) {
	return time.Parse("2006-01-02_15-04-05", stamp)
}

func (l BackupLayout) SelectedPaths(includeDB, includeFiles, includeManifest bool) []string {
	var paths []string
	if includeDB {
		paths = append(paths, l.DBArtifact, l.DBChecksum)
	}
	if includeFiles {
		paths = append(paths, l.FilesArtifact, l.FilesChecksum)
	}
	if includeManifest {
		paths = append(paths, l.ManifestJSON, l.ManifestText)
	}
	return paths
}

func (l BackupLayout) CompleteSetPaths() []string {
	return []string{
		l.DBArtifact,
		l.DBChecksum,
		l.FilesArtifact,
		l.FilesChecksum,
		l.ManifestJSON,
		l.ManifestText,
	}
}

func (l BackupLayout) TempPaths() []string {
	var paths []string
	for _, path := range l.CompleteSetPaths() {
		paths = append(paths, path+".tmp")
	}
	return paths
}

type Artifact struct {
	Path         string
	Name         string
	ChecksumPath string
	Checksum     string
	SizeBytes    int64
}

type BackupGroup struct {
	Prefix string
	Stamp  string
}

type ManifestArtifacts struct {
	DBBackup    string `json:"db_backup"`
	FilesBackup string `json:"files_backup"`
}

type ManifestChecksums struct {
	DBBackup    string `json:"db_backup"`
	FilesBackup string `json:"files_backup"`
}

type CompleteManifest struct {
	Version                int               `json:"version"`
	Scope                  string            `json:"scope"`
	CreatedAt              string            `json:"created_at"`
	Contour                string            `json:"contour"`
	ComposeProject         string            `json:"compose_project"`
	EnvFile                string            `json:"env_file"`
	EspoCRMImage           string            `json:"espocrm_image"`
	MariaDBTag             string            `json:"mariadb_tag"`
	RetentionDays          int               `json:"retention_days"`
	ConsistentSnapshot     bool              `json:"consistent_snapshot"`
	AppServicesWereRunning bool              `json:"app_services_were_running"`
	Artifacts              ManifestArtifacts `json:"artifacts"`
	Checksums              ManifestChecksums `json:"checksums"`
	DBBackupCreated        bool              `json:"db_backup_created"`
	FilesBackupCreated     bool              `json:"files_backup_created"`
	DBBackupSizeBytes      int64             `json:"-"`
	FilesBackupSizeBytes   int64             `json:"-"`
}

func NewCompleteManifest(req BackupRequest, dbArtifact, filesArtifact Artifact, appServicesWereRunning bool) (CompleteManifest, error) {
	manifest := CompleteManifest{
		Version:                1,
		Scope:                  strings.TrimSpace(req.Scope),
		CreatedAt:              req.CreatedAt.UTC().Format(time.RFC3339),
		Contour:                strings.TrimSpace(req.Scope),
		ComposeProject:         strings.TrimSpace(req.Metadata.ComposeProject),
		EnvFile:                strings.TrimSpace(req.Metadata.EnvFileName),
		EspoCRMImage:           strings.TrimSpace(req.Metadata.EspoCRMImage),
		MariaDBTag:             strings.TrimSpace(req.Metadata.MariaDBTag),
		RetentionDays:          req.RetentionDays,
		ConsistentSnapshot:     !req.NoStop,
		AppServicesWereRunning: appServicesWereRunning,
		Artifacts: ManifestArtifacts{
			DBBackup:    filepath.Base(dbArtifact.Path),
			FilesBackup: filepath.Base(filesArtifact.Path),
		},
		Checksums: ManifestChecksums{
			DBBackup:    dbArtifact.Checksum,
			FilesBackup: filesArtifact.Checksum,
		},
		DBBackupCreated:      true,
		FilesBackupCreated:   true,
		DBBackupSizeBytes:    dbArtifact.SizeBytes,
		FilesBackupSizeBytes: filesArtifact.SizeBytes,
	}
	if err := manifest.ValidateComplete(); err != nil {
		return CompleteManifest{}, err
	}
	return manifest, nil
}

func (m CompleteManifest) ValidateComplete() error {
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
	if strings.TrimSpace(m.ComposeProject) == "" {
		return fmt.Errorf("compose_project обязателен")
	}
	if strings.TrimSpace(m.EnvFile) == "" {
		return fmt.Errorf("env_file обязателен")
	}
	if strings.TrimSpace(m.EspoCRMImage) == "" {
		return fmt.Errorf("espocrm_image обязателен")
	}
	if strings.TrimSpace(m.MariaDBTag) == "" {
		return fmt.Errorf("mariadb_tag обязателен")
	}
	return nil
}

func validateManifestName(field, value string) error {
	value = strings.TrimSpace(value)
	if value == "" {
		return fmt.Errorf("%s обязателен", field)
	}
	if filepath.Base(value) != value || value == "." || value == ".." {
		return fmt.Errorf("%s должен быть именем файла без пути", field)
	}
	return nil
}

func ValidateChecksum(field, value string) error {
	value = strings.TrimSpace(value)
	if len(value) != 64 {
		return fmt.Errorf("%s должен быть 64-символьным sha256", field)
	}
	if _, err := hex.DecodeString(strings.ToLower(value)); err != nil {
		return fmt.Errorf("%s должен быть hex: %w", field, err)
	}
	return nil
}

type BackupArtifacts struct {
	ProjectDir    string `json:"project_dir,omitempty"`
	ComposeFile   string `json:"compose_file,omitempty"`
	EnvFile       string `json:"env_file,omitempty"`
	BackupRoot    string `json:"backup_root,omitempty"`
	ManifestText  string `json:"manifest_txt,omitempty"`
	ManifestJSON  string `json:"manifest_json,omitempty"`
	DBBackup      string `json:"db_backup,omitempty"`
	FilesBackup   string `json:"files_backup,omitempty"`
	DBChecksum    string `json:"db_checksum,omitempty"`
	FilesChecksum string `json:"files_checksum,omitempty"`
}

type BackupDetails struct {
	Scope                  string `json:"scope"`
	Ready                  bool   `json:"ready"`
	CreatedAt              string `json:"created_at,omitempty"`
	SkipDB                 bool   `json:"skip_db"`
	SkipFiles              bool   `json:"skip_files"`
	NoStop                 bool   `json:"no_stop"`
	ConsistentSnapshot     bool   `json:"consistent_snapshot"`
	AppServicesWereRunning bool   `json:"app_services_were_running"`
	RetentionDays          int    `json:"retention_days"`
	Steps                  int    `json:"steps"`
	Completed              int    `json:"completed,omitempty"`
	Skipped                int    `json:"skipped,omitempty"`
	Blocked                int    `json:"blocked,omitempty"`
	Failed                 int    `json:"failed,omitempty"`
	Warnings               int    `json:"warnings,omitempty"`
}

type Step struct {
	Code   string `json:"code"`
	Status string `json:"status"`
}

type BackupResult struct {
	Command         string          `json:"command"`
	OK              bool            `json:"ok"`
	ProcessExitCode int             `json:"process_exit_code"`
	Warnings        []string        `json:"warnings,omitempty"`
	Details         BackupDetails   `json:"details"`
	Artifacts       BackupArtifacts `json:"artifacts,omitempty"`
	Items           []Step          `json:"items"`
	Error           *BackupError    `json:"error,omitempty"`
}

func NewBackupResult(req BackupRequest) BackupResult {
	return BackupResult{
		Command:         CommandBackup,
		ProcessExitCode: ExitIOError,
		Details: BackupDetails{
			Scope:              strings.TrimSpace(req.Scope),
			SkipDB:             req.SkipDB,
			SkipFiles:          req.SkipFiles,
			NoStop:             req.NoStop,
			ConsistentSnapshot: !req.NoStop,
			RetentionDays:      req.RetentionDays,
		},
		Artifacts: BackupArtifacts{
			ProjectDir:  strings.TrimSpace(req.ProjectDir),
			ComposeFile: strings.TrimSpace(req.ComposeFile),
			EnvFile:     strings.TrimSpace(req.EnvFile),
			BackupRoot:  strings.TrimSpace(req.BackupRoot),
		},
	}
}

func (r *BackupResult) SetLayout(layout BackupLayout, includeDB, includeFiles, includeManifest bool) {
	r.Artifacts.BackupRoot = layout.Root
	if includeDB {
		r.Artifacts.DBBackup = layout.DBArtifact
		r.Artifacts.DBChecksum = layout.DBChecksum
	}
	if includeFiles {
		r.Artifacts.FilesBackup = layout.FilesArtifact
		r.Artifacts.FilesChecksum = layout.FilesChecksum
	}
	if includeManifest {
		r.Artifacts.ManifestJSON = layout.ManifestJSON
		r.Artifacts.ManifestText = layout.ManifestText
	}
}

func (r *BackupResult) AddStep(code, status string) {
	r.Items = append(r.Items, Step{Code: code, Status: status})
	r.recount()
}

func (r *BackupResult) AddWarning(message string) {
	message = strings.TrimSpace(message)
	if message == "" {
		return
	}
	r.Warnings = append(r.Warnings, message)
	r.Details.Warnings = len(r.Warnings)
}

func (r *BackupResult) Succeed() {
	r.OK = true
	r.ProcessExitCode = ExitOK
	r.Error = nil
	r.recount()
}

func (r *BackupResult) Fail(f BackupFailure) {
	r.OK = false
	r.ProcessExitCode = f.BackupError.ExitCode
	errCopy := f.BackupError
	r.Error = &errCopy
	r.recount()
}

func (r *BackupResult) recount() {
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
	r.Details.Warnings = len(r.Warnings)
	r.Details.Ready = failed == 0 && blocked == 0 && len(r.Items) > 0
}

var (
	dbNamePattern       = regexp.MustCompile(`^(.+)_([0-9]{4}-[0-9]{2}-[0-9]{2}_[0-9]{2}-[0-9]{2}-[0-9]{2})\.sql\.gz$`)
	filesNamePattern    = regexp.MustCompile(`^(.+)_files_([0-9]{4}-[0-9]{2}-[0-9]{2}_[0-9]{2}-[0-9]{2}-[0-9]{2})\.tar\.gz$`)
	manifestNamePattern = regexp.MustCompile(`^(.+)_([0-9]{4}-[0-9]{2}-[0-9]{2}_[0-9]{2}-[0-9]{2}-[0-9]{2})\.manifest\.(json|txt)$`)
)

func ParseBackupGroupName(name string) (BackupGroup, bool) {
	name = strings.TrimSuffix(filepath.Base(name), ".sha256")
	for _, pattern := range []*regexp.Regexp{dbNamePattern, filesNamePattern, manifestNamePattern} {
		match := pattern.FindStringSubmatch(name)
		if match == nil {
			continue
		}
		return BackupGroup{Prefix: match[1], Stamp: match[2]}, true
	}
	return BackupGroup{}, false
}

func SortBackupGroups(groups []BackupGroup) {
	sort.Slice(groups, func(i, j int) bool {
		if groups[i].Prefix == groups[j].Prefix {
			return groups[i].Stamp < groups[j].Stamp
		}
		return groups[i].Prefix < groups[j].Prefix
	})
}

func ApplicationServices() []string {
	return []string{"espocrm", "espocrm-daemon", "espocrm-websocket"}
}

func RunningApplicationServices(running []string) []string {
	seen := map[string]struct{}{}
	for _, service := range running {
		service = strings.TrimSpace(service)
		if service == "" {
			continue
		}
		seen[service] = struct{}{}
	}

	var selected []string
	for _, service := range ApplicationServices() {
		if _, ok := seen[service]; ok {
			selected = append(selected, service)
		}
	}
	return selected
}

func (f BackupFailure) ExitCode() int {
	return f.BackupError.ExitCode
}

func (f BackupFailure) FailureCode() string {
	return f.Code
}

func (f BackupFailure) ErrorKind() string {
	return string(f.Kind)
}
