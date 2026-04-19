package result

type IncludedOmittedSections struct {
	IncludedSections []string `json:"included_sections"`
	OmittedSections  []string `json:"omitted_sections,omitempty"`
}

type IncludedOmittedSummary struct {
	Sections int `json:"sections"`
	Included int `json:"included"`
	Omitted  int `json:"omitted"`
	Warnings int `json:"warnings"`
	IncludedOmittedSections
}

type SectionSummary struct {
	IncludedOmittedSummary
	Failed         int      `json:"failed"`
	FailedSections []string `json:"failed_sections,omitempty"`
}

type SectionItem struct {
	Code    string `json:"code"`
	Status  string `json:"status"`
	Summary string `json:"summary"`
	Details string `json:"details,omitempty"`
	Action  string `json:"action,omitempty"`
}

func NewIncludedOmittedSections(included, omitted []string) IncludedOmittedSections {
	return IncludedOmittedSections{
		IncludedSections: cloneStrings(included),
		OmittedSections:  cloneStrings(omitted),
	}
}

func NewIncludedOmittedSummary(sections, warnings int, included, omitted []string) IncludedOmittedSummary {
	return IncludedOmittedSummary{
		Sections:                sections,
		Included:                len(included),
		Omitted:                 len(omitted),
		Warnings:                warnings,
		IncludedOmittedSections: NewIncludedOmittedSections(included, omitted),
	}
}

func NewSectionSummary(sections, warnings int, included, omitted, failed []string) SectionSummary {
	return SectionSummary{
		IncludedOmittedSummary: NewIncludedOmittedSummary(sections, warnings, included, omitted),
		Failed:                 len(failed),
		FailedSections:         cloneStrings(failed),
	}
}

type OperationExportDetails struct {
	JournalReadDetails
	ID            string `json:"id"`
	BundleKind    string `json:"bundle_kind"`
	BundleVersion int    `json:"bundle_version"`
	ExportedAt    string `json:"exported_at"`
	IncludedOmittedSections
}

type OperationExportArtifacts struct {
	BundlePath string `json:"bundle_path"`
}

type SupportBundleDetails struct {
	Scope         string `json:"scope"`
	BundleKind    string `json:"bundle_kind"`
	BundleVersion int    `json:"bundle_version"`
	GeneratedAt   string `json:"generated_at"`
	TailLines     int    `json:"tail_lines"`
	RetentionDays int    `json:"retention_days"`
	IncludedOmittedSummary
}

type SupportBundleArtifacts struct {
	ProjectDir  string `json:"project_dir"`
	ComposeFile string `json:"compose_file"`
	EnvFile     string `json:"env_file"`
	BackupRoot  string `json:"backup_root"`
	BundlePath  string `json:"bundle_path,omitempty"`
}

type SupportBundleItem struct {
	SectionItem
	Files []string `json:"files,omitempty"`
}

type OverviewDetails struct {
	Scope       string `json:"scope"`
	GeneratedAt string `json:"generated_at"`
	SectionSummary
}

type OverviewArtifacts struct {
	ProjectDir  string `json:"project_dir"`
	ComposeFile string `json:"compose_file"`
	EnvFile     string `json:"env_file,omitempty"`
	BackupRoot  string `json:"backup_root,omitempty"`
}

type StatusReportDetails struct {
	Scope       string `json:"scope"`
	GeneratedAt string `json:"generated_at"`
	SectionSummary
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
	Scope          string `json:"scope"`
	GeneratedAt    string `json:"generated_at"`
	Mode           string `json:"mode"`
	Unattended     bool   `json:"unattended"`
	Outcome        string `json:"outcome"`
	DryRun         bool   `json:"dry_run"`
	CheckedItems   int    `json:"checked_items"`
	CandidateItems int    `json:"candidate_items"`
	KeptItems      int    `json:"kept_items"`
	ProtectedItems int    `json:"protected_items"`
	RemovedItems   int    `json:"removed_items"`
	FailedItems    int    `json:"failed_items"`
	SectionSummary
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

func cloneStrings(values []string) []string {
	if values == nil {
		return nil
	}
	return append([]string(nil), values...)
}
