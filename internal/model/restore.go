package model

import (
	"context"
	"strings"
)

const (
	CommandRestore              = "restore"
	RestoreFailedCode           = "restore_failed"
	RestoreStepSourceResolution = "source_resolution"
	RestoreStepSnapshot         = "snapshot_recovery_point"
	RestoreStepDBRestore        = "db_restore"
	RestoreStepFilesRestore     = "files_restore"
	RestoreStepPermission       = "permission_reconcile"
	RestoreStepPostCheck        = "post_restore_check"
	RestoreSourceManifest       = "manifest"
	RestoreSourceDirect         = "direct"
	RestoreSelectionManifest    = "manifest_full"
	RestoreSelectionDirectPair  = "direct_pair"
	RestoreSelectionDirectDB    = "direct_db_only"
	RestoreSelectionDirectFiles = "direct_files_only"
)

type RestoreRuntime interface {
	RunningServices(ctx context.Context, target RuntimeTarget) ([]string, error)
	StopServices(ctx context.Context, target RuntimeTarget, services ...string) error
	StartServices(ctx context.Context, target RuntimeTarget, services ...string) error
	RestoreDatabase(ctx context.Context, target RuntimeTarget, dbBackupPath string) error
	ReconcileFilesPermissions(ctx context.Context, target RuntimeTarget) error
	PostRestoreCheck(ctx context.Context, target RuntimeTarget, services ...string) error
}

type RestoreStore interface {
	LoadRestoreManifest(ctx context.Context, path string) (BackupVerifyManifest, error)
	VerifyDBArtifact(ctx context.Context, path, expectedChecksum string) (Artifact, error)
	VerifyFilesArtifact(ctx context.Context, path, expectedChecksum string) (Artifact, error)
	RestoreFilesArtifact(ctx context.Context, filesBackupPath, targetDir string, requireExactRoot bool) error
}

type RestoreRequest struct {
	Scope       string
	ProjectDir  string
	ComposeFile string
	EnvFile     string
	BackupRoot  string
	StorageDir  string
	Manifest    string
	DBBackup    string
	FilesBackup string
	SkipDB      bool
	SkipFiles   bool
	NoSnapshot  bool
	NoStop      bool
	NoStart     bool
	Target      RuntimeTarget
	Snapshot    BackupRequest
}

type RestoreDetails struct {
	Scope                  string `json:"scope"`
	Ready                  bool   `json:"ready"`
	SelectionMode          string `json:"selection_mode,omitempty"`
	SourceKind             string `json:"source_kind,omitempty"`
	Steps                  int    `json:"steps"`
	Completed              int    `json:"completed,omitempty"`
	Skipped                int    `json:"skipped,omitempty"`
	Blocked                int    `json:"blocked,omitempty"`
	Failed                 int    `json:"failed,omitempty"`
	Warnings               int    `json:"warnings,omitempty"`
	SnapshotEnabled        bool   `json:"snapshot_enabled"`
	SkipDB                 bool   `json:"skip_db"`
	SkipFiles              bool   `json:"skip_files"`
	NoStop                 bool   `json:"no_stop"`
	NoStart                bool   `json:"no_start"`
	AppServicesWereRunning bool   `json:"app_services_were_running"`
}

type RestoreArtifacts struct {
	ProjectDir            string `json:"project_dir,omitempty"`
	ComposeFile           string `json:"compose_file,omitempty"`
	EnvFile               string `json:"env_file,omitempty"`
	BackupRoot            string `json:"backup_root,omitempty"`
	ManifestTXT           string `json:"manifest_txt,omitempty"`
	ManifestJSON          string `json:"manifest_json,omitempty"`
	DBBackup              string `json:"db_backup,omitempty"`
	FilesBackup           string `json:"files_backup,omitempty"`
	SnapshotManifestTXT   string `json:"snapshot_manifest_txt,omitempty"`
	SnapshotManifestJSON  string `json:"snapshot_manifest_json,omitempty"`
	SnapshotDBBackup      string `json:"snapshot_db_backup,omitempty"`
	SnapshotFilesBackup   string `json:"snapshot_files_backup,omitempty"`
	SnapshotDBChecksum    string `json:"snapshot_db_checksum,omitempty"`
	SnapshotFilesChecksum string `json:"snapshot_files_checksum,omitempty"`
}

