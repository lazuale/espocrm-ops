package result

type JournalReadDetails struct {
	TotalFilesSeen int `json:"total_files_seen"`
	LoadedEntries  int `json:"loaded_entries"`
	SkippedCorrupt int `json:"skipped_corrupt"`
}

type HistoryDetails struct {
	JournalReadDetails
	Limit        int    `json:"limit"`
	Command      string `json:"command"`
	OKOnly       bool   `json:"ok_only"`
	FailedOnly   bool   `json:"failed_only"`
	Status       string `json:"status,omitempty"`
	Scope        string `json:"scope,omitempty"`
	RecoveryOnly bool   `json:"recovery_only"`
	TargetPrefix string `json:"target_prefix,omitempty"`
	Returned     int    `json:"returned"`
	RecentFirst  bool   `json:"recent_first"`
	Since        string `json:"since,omitempty"`
	Until        string `json:"until,omitempty"`
}

type OperationLookupDetails struct {
	JournalReadDetails
	ID      string `json:"id,omitempty"`
	Command string `json:"command,omitempty"`
}

type PruneDetails struct {
	JournalReadDetails
	Checked           int    `json:"checked"`
	Retained          int    `json:"retained"`
	Protected         int    `json:"protected"`
	Deleted           int    `json:"deleted"`
	RemovedDirs       int    `json:"removed_dirs"`
	KeepDays          int    `json:"keep_days"`
	KeepLast          int    `json:"keep_last"`
	LatestOperationID string `json:"latest_operation_id,omitempty"`
	DryRun            bool   `json:"dry_run"`
}

type VerifyBackupDetails struct {
	Scope     string `json:"scope"`
	CreatedAt string `json:"created_at"`
}

type BackupDetails struct {
	Scope     string `json:"scope"`
	CreatedAt string `json:"created_at"`
	Sidecars  bool   `json:"sidecars"`
}

type BackupExecuteDetails struct {
	Scope                  string `json:"scope"`
	CreatedAt              string `json:"created_at"`
	SkipDB                 bool   `json:"skip_db"`
	SkipFiles              bool   `json:"skip_files"`
	NoStop                 bool   `json:"no_stop"`
	ConsistentSnapshot     bool   `json:"consistent_snapshot"`
	AppServicesWereRunning bool   `json:"app_services_were_running"`
	RetentionDays          int    `json:"retention_days"`
}

type BackupCatalogDetails struct {
	BackupRoot     string `json:"backup_root"`
	VerifyChecksum bool   `json:"verify_checksum"`
	ReadyOnly      bool   `json:"ready_only"`
	Limit          int    `json:"limit"`
	TotalSets      int    `json:"total_sets"`
	ShownSets      int    `json:"shown_sets"`
}

type BackupAuditDetails struct {
	BackupRoot          string `json:"backup_root"`
	Success             bool   `json:"success"`
	VerifyChecksum      bool   `json:"verify_checksum"`
	SelectedPrefix      string `json:"selected_prefix,omitempty"`
	SelectedStamp       string `json:"selected_stamp,omitempty"`
	DBMaxAgeHours       int    `json:"db_max_age_hours"`
	FilesMaxAgeHours    int    `json:"files_max_age_hours"`
	ManifestMaxAgeHours int    `json:"manifest_max_age_hours"`
	FailureFindings     int    `json:"failure_findings"`
	NonFailureFindings  int    `json:"non_failure_findings"`
}

type RestorePlanCheck struct {
	Name    string `json:"name"`
	Status  string `json:"status"`
	Details string `json:"details,omitempty"`
}

type RestorePlanDetails struct {
	SourceKind  string             `json:"source_kind"`
	SourcePath  string             `json:"source_path"`
	Destructive bool               `json:"destructive"`
	Changes     []string           `json:"changes,omitempty"`
	NonChanges  []string           `json:"non_changes,omitempty"`
	Checks      []RestorePlanCheck `json:"checks,omitempty"`
	NextStep    string             `json:"next_step,omitempty"`
}

