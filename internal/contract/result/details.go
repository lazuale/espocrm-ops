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

type OperationExportDetails struct {
	JournalReadDetails
	ID               string   `json:"id"`
	BundleKind       string   `json:"bundle_kind"`
	BundleVersion    int      `json:"bundle_version"`
	ExportedAt       string   `json:"exported_at"`
	IncludedSections []string `json:"included_sections"`
	OmittedSections  []string `json:"omitted_sections,omitempty"`
}

type OperationExportArtifacts struct {
	BundlePath string `json:"bundle_path"`
}

type SupportBundleDetails struct {
	Scope            string   `json:"scope"`
	BundleKind       string   `json:"bundle_kind"`
	BundleVersion    int      `json:"bundle_version"`
	GeneratedAt      string   `json:"generated_at"`
	TailLines        int      `json:"tail_lines"`
	Sections         int      `json:"sections"`
	Included         int      `json:"included"`
	Omitted          int      `json:"omitted"`
	Warnings         int      `json:"warnings"`
	RetentionDays    int      `json:"retention_days"`
	IncludedSections []string `json:"included_sections"`
	OmittedSections  []string `json:"omitted_sections,omitempty"`
}

type SupportBundleArtifacts struct {
	ProjectDir  string `json:"project_dir"`
	ComposeFile string `json:"compose_file"`
	EnvFile     string `json:"env_file"`
	BackupRoot  string `json:"backup_root"`
	BundlePath  string `json:"bundle_path,omitempty"`
}

type SupportBundleItem struct {
	Code    string   `json:"code"`
	Status  string   `json:"status"`
	Summary string   `json:"summary"`
	Details string   `json:"details,omitempty"`
	Action  string   `json:"action,omitempty"`
	Files   []string `json:"files,omitempty"`
}

type OverviewDetails struct {
	Scope            string   `json:"scope"`
	GeneratedAt      string   `json:"generated_at"`
	Sections         int      `json:"sections"`
	Included         int      `json:"included"`
	Omitted          int      `json:"omitted"`
	Failed           int      `json:"failed"`
	Warnings         int      `json:"warnings"`
	IncludedSections []string `json:"included_sections"`
	OmittedSections  []string `json:"omitted_sections,omitempty"`
	FailedSections   []string `json:"failed_sections,omitempty"`
}

type OverviewArtifacts struct {
	ProjectDir  string `json:"project_dir"`
	ComposeFile string `json:"compose_file"`
	EnvFile     string `json:"env_file,omitempty"`
	BackupRoot  string `json:"backup_root,omitempty"`
}

type StatusReportDetails struct {
	Scope            string   `json:"scope"`
	GeneratedAt      string   `json:"generated_at"`
	Sections         int      `json:"sections"`
	Included         int      `json:"included"`
	Omitted          int      `json:"omitted"`
	Failed           int      `json:"failed"`
	Warnings         int      `json:"warnings"`
	IncludedSections []string `json:"included_sections"`
	OmittedSections  []string `json:"omitted_sections,omitempty"`
	FailedSections   []string `json:"failed_sections,omitempty"`
}

type StatusReportArtifacts struct {
	ProjectDir  string `json:"project_dir"`
	ComposeFile string `json:"compose_file"`
	EnvFile     string `json:"env_file,omitempty"`
	BackupRoot  string `json:"backup_root,omitempty"`
	ReportsDir  string `json:"reports_dir,omitempty"`
	SupportDir  string `json:"support_dir,omitempty"`
}

type MaintenanceDetails struct {
	Scope            string   `json:"scope"`
	GeneratedAt      string   `json:"generated_at"`
	Sections         int      `json:"sections"`
	Included         int      `json:"included"`
	Omitted          int      `json:"omitted"`
	Failed           int      `json:"failed"`
	Warnings         int      `json:"warnings"`
	DryRun           bool     `json:"dry_run"`
	IncludedSections []string `json:"included_sections"`
	OmittedSections  []string `json:"omitted_sections,omitempty"`
	FailedSections   []string `json:"failed_sections,omitempty"`
}

type MaintenanceArtifacts struct {
	ProjectDir             string `json:"project_dir"`
	ComposeFile            string `json:"compose_file"`
	EnvFile                string `json:"env_file,omitempty"`
	JournalDir             string `json:"journal_dir,omitempty"`
	BackupRoot             string `json:"backup_root,omitempty"`
	ReportsDir             string `json:"reports_dir,omitempty"`
	SupportDir             string `json:"support_dir,omitempty"`
	RestoreDrillEnvDir     string `json:"restore_drill_env_dir,omitempty"`
	RestoreDrillStorageDir string `json:"restore_drill_storage_dir,omitempty"`
	RestoreDrillBackupDir  string `json:"restore_drill_backup_dir,omitempty"`
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
	JournalReadDetails
	BackupRoot     string `json:"backup_root"`
	VerifyChecksum bool   `json:"verify_checksum"`
	ReadyOnly      bool   `json:"ready_only"`
	Limit          int    `json:"limit"`
	TotalSets      int    `json:"total_sets"`
	ShownSets      int    `json:"shown_sets"`
}

