package result

type SectionItem struct {
	Code    string `json:"code"`
	Status  string `json:"status"`
	Summary string `json:"summary"`
	Details string `json:"details,omitempty"`
	Action  string `json:"action,omitempty"`
}

type BackupVerifyDetails struct {
	Scope     string `json:"scope"`
	CreatedAt string `json:"created_at"`
}

type BackupDetails struct {
	Scope                  string `json:"scope"`
	CreatedAt              string `json:"created_at"`
	SkipDB                 bool   `json:"skip_db"`
	SkipFiles              bool   `json:"skip_files"`
	NoStop                 bool   `json:"no_stop"`
	ConsistentSnapshot     bool   `json:"consistent_snapshot"`
	AppServicesWereRunning bool   `json:"app_services_were_running"`
	RetentionDays          int    `json:"retention_days"`
}

type BackupVerifyArtifacts struct {
	Manifest    string `json:"manifest"`
	DBBackup    string `json:"db_backup,omitempty"`
	FilesBackup string `json:"files_backup,omitempty"`
}

type BackupArtifacts struct {
	ManifestTXT   string `json:"manifest_txt"`
	ManifestJSON  string `json:"manifest_json"`
	DBBackup      string `json:"db_backup,omitempty"`
	FilesBackup   string `json:"files_backup,omitempty"`
	DBChecksum    string `json:"db_checksum,omitempty"`
	FilesChecksum string `json:"files_checksum,omitempty"`
}

type RestoreDetails struct {
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

type RestoreArtifacts struct {
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

type RestoreItem struct {
	SectionItem
}

type MigrateDetails struct {
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

type MigrateArtifacts struct {
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

type MigrateItem struct {
	SectionItem
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