type RestoreFilesDetails struct {
	DryRun bool               `json:"dry_run"`
	Plan   RestorePlanDetails `json:"plan"`
}

type RestoreDBDetails struct {
	DryRun bool               `json:"dry_run"`
	DBUser string             `json:"db_user"`
	Plan   RestorePlanDetails `json:"plan"`
}

type VerifyBackupArtifacts struct {
	Manifest    string `json:"manifest"`
	DBBackup    string `json:"db_backup,omitempty"`
	FilesBackup string `json:"files_backup,omitempty"`
}

type BackupArtifacts struct {
	Manifest      string `json:"manifest"`
	DBBackup      string `json:"db_backup"`
	FilesBackup   string `json:"files_backup"`
	DBChecksum    string `json:"db_checksum"`
	FilesChecksum string `json:"files_checksum"`
}

type BackupExecuteArtifacts struct {
	ManifestTXT   string `json:"manifest_txt"`
	ManifestJSON  string `json:"manifest_json"`
	DBBackup      string `json:"db_backup,omitempty"`
	FilesBackup   string `json:"files_backup,omitempty"`
	DBChecksum    string `json:"db_checksum,omitempty"`
	FilesChecksum string `json:"files_checksum,omitempty"`
}

type RestoreFilesArtifacts struct {
	Manifest  string `json:"manifest"`
	Files     string `json:"files_backup,omitempty"`
	TargetDir string `json:"target_dir"`
}

type RestoreDBArtifacts struct {
	Manifest    string `json:"manifest"`
	DBBackup    string `json:"db_backup,omitempty"`
	DBContainer string `json:"db_container"`
	DBName      string `json:"db_name"`
}

type UpdateBackupDetails struct {
	TimeoutSeconds         int    `json:"timeout_seconds"`
	StartedDBTemporarily   bool   `json:"started_db_temporarily"`
	CreatedAt              string `json:"created_at"`
	ConsistentSnapshot     bool   `json:"consistent_snapshot"`
	AppServicesWereRunning bool   `json:"app_services_were_running"`
}

type UpdateBackupArtifacts struct {
	Scope         string `json:"scope"`
	ManifestTXT   string `json:"manifest_txt"`
	ManifestJSON  string `json:"manifest_json"`
	DBBackup      string `json:"db_backup,omitempty"`
	FilesBackup   string `json:"files_backup,omitempty"`
	DBChecksum    string `json:"db_checksum,omitempty"`
	FilesChecksum string `json:"files_checksum,omitempty"`
}

type UpdateRuntimeDetails struct {
	TimeoutSeconds int      `json:"timeout_seconds"`
	SkipPull       bool     `json:"skip_pull"`
	SkipHTTPProbe  bool     `json:"skip_http_probe"`
	ServicesReady  []string `json:"services_ready,omitempty"`
}

type UpdateRuntimeArtifacts struct {
	ProjectDir  string `json:"project_dir"`
	ComposeFile string `json:"compose_file"`
	EnvFile     string `json:"env_file"`
	SiteURL     string `json:"site_url,omitempty"`
}

type UpdatePlanDetails struct {
	Scope          string           `json:"scope"`
	Ready          bool             `json:"ready"`
	Steps          int              `json:"steps"`
	WouldRun       int              `json:"would_run"`
	Skipped        int              `json:"skipped"`
	Blocked        int              `json:"blocked"`
	Unknown        int              `json:"unknown"`
	Warnings       int              `json:"warnings"`
	TimeoutSeconds int              `json:"timeout_seconds"`
	SkipDoctor     bool             `json:"skip_doctor"`
	SkipBackup     bool             `json:"skip_backup"`
	SkipPull       bool             `json:"skip_pull"`
	SkipHTTPProbe  bool             `json:"skip_http_probe"`
	Recovery       *RecoveryDetails `json:"recovery,omitempty"`
}

