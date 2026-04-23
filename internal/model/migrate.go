package model

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
)

const (
	CommandMigrate               = "migrate"
	MigrateFailedCode            = "migrate_failed"
	MigrateStepSourceSelection   = "source_selection"
	MigrateStepCompatibility     = "compatibility"
	MigrateStepTargetSnapshot    = "target_snapshot"
	MigrateStepDBApply           = "db_restore"
	MigrateStepFilesApply        = "files_restore"
	MigrateStepPermission        = "permission_reconcile"
	MigrateStepPostCheck         = "post_migrate_check"
	MigrateSourceBackupRoot      = "backup_root"
	MigrateSourceDirect          = "direct"
	MigrateSelectionLatestFull   = "auto_latest_complete"
	MigrateSelectionExplicitPair = "explicit_pair"
	MigrateSelectionExplicitDB   = "explicit_db_only"
	MigrateSelectionExplicitFile = "explicit_files_only"
)

type MigrateRuntime interface {
	RunningServices(ctx context.Context, target RuntimeTarget) ([]string, error)
	StopServices(ctx context.Context, target RuntimeTarget, services ...string) error
	StartServices(ctx context.Context, target RuntimeTarget, services ...string) error
	RestoreDatabase(ctx context.Context, target RuntimeTarget, dbBackupPath string) error
	ReconcileFilesPermissions(ctx context.Context, target RuntimeTarget) error
	PostRestoreCheck(ctx context.Context, target RuntimeTarget, services ...string) error
}

type MigrateStore interface {
	LoadRestoreManifest(ctx context.Context, path string) (BackupVerifyManifest, error)
	ListBackupGroups(ctx context.Context, root string) ([]BackupGroup, error)
	VerifyDBArtifact(ctx context.Context, path, expectedChecksum string) (Artifact, error)
	VerifyFilesArtifact(ctx context.Context, path, expectedChecksum string) (Artifact, error)
	RestoreFilesArtifact(ctx context.Context, filesBackupPath, targetDir string, requireExactRoot bool) error
}

type MigrateCompatibilitySettings struct {
	EspoCRMImage    string
	MariaDBTag      string
	DefaultLanguage string
	TimeZone        string
}

type MigrateCompatibilityMismatch struct {
	Name        string `json:"name"`
	SourceValue string `json:"source_value"`
	TargetValue string `json:"target_value"`
}

type MigrateRequest struct {
	SourceScope      string
	TargetScope      string
	ProjectDir       string
	ComposeFile      string
	SourceEnvFile    string
	TargetEnvFile    string
	SourceBackupRoot string
	TargetBackupRoot string
	DBBackup         string
	FilesBackup      string
	SkipDB           bool
	SkipFiles        bool
	NoStart          bool
	StorageDir       string
	Target           RuntimeTarget
	Snapshot         BackupRequest
	SourceSettings   MigrateCompatibilitySettings
	TargetSettings   MigrateCompatibilitySettings
}

type MigrateDetails struct {
	SourceScope            string `json:"source_scope"`
	TargetScope            string `json:"target_scope"`
	Ready                  bool   `json:"ready"`
	SelectionMode          string `json:"selection_mode,omitempty"`
	SourceKind             string `json:"source_kind,omitempty"`
	SnapshotEnabled        bool   `json:"snapshot_enabled"`
	SkipDB                 bool   `json:"skip_db"`
	SkipFiles              bool   `json:"skip_files"`
	NoStart                bool   `json:"no_start"`
	AppServicesWereRunning bool   `json:"app_services_were_running"`
	StartedDBTemporarily   bool   `json:"started_db_temporarily"`
	Steps                  int    `json:"steps"`
	Planned                int    `json:"planned,omitempty"`
	Completed              int    `json:"completed,omitempty"`
	Skipped                int    `json:"skipped,omitempty"`
	Blocked                int    `json:"blocked,omitempty"`
	Failed                 int    `json:"failed,omitempty"`
	Warnings               int    `json:"warnings,omitempty"`
}