type RestoreResult struct {
	Command         string           `json:"command"`
	OK              bool             `json:"ok"`
	ProcessExitCode int              `json:"process_exit_code"`
	Warnings        []string         `json:"warnings,omitempty"`
	Details         RestoreDetails   `json:"details"`
	Artifacts       RestoreArtifacts `json:"artifacts,omitempty"`
	Items           []Step           `json:"items"`
	Error           *BackupError     `json:"error,omitempty"`
}

type RestoreSource struct {
	SelectionMode    string
	SourceKind       string
	ManifestPath     string
	DBBackup         Artifact
	FilesBackup      Artifact
	DirectFilesExact bool
}

func NewRestoreResult(req RestoreRequest) RestoreResult {
	return RestoreResult{
		Command:         CommandRestore,
		ProcessExitCode: ExitIOError,
		Details: RestoreDetails{
			Scope:           strings.TrimSpace(req.Scope),
			SnapshotEnabled: !req.NoSnapshot,
			SkipDB:          req.SkipDB,
			SkipFiles:       req.SkipFiles,
			NoStop:          req.NoStop,
			NoStart:         req.NoStart,
		},
		Artifacts: RestoreArtifacts{
			ProjectDir:   strings.TrimSpace(req.ProjectDir),
			ComposeFile:  strings.TrimSpace(req.ComposeFile),
			EnvFile:      strings.TrimSpace(req.EnvFile),
			BackupRoot:   strings.TrimSpace(req.BackupRoot),
			ManifestJSON: strings.TrimSpace(req.Manifest),
			DBBackup:     strings.TrimSpace(req.DBBackup),
			FilesBackup:  strings.TrimSpace(req.FilesBackup),
		},
	}
}

func (r *RestoreResult) AddStep(code, status string) {
	if code == "" {
		return
	}
	r.Items = append(r.Items, Step{Code: code, Status: status})
	r.recount()
}

func (r *RestoreResult) AddWarning(message string) {
	message = strings.TrimSpace(message)
	if message == "" {
		return
	}
	r.Warnings = append(r.Warnings, message)
	r.Details.Warnings = len(r.Warnings)
}

func (r *RestoreResult) ApplySource(source RestoreSource) {
	r.Details.SelectionMode = source.SelectionMode
	r.Details.SourceKind = source.SourceKind
	r.Artifacts.ManifestTXT = matchingRestoreManifestTXT(source.ManifestPath)
	r.Artifacts.ManifestJSON = source.ManifestPath
	if source.DBBackup.Path != "" {
		r.Artifacts.DBBackup = source.DBBackup.Path
	}
	if source.FilesBackup.Path != "" {
		r.Artifacts.FilesBackup = source.FilesBackup.Path
	}
}

func (r *RestoreResult) ApplySnapshot(snapshot BackupResult) {
	r.Artifacts.SnapshotManifestTXT = snapshot.Artifacts.ManifestText
	r.Artifacts.SnapshotManifestJSON = snapshot.Artifacts.ManifestJSON
	r.Artifacts.SnapshotDBBackup = snapshot.Artifacts.DBBackup
	r.Artifacts.SnapshotFilesBackup = snapshot.Artifacts.FilesBackup
	r.Artifacts.SnapshotDBChecksum = snapshot.Artifacts.DBChecksum
	r.Artifacts.SnapshotFilesChecksum = snapshot.Artifacts.FilesChecksum
}

func (r *RestoreResult) Succeed() {
	r.OK = true
	r.ProcessExitCode = ExitOK
	r.Error = nil
	r.recount()
}

func (r *RestoreResult) Fail(f BackupFailure) {
	r.OK = false
	r.ProcessExitCode = f.BackupError.ExitCode
	errCopy := f.BackupError
	r.Error = &errCopy
	r.recount()
}

func (r *RestoreResult) recount() {
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

func NewRestoreFailure(kind ErrorKind, message string, cause error) BackupFailure {
	return NewCommandFailure(kind, RestoreFailedCode, message, cause)
}

func matchingRestoreManifestTXT(path string) string {
	if !strings.HasSuffix(path, ".manifest.json") {
		return ""
	}
	return strings.TrimSuffix(path, ".manifest.json") + ".manifest.txt"
}