type UpdatePlanArtifacts struct {
	ProjectDir     string `json:"project_dir"`
	ComposeFile    string `json:"compose_file"`
	EnvFile        string `json:"env_file"`
	ComposeProject string `json:"compose_project,omitempty"`
	BackupRoot     string `json:"backup_root,omitempty"`
	SiteURL        string `json:"site_url,omitempty"`
}

type UpdatePlanItem struct {
	Code    string `json:"code"`
	Status  string `json:"status"`
	Summary string `json:"summary"`
	Details string `json:"details,omitempty"`
	Action  string `json:"action,omitempty"`
}

type UpdateDetails struct {
	Scope          string           `json:"scope"`
	Ready          bool             `json:"ready"`
	Steps          int              `json:"steps"`
	Completed      int              `json:"completed"`
	Skipped        int              `json:"skipped"`
	Failed         int              `json:"failed"`
	NotRun         int              `json:"not_run"`
	TimeoutSeconds int              `json:"timeout_seconds"`
	SkipDoctor     bool             `json:"skip_doctor"`
	SkipBackup     bool             `json:"skip_backup"`
	SkipPull       bool             `json:"skip_pull"`
	SkipHTTPProbe  bool             `json:"skip_http_probe"`
	Recovery       *RecoveryDetails `json:"recovery,omitempty"`
}

type UpdateArtifacts struct {
	ProjectDir     string `json:"project_dir"`
	ComposeFile    string `json:"compose_file"`
	EnvFile        string `json:"env_file"`
	ComposeProject string `json:"compose_project,omitempty"`
	BackupRoot     string `json:"backup_root,omitempty"`
	SiteURL        string `json:"site_url,omitempty"`
	ManifestTXT    string `json:"manifest_txt,omitempty"`
	ManifestJSON   string `json:"manifest_json,omitempty"`
	DBBackup       string `json:"db_backup,omitempty"`
	FilesBackup    string `json:"files_backup,omitempty"`
	DBChecksum     string `json:"db_checksum,omitempty"`
	FilesChecksum  string `json:"files_checksum,omitempty"`
}

type UpdateItem struct {
	Code    string `json:"code"`
	Status  string `json:"status"`
	Summary string `json:"summary"`
	Details string `json:"details,omitempty"`
	Action  string `json:"action,omitempty"`
}

type RollbackDetails struct {
	Scope                  string           `json:"scope"`
	Ready                  bool             `json:"ready"`
	SelectionMode          string           `json:"selection_mode,omitempty"`
	RequestedSelectionMode string           `json:"requested_selection_mode,omitempty"`
	Steps                  int              `json:"steps"`
	Completed              int              `json:"completed"`
	Skipped                int              `json:"skipped"`
	Failed                 int              `json:"failed"`
	NotRun                 int              `json:"not_run"`
	Warnings               int              `json:"warnings"`
	TimeoutSeconds         int              `json:"timeout_seconds"`
	SnapshotEnabled        bool             `json:"snapshot_enabled"`
	NoStart                bool             `json:"no_start"`
	SkipHTTPProbe          bool             `json:"skip_http_probe"`
	StartedDBTemporarily   bool             `json:"started_db_temporarily"`
	ServicesReady          []string         `json:"services_ready,omitempty"`
	Recovery               *RecoveryDetails `json:"recovery,omitempty"`
}