type MigrateArtifacts struct {
	ProjectDir            string `json:"project_dir,omitempty"`
	ComposeFile           string `json:"compose_file,omitempty"`
	SourceEnvFile         string `json:"source_env_file,omitempty"`
	TargetEnvFile         string `json:"target_env_file,omitempty"`
	SourceBackupRoot      string `json:"source_backup_root,omitempty"`
	TargetBackupRoot      string `json:"target_backup_root,omitempty"`
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

type MigrateResult struct {
	Command         string           `json:"command"`
	OK              bool             `json:"ok"`
	ProcessExitCode int              `json:"process_exit_code"`
	Warnings        []string         `json:"warnings,omitempty"`
	Details         MigrateDetails   `json:"details"`
	Artifacts       MigrateArtifacts `json:"artifacts,omitempty"`
	Items           []Step           `json:"items"`
	Error           *BackupError     `json:"error,omitempty"`
}

type MigrateSource struct {
	SelectionMode    string
	SourceKind       string
	ManifestPath     string
	DBBackup         Artifact
	FilesBackup      Artifact
	DirectFilesExact bool
}

func NewMigrateResult(req MigrateRequest) MigrateResult {
	return MigrateResult{
		Command:         CommandMigrate,
		ProcessExitCode: ExitIOError,
		Details: MigrateDetails{
			SourceScope:     strings.TrimSpace(req.SourceScope),
			TargetScope:     strings.TrimSpace(req.TargetScope),
			SnapshotEnabled: true,
			SkipDB:          req.SkipDB,
			SkipFiles:       req.SkipFiles,
			NoStart:         req.NoStart,
		},
		Artifacts: MigrateArtifacts{
			ProjectDir:       strings.TrimSpace(req.ProjectDir),
			ComposeFile:      strings.TrimSpace(req.ComposeFile),
			SourceEnvFile:    strings.TrimSpace(req.SourceEnvFile),
			TargetEnvFile:    strings.TrimSpace(req.TargetEnvFile),
			SourceBackupRoot: strings.TrimSpace(req.SourceBackupRoot),
			TargetBackupRoot: strings.TrimSpace(req.TargetBackupRoot),
			DBBackup:         strings.TrimSpace(req.DBBackup),
			FilesBackup:      strings.TrimSpace(req.FilesBackup),
		},
	}
}

func (r *MigrateResult) AddStep(code, status string) {
	if strings.TrimSpace(code) == "" {
		return
	}
	r.Items = append(r.Items, Step{Code: code, Status: status})
	r.recount()
}

func (r *MigrateResult) AddWarning(message string) {
	message = strings.TrimSpace(message)
	if message == "" {
		return
	}
	r.Warnings = append(r.Warnings, message)
	r.Details.Warnings = len(r.Warnings)
}

func (r *MigrateResult) ApplySource(source MigrateSource) {
	r.Details.SelectionMode = source.SelectionMode
	r.Details.SourceKind = source.SourceKind
	r.Artifacts.ManifestTXT = matchingMigrateManifestTXT(source.ManifestPath)
	r.Artifacts.ManifestJSON = strings.TrimSpace(source.ManifestPath)
	if source.DBBackup.Path != "" {
		r.Artifacts.DBBackup = source.DBBackup.Path
	}
	if source.FilesBackup.Path != "" {
		r.Artifacts.FilesBackup = source.FilesBackup.Path
	}
}

func (r *MigrateResult) ApplySnapshot(snapshot BackupResult) {
	r.Artifacts.SnapshotManifestTXT = snapshot.Artifacts.ManifestText
	r.Artifacts.SnapshotManifestJSON = snapshot.Artifacts.ManifestJSON
	r.Artifacts.SnapshotDBBackup = snapshot.Artifacts.DBBackup
	r.Artifacts.SnapshotFilesBackup = snapshot.Artifacts.FilesBackup
	r.Artifacts.SnapshotDBChecksum = snapshot.Artifacts.DBChecksum
	r.Artifacts.SnapshotFilesChecksum = snapshot.Artifacts.FilesChecksum
}

func (r *MigrateResult) Succeed() {
	r.OK = true
	r.ProcessExitCode = ExitOK
	r.Error = nil
	r.recount()
}

func (r *MigrateResult) Fail(f BackupFailure) {
	r.OK = false
	r.ProcessExitCode = f.BackupError.ExitCode
	errCopy := f.BackupError
	r.Error = &errCopy
	r.recount()
}

func (r *MigrateResult) recount() {
	var planned, completed, skipped, blocked, failed int
	for _, step := range r.Items {
		switch step.Status {
		case StatusPlanned:
			planned++
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
	r.Details.Planned = planned
	r.Details.Completed = completed
	r.Details.Skipped = skipped
	r.Details.Blocked = blocked
	r.Details.Failed = failed
	r.Details.Warnings = len(r.Warnings)
	r.Details.Ready = failed == 0 && blocked == 0 && len(r.Items) > 0
}

func NewMigrateFailure(kind ErrorKind, message string, cause error) BackupFailure {
	return NewCommandFailure(kind, MigrateFailedCode, message, cause)
}

func MigrateCompatibilityMismatches(source, target MigrateCompatibilitySettings) []MigrateCompatibilityMismatch {
	type field struct {
		name   string
		source string
		target string
	}
	fields := []field{
		{name: "ESPOCRM_IMAGE", source: source.EspoCRMImage, target: target.EspoCRMImage},
		{name: "MARIADB_TAG", source: source.MariaDBTag, target: target.MariaDBTag},
		{name: "ESPO_DEFAULT_LANGUAGE", source: source.DefaultLanguage, target: target.DefaultLanguage},
		{name: "ESPO_TIME_ZONE", source: source.TimeZone, target: target.TimeZone},
	}

	mismatches := make([]MigrateCompatibilityMismatch, 0, len(fields))
	for _, current := range fields {
		if strings.TrimSpace(current.source) == strings.TrimSpace(current.target) {
			continue
		}
		mismatches = append(mismatches, MigrateCompatibilityMismatch{
			Name:        current.name,
			SourceValue: strings.TrimSpace(current.source),
			TargetValue: strings.TrimSpace(current.target),
		})
	}
	return mismatches
}

func ValidateMigrateDirectPair(dbPath, filesPath string) error {
	dbGroup, dbOK := ParseBackupGroupName(filepath.Base(dbPath))
	filesGroup, filesOK := ParseBackupGroupName(filepath.Base(filesPath))
	if !dbOK {
		return BackupVerifyArtifactError{Label: "db backup", Err: fmt.Errorf("имя DB artifact не canonical")}
	}
	if !filesOK {
		return BackupVerifyArtifactError{Label: "files backup", Err: fmt.Errorf("имя files artifact не canonical")}
	}
	if dbGroup != filesGroup {
		return BackupVerifyArtifactError{Label: "direct pair", Err: fmt.Errorf("DB и files artifacts относятся к разным backup-set")}
	}
	return nil
}

func matchingMigrateManifestTXT(path string) string {
	if !strings.HasSuffix(path, ".manifest.json") {
		return ""
	}
	return strings.TrimSuffix(path, ".manifest.json") + ".manifest.txt"
}
