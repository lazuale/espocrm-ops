package result

type JournalReadDetails struct {
	TotalFilesSeen int `json:"total_files_seen"`
	LoadedEntries  int `json:"loaded_entries"`
	SkippedCorrupt int `json:"skipped_corrupt"`
}

type HistoryDetails struct {
	JournalReadDetails
	Limit      int    `json:"limit"`
	Command    string `json:"command"`
	OKOnly     bool   `json:"ok_only"`
	FailedOnly bool   `json:"failed_only"`
	Since      string `json:"since,omitempty"`
	Until      string `json:"until,omitempty"`
}

type OperationLookupDetails struct {
	JournalReadDetails
	ID      string `json:"id,omitempty"`
	Command string `json:"command,omitempty"`
}

type PruneDetails struct {
	JournalReadDetails
	Checked     int  `json:"checked"`
	Deleted     int  `json:"deleted"`
	RemovedDirs int  `json:"removed_dirs"`
	KeepDays    int  `json:"keep_days"`
	Keep        int  `json:"keep"`
	DryRun      bool `json:"dry_run"`
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

type PruneItem struct {
	Type string `json:"type"`
	Path string `json:"path"`
}
