package backup

const (
	AuditStatusOK      = "ok"
	AuditStatusWarn    = "warn"
	AuditStatusFail    = "fail"
	AuditStatusSkipped = "skipped"
)

type AuditInfo struct {
	BackupRoot      string           `json:"backup_root"`
	Success         bool             `json:"success"`
	VerifyChecksum  bool             `json:"verify_checksum"`
	SelectedSet     AuditSelectedSet `json:"selected_set"`
	Thresholds      AuditThresholds  `json:"thresholds"`
	DBBackup        AuditComponent   `json:"db_backup"`
	FilesBackup     AuditComponent   `json:"files_backup"`
	ManifestJSON    AuditComponent   `json:"manifest_json"`
	ManifestTXT     AuditComponent   `json:"manifest_txt"`
	Findings        []AuditFinding   `json:"findings,omitempty"`
	DiscoveredStamp string           `json:"discovered_at,omitempty"`
}

type AuditSelectedSet struct {
	Prefix string `json:"prefix,omitempty"`
	Stamp  string `json:"stamp,omitempty"`
}

type AuditThresholds struct {
	DBMaxAgeHours       int `json:"db_max_age_hours"`
	FilesMaxAgeHours    int `json:"files_max_age_hours"`
	ManifestMaxAgeHours int `json:"manifest_max_age_hours"`
}

type AuditComponent struct {
	Status           string `json:"status"`
	Message          string `json:"message,omitempty"`
	File             string `json:"file,omitempty"`
	Sidecar          string `json:"sidecar,omitempty"`
	AgeHours         *int   `json:"age_hours,omitempty"`
	MaxAgeHours      int    `json:"max_age_hours"`
	ChecksumVerified bool   `json:"checksum_verified,omitempty"`
}

type AuditFinding struct {
	Level   string `json:"level"`
	Subject string `json:"subject"`
	Message string `json:"message"`
}

func ManifestMaxAgeHours(skipDB, skipFiles bool, dbMaxAgeHours, filesMaxAgeHours int) int {
	switch {
	case skipDB:
		return filesMaxAgeHours
	case skipFiles:
		return dbMaxAgeHours
	default:
		return maxInt(dbMaxAgeHours, filesMaxAgeHours)
	}
}

func SkippedAuditComponent(message string, maxAgeHours int) AuditComponent {
	return AuditComponent{
		Status:      AuditStatusSkipped,
		Message:     message,
		MaxAgeHours: maxAgeHours,
	}
}

func AuditFindings(info AuditInfo) []AuditFinding {
	components := []struct {
		subject   string
		component AuditComponent
	}{
		{subject: "db_backup", component: info.DBBackup},
		{subject: "files_backup", component: info.FilesBackup},
		{subject: "manifest_json", component: info.ManifestJSON},
		{subject: "manifest_txt", component: info.ManifestTXT},
	}

	findings := []AuditFinding{}
	for _, item := range components {
		switch item.component.Status {
		case AuditStatusFail, AuditStatusWarn:
			findings = append(findings, AuditFinding{
				Level:   item.component.Status,
				Subject: item.subject,
				Message: item.component.Message,
			})
		}
	}

	return findings
}

func FailedAuditFindingCount(findings []AuditFinding) int {
	count := 0
	for _, finding := range findings {
		if finding.Level == AuditStatusFail {
			count++
		}
	}

	return count
}

func AppendAuditMessage(base, extra string) string {
	if base == "" {
		return extra
	}
	if extra == "" {
		return base
	}

	return base + "; " + extra
}

func MaxAuditStatus(a, b string) string {
	if auditStatusRank(b) > auditStatusRank(a) {
		return b
	}

	return a
}

func auditStatusRank(status string) int {
	switch status {
	case AuditStatusFail:
		return 3
	case AuditStatusWarn:
		return 2
	case AuditStatusOK:
		return 1
	default:
		return 0
	}
}

func maxInt(a, b int) int {
	if a >= b {
		return a
	}

	return b
}