type BackupShowDetails struct {
	JournalReadDetails
	BackupRoot     string `json:"backup_root"`
	ID             string `json:"id"`
	VerifyChecksum bool   `json:"verify_checksum"`
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

type RestoreExecutionDetails struct {
	Scope                  string `json:"scope"`
	Ready                  bool   `json:"ready"`
	SelectionMode          string `json:"selection_mode"`
	SourceKind             string `json:"source_kind"`
	Steps                  int    `json:"steps"`
	WouldRun               int    `json:"would_run,omitempty"`
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
	StartedDBTemporarily   bool   `json:"started_db_temporarily"`
}

type RestoreExecutionArtifacts struct {
	ProjectDir            string `json:"project_dir"`
	ComposeFile           string `json:"compose_file"`
	EnvFile               string `json:"env_file"`
	BackupRoot            string `json:"backup_root"`
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

type RestoreExecutionItem struct {
	Code    string `json:"code"`
	Status  string `json:"status"`
	Summary string `json:"summary"`
	Details string `json:"details,omitempty"`
	Action  string `json:"action,omitempty"`
}

type RestoreDrillDetails struct {
	Scope                  string   `json:"scope"`
	Ready                  bool     `json:"ready"`
	RequestedSelectionMode string   `json:"requested_selection_mode,omitempty"`
	SelectionMode          string   `json:"selection_mode,omitempty"`
	Steps                  int      `json:"steps"`
	Completed              int      `json:"completed"`
	Failed                 int      `json:"failed"`
	NotRun                 int      `json:"not_run"`
	Warnings               int      `json:"warnings,omitempty"`
	TimeoutSeconds         int      `json:"timeout_seconds"`
	SkipHTTPProbe          bool     `json:"skip_http_probe"`
	KeepArtifacts          bool     `json:"keep_artifacts"`
	DrillAppPort           int      `json:"drill_app_port"`
	DrillWSPort            int      `json:"drill_ws_port"`
	ServicesReady          []string `json:"services_ready,omitempty"`
}

type RestoreDrillArtifacts struct {
	ProjectDir           string `json:"project_dir"`
	ComposeFile          string `json:"compose_file"`
	SourceEnvFile        string `json:"source_env_file"`
	SourceComposeProject string `json:"source_compose_project,omitempty"`
	SourceBackupRoot     string `json:"source_backup_root,omitempty"`
	SelectedPrefix       string `json:"selected_prefix,omitempty"`
	SelectedStamp        string `json:"selected_stamp,omitempty"`
	ManifestTXT          string `json:"manifest_txt,omitempty"`
	ManifestJSON         string `json:"manifest_json,omitempty"`
	DBBackup             string `json:"db_backup,omitempty"`
	FilesBackup          string `json:"files_backup,omitempty"`
	DrillEnvFile         string `json:"drill_env_file,omitempty"`
	DrillComposeProject  string `json:"drill_compose_project,omitempty"`
	DrillBackupRoot      string `json:"drill_backup_root,omitempty"`
	DrillDBStorage       string `json:"drill_db_storage,omitempty"`
	DrillESPOStorage     string `json:"drill_espo_storage,omitempty"`
	SiteURL              string `json:"site_url,omitempty"`
	WSPublicURL          string `json:"ws_public_url,omitempty"`
	ReportTXT            string `json:"report_txt,omitempty"`
	ReportJSON           string `json:"report_json,omitempty"`
}

type RestoreDrillItem struct {
	Code    string `json:"code"`
	Status  string `json:"status"`
	Summary string `json:"summary"`
	Details string `json:"details,omitempty"`
	Action  string `json:"action,omitempty"`
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

type MigrateBackupDetails struct {
	SourceScope            string `json:"source_scope"`
	TargetScope            string `json:"target_scope"`
	Ready                  bool   `json:"ready"`
	SelectionMode          string `json:"selection_mode,omitempty"`
	RequestedSelectionMode string `json:"requested_selection_mode,omitempty"`
	Steps                  int    `json:"steps"`
	Completed              int    `json:"completed"`
	Skipped                int    `json:"skipped"`
	Failed                 int    `json:"failed"`
	NotRun                 int    `json:"not_run"`
	Warnings               int    `json:"warnings"`
	SkipDB                 bool   `json:"skip_db"`
	SkipFiles              bool   `json:"skip_files"`
	NoStart                bool   `json:"no_start"`
	StartedDBTemporarily   bool   `json:"started_db_temporarily"`
}

type MigrateBackupArtifacts struct {
	ProjectDir           string `json:"project_dir"`
	ComposeFile          string `json:"compose_file"`
	SourceEnvFile        string `json:"source_env_file"`
	TargetEnvFile        string `json:"target_env_file"`
	SourceBackupRoot     string `json:"source_backup_root,omitempty"`
	TargetBackupRoot     string `json:"target_backup_root,omitempty"`
	RequestedDBBackup    string `json:"requested_db_backup,omitempty"`
	RequestedFilesBackup string `json:"requested_files_backup,omitempty"`
	SelectedPrefix       string `json:"selected_prefix,omitempty"`
	SelectedStamp        string `json:"selected_stamp,omitempty"`
	ManifestTXT          string `json:"manifest_txt,omitempty"`
	ManifestJSON         string `json:"manifest_json,omitempty"`
	DBBackup             string `json:"db_backup,omitempty"`
	FilesBackup          string `json:"files_backup,omitempty"`
}

type MigrateBackupItem struct {
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