type RollbackArtifacts struct {
	ProjectDir            string `json:"project_dir"`
	ComposeFile           string `json:"compose_file"`
	EnvFile               string `json:"env_file"`
	ComposeProject        string `json:"compose_project,omitempty"`
	BackupRoot            string `json:"backup_root,omitempty"`
	SiteURL               string `json:"site_url,omitempty"`
	RequestedDBBackup     string `json:"requested_db_backup,omitempty"`
	RequestedFilesBackup  string `json:"requested_files_backup,omitempty"`
	SelectedPrefix        string `json:"selected_prefix,omitempty"`
	SelectedStamp         string `json:"selected_stamp,omitempty"`
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

type RollbackItem struct {
	Code    string `json:"code"`
	Status  string `json:"status"`
	Summary string `json:"summary"`
	Details string `json:"details,omitempty"`
	Action  string `json:"action,omitempty"`
}

type RollbackPlanDetails struct {
	Scope           string           `json:"scope"`
	Ready           bool             `json:"ready"`
	SelectionMode   string           `json:"selection_mode,omitempty"`
	Steps           int              `json:"steps"`
	WouldRun        int              `json:"would_run"`
	Skipped         int              `json:"skipped"`
	Blocked         int              `json:"blocked"`
	Unknown         int              `json:"unknown"`
	Warnings        int              `json:"warnings"`
	TimeoutSeconds  int              `json:"timeout_seconds"`
	SnapshotEnabled bool             `json:"snapshot_enabled"`
	NoStart         bool             `json:"no_start"`
	SkipHTTPProbe   bool             `json:"skip_http_probe"`
	Recovery        *RecoveryDetails `json:"recovery,omitempty"`
}

type RecoveryDetails struct {
	SourceOperationID string `json:"source_operation_id"`
	RequestedMode     string `json:"requested_mode"`
	AppliedMode       string `json:"applied_mode"`
	Decision          string `json:"decision"`
	ResumeStep        string `json:"resume_step,omitempty"`
}

type RollbackPlanArtifacts struct {
	ProjectDir     string `json:"project_dir"`
	ComposeFile    string `json:"compose_file"`
	EnvFile        string `json:"env_file"`
	ComposeProject string `json:"compose_project,omitempty"`
	BackupRoot     string `json:"backup_root,omitempty"`
	SiteURL        string `json:"site_url,omitempty"`
	SelectedPrefix string `json:"selected_prefix,omitempty"`
	SelectedStamp  string `json:"selected_stamp,omitempty"`
	ManifestTXT    string `json:"manifest_txt,omitempty"`
	ManifestJSON   string `json:"manifest_json,omitempty"`
	DBBackup       string `json:"db_backup,omitempty"`
	FilesBackup    string `json:"files_backup,omitempty"`
}

type RollbackPlanItem struct {
	Code    string `json:"code"`
	Status  string `json:"status"`
	Summary string `json:"summary"`
	Details string `json:"details,omitempty"`
	Action  string `json:"action,omitempty"`
}

type DoctorDetails struct {
	TargetScope string `json:"target_scope"`
	Ready       bool   `json:"ready"`
	Checks      int    `json:"checks"`
	Passed      int    `json:"passed"`
	Warnings    int    `json:"warnings"`
	Failed      int    `json:"failed"`
}

type DoctorScopeArtifact struct {
	Scope      string `json:"scope"`
	EnvFile    string `json:"env_file,omitempty"`
	BackupRoot string `json:"backup_root,omitempty"`
}

type DoctorArtifacts struct {
	ProjectDir  string                `json:"project_dir"`
	ComposeFile string                `json:"compose_file"`
	Scopes      []DoctorScopeArtifact `json:"scopes,omitempty"`
}

type DoctorCheck struct {
	Scope   string `json:"scope,omitempty"`
	Code    string `json:"code"`
	Status  string `json:"status"`
	Summary string `json:"summary"`
	Details string `json:"details,omitempty"`
	Action  string `json:"action,omitempty"`
}

type RunOperationDetails struct {
	Scope          string   `json:"scope"`
	Operation      string   `json:"operation"`
	Command        []string `json:"command"`
	ComposeProject string   `json:"compose_project,omitempty"`
}

type RunOperationArtifacts struct {
	ProjectDir string `json:"project_dir"`
	EnvFile    string `json:"env_file,omitempty"`
	BackupRoot string `json:"backup_root,omitempty"`
}
