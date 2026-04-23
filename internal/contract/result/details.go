package result

type SectionItem struct {
	Code    string `json:"code"`
	Status  string `json:"status"`
	Summary string `json:"summary"`
	Details string `json:"details,omitempty"`
	Action  string `json:"action,omitempty"`
}

type BackupVerifyDetails struct {
	Ready      bool   `json:"ready"`
	SourceKind string `json:"source_kind,omitempty"`
	Scope      string `json:"scope,omitempty"`
	CreatedAt  string `json:"created_at,omitempty"`
	Steps      int    `json:"steps"`
	Completed  int    `json:"completed,omitempty"`
	Skipped    int    `json:"skipped,omitempty"`
	Blocked    int    `json:"blocked,omitempty"`
	Failed     int    `json:"failed,omitempty"`
}

func (BackupVerifyDetails) isResultDetails() {}

type BackupDetails struct {
	Scope                  string `json:"scope"`
	Ready                  bool   `json:"ready"`
	CreatedAt              string `json:"created_at"`
	Steps                  int    `json:"steps,omitempty"`
	Completed              int    `json:"completed,omitempty"`
	Skipped                int    `json:"skipped,omitempty"`
	Blocked                int    `json:"blocked,omitempty"`
	Failed                 int    `json:"failed,omitempty"`
	Warnings               int    `json:"warnings,omitempty"`
	SkipDB                 bool   `json:"skip_db"`
	SkipFiles              bool   `json:"skip_files"`
	NoStop                 bool   `json:"no_stop"`
	ConsistentSnapshot     bool   `json:"consistent_snapshot"`
	AppServicesWereRunning bool   `json:"app_services_were_running"`
	RetentionDays          int    `json:"retention_days"`
}

func (BackupDetails) isResultDetails() {}

type BackupItem struct {
	SectionItem
}

func (BackupItem) isResultItem() {}

type BackupVerifyArtifacts struct {
	BackupRoot    string `json:"backup_root,omitempty"`
	Manifest      string `json:"manifest,omitempty"`
	DBBackup      string `json:"db_backup,omitempty"`
	DBChecksum    string `json:"db_checksum,omitempty"`
	FilesBackup   string `json:"files_backup,omitempty"`
	FilesChecksum string `json:"files_checksum,omitempty"`
}

func (BackupVerifyArtifacts) isResultArtifacts() {}

type BackupVerifyItem struct {
	SectionItem
}

func (BackupVerifyItem) isResultItem() {}

type BackupArtifacts struct {
	ProjectDir    string `json:"project_dir,omitempty"`
	ComposeFile   string `json:"compose_file,omitempty"`
	EnvFile       string `json:"env_file,omitempty"`
	BackupRoot    string `json:"backup_root,omitempty"`
	ManifestTXT   string `json:"manifest_txt"`
	ManifestJSON  string `json:"manifest_json"`
	DBBackup      string `json:"db_backup,omitempty"`
	FilesBackup   string `json:"files_backup,omitempty"`
	DBChecksum    string `json:"db_checksum,omitempty"`
	FilesChecksum string `json:"files_checksum,omitempty"`
}

func (BackupArtifacts) isResultArtifacts() {}

type RestoreDetails struct {
	Scope                  string `json:"scope"`
	Ready                  bool   `json:"ready"`
	SelectionMode          string `json:"selection_mode"`
	SourceKind             string `json:"source_kind"`
	Steps                  int    `json:"steps"`
	Planned                int    `json:"planned,omitempty"`
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

func (RestoreDetails) isResultDetails() {}

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

func (RestoreArtifacts) isResultArtifacts() {}

type RestoreItem struct {
	SectionItem
}

func (RestoreItem) isResultItem() {}

type MigrateDetails struct {
	SourceScope string `json:"source_scope"`
	TargetScope string `json:"target_scope"`
	Ready       bool   `json:"ready"`
	// SourceKind is the canonical source truth surface. SelectionMode is only a
	// bounded diagnostic subtype for the retained migrate source policy.
	SelectionMode          string `json:"selection_mode,omitempty"`
	SourceKind             string `json:"source_kind,omitempty"`
	SnapshotEnabled        bool   `json:"snapshot_enabled"`
	Steps                  int    `json:"steps"`
	Planned                int    `json:"planned,omitempty"`
	Completed              int    `json:"completed"`
	Skipped                int    `json:"skipped"`
	Blocked                int    `json:"blocked"`
	Failed                 int    `json:"failed"`
	Warnings               int    `json:"warnings"`
	SkipDB                 bool   `json:"skip_db"`
	SkipFiles              bool   `json:"skip_files"`
	NoStart                bool   `json:"no_start"`
	AppServicesWereRunning bool   `json:"app_services_were_running"`
	StartedDBTemporarily   bool   `json:"started_db_temporarily"`
}

func (MigrateDetails) isResultDetails() {}

type MigrateArtifacts struct {
	ProjectDir            string `json:"project_dir"`
	ComposeFile           string `json:"compose_file"`
	SourceEnvFile         string `json:"source_env_file"`
	TargetEnvFile         string `json:"target_env_file"`
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

func (MigrateArtifacts) isResultArtifacts() {}

type MigrateItem struct {
	SectionItem
}

func (MigrateItem) isResultItem() {}

type DoctorDetails struct {
	TargetScope string `json:"target_scope"`
	Ready       bool   `json:"ready"`
	Checks      int    `json:"checks"`
	Passed      int    `json:"passed"`
	Warnings    int    `json:"warnings"`
	Failed      int    `json:"failed"`
}

func (DoctorDetails) isResultDetails() {}

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

func (DoctorArtifacts) isResultArtifacts() {}

type DoctorCheck struct {
	Scope   string `json:"scope,omitempty"`
	Code    string `json:"code"`
	Status  string `json:"status"`
	Summary string `json:"summary"`
	Details string `json:"details,omitempty"`
	Action  string `json:"action,omitempty"`
}

func (DoctorCheck) isResultItem() {}
