package backup

import (
	"fmt"
	"strings"
	"time"

	domainbackup "github.com/lazuale/espocrm-ops/internal/domain/backup"
	"github.com/lazuale/espocrm-ops/internal/platform/backupstore"
)

const (
	AuditStatusOK      = domainbackup.AuditStatusOK
	AuditStatusWarn    = domainbackup.AuditStatusWarn
	AuditStatusFail    = domainbackup.AuditStatusFail
	AuditStatusSkipped = domainbackup.AuditStatusSkipped
)

type AuditRequest struct {
	BackupRoot       string
	SkipDB           bool
	SkipFiles        bool
	VerifyChecksum   bool
	DBMaxAgeHours    int
	FilesMaxAgeHours int
	Now              time.Time
}

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

func Audit(req AuditRequest) (AuditInfo, error) {
	backupRoot := strings.TrimSpace(req.BackupRoot)
	if backupRoot == "" {
		return AuditInfo{}, fmt.Errorf("backup root is required")
	}
	if req.SkipDB && req.SkipFiles {
		return AuditInfo{}, fmt.Errorf("nothing to audit: both db and files are skipped")
	}
	if req.DBMaxAgeHours < 0 || req.FilesMaxAgeHours < 0 {
		return AuditInfo{}, fmt.Errorf("max age hours must be non-negative")
	}
	now := req.Now
	if now.IsZero() {
		now = time.Now()
	}

	manifestMaxAgeHours := domainbackup.ManifestMaxAgeHours(req.SkipDB, req.SkipFiles, req.DBMaxAgeHours, req.FilesMaxAgeHours)

	info := AuditInfo{
		BackupRoot:     backupRoot,
		Success:        true,
		VerifyChecksum: req.VerifyChecksum,
		Thresholds: AuditThresholds{
			DBMaxAgeHours:       req.DBMaxAgeHours,
			FilesMaxAgeHours:    req.FilesMaxAgeHours,
			ManifestMaxAgeHours: manifestMaxAgeHours,
		},
		DiscoveredStamp: now.UTC().Format(time.RFC3339),
	}

	group, ok, err := auditSelectedGroup(backupRoot, req.SkipDB, req.SkipFiles)
	if err != nil {
		return AuditInfo{}, err
	}
	var set domainbackup.BackupSet
	if ok {
		info.SelectedSet = AuditSelectedSet{
			Prefix: group.Prefix,
			Stamp:  group.Stamp,
		}
		set = domainbackup.BuildBackupSet(backupRoot, group.Prefix, group.Stamp)
	}

	if req.SkipDB {
		info.DBBackup = AuditComponent(domainbackup.SkippedAuditComponent("Check skipped because of --skip-db", req.DBMaxAgeHours))
	} else {
		info.DBBackup = auditBackupComponent(set.DBBackup.Path, "db backup", req.DBMaxAgeHours, req.VerifyChecksum, now)
	}
	if req.SkipFiles {
		info.FilesBackup = AuditComponent(domainbackup.SkippedAuditComponent("Check skipped because of --skip-files", req.FilesMaxAgeHours))
	} else {
		info.FilesBackup = auditBackupComponent(set.FilesBackup.Path, "files backup", req.FilesMaxAgeHours, req.VerifyChecksum, now)
	}
	info.ManifestJSON = auditManifestComponent(set.ManifestJSON.Path, "manifest json", manifestMaxAgeHours, now, validateManifestJSONForAudit)
	info.ManifestTXT = auditManifestComponent(set.ManifestTXT.Path, "manifest txt", manifestMaxAgeHours, now, validateManifestTXTForAudit)

	info.Findings = auditFindingsFromDomain(domainbackup.AuditFindings(domainAuditInfo(info)))
	info.Success = domainbackup.FailedAuditFindingCount(domainAuditFindings(info.Findings)) == 0

	return info, nil
}

func auditSelectedGroup(backupRoot string, skipDB, skipFiles bool) (domainbackup.BackupGroup, bool, error) {
	mode := backupstore.GroupModeAny
	if skipDB {
		mode = backupstore.GroupModeFiles
	} else if skipFiles {
		mode = backupstore.GroupModeDB
	}

	groups, err := backupstore.Groups(backupRoot, mode)
	if err != nil {
		return domainbackup.BackupGroup{}, false, err
	}
	if len(groups) == 0 {
		return domainbackup.BackupGroup{}, false, nil
	}

	return groups[0], true, nil
}

func auditBackupComponent(path, label string, maxAgeHours int, verifyChecksum bool, now time.Time) AuditComponent {
	component := AuditComponent{
		Status:      AuditStatusOK,
		Message:     "latest " + label + " is valid and fresh",
		MaxAgeHours: maxAgeHours,
	}

	inspection, err := backupstore.InspectBackupArtifact(path, label, verifyChecksum)
	if err != nil || !inspection.FileInfo.Exists {
		component.Status = AuditStatusFail
		component.Message = "Expected file from the selected backup set was not found"
		return component
	}
	if inspection.FileInfo.IsDir {
		component.Status = AuditStatusFail
		component.Message = "Expected a backup file, found a directory"
		component.File = path
		return component
	}

	age := domainbackup.AgeHours(now, inspection.FileInfo.ModTime)
	component.File = path
	component.Sidecar = inspection.SidecarPath
	component.AgeHours = &age

	if !inspection.SidecarInfo.Exists {
		component.Status = AuditStatusFail
		component.Message = "Checksum sidecar file was not found next to the backup"
		component.Sidecar = ""
	}
	if age > maxAgeHours {
		component.Status = AuditStatusFail
		component.Message = domainbackup.AppendAuditMessage(component.Message, fmt.Sprintf("backup is older than the threshold: %dh > %dh", age, maxAgeHours))
	}

	if component.Sidecar != "" && inspection.ChecksumError != nil {
		component.Status = AuditStatusFail
		component.Message = domainbackup.AppendAuditMessage(component.Message, inspection.ChecksumError.Error())
	} else if component.Sidecar != "" && inspection.ChecksumVerified {
		component.ChecksumVerified = true
	} else if component.Sidecar != "" && !verifyChecksum {
		component.Status = domainbackup.MaxAuditStatus(component.Status, AuditStatusWarn)
		component.Message = domainbackup.AppendAuditMessage(component.Message, "Checksum verification skipped because of --no-verify-checksum")
	}

	return component
}

func auditManifestComponent(path, label string, maxAgeHours int, now time.Time, validate func(string) error) AuditComponent {
	component := AuditComponent{
		Status:      AuditStatusOK,
		Message:     "latest " + label + " is valid and fresh",
		MaxAgeHours: maxAgeHours,
	}

	fileInfo, err := backupstore.InspectFile(path)
	if err != nil || !fileInfo.Exists {
		component.Status = AuditStatusFail
		component.Message = "Expected manifest from the selected backup set was not found"
		return component
	}
	if fileInfo.IsDir {
		component.Status = AuditStatusFail
		component.Message = "Expected a manifest file, found a directory"
		component.File = path
		return component
	}

	age := domainbackup.AgeHours(now, fileInfo.ModTime)
	component.File = path
	component.AgeHours = &age
	if age > maxAgeHours {
		component.Status = AuditStatusFail
		component.Message = fmt.Sprintf("manifest is older than the threshold: %dh > %dh", age, maxAgeHours)
	}
	if err := validate(path); err != nil {
		component.Status = AuditStatusFail
		component.Message = domainbackup.AppendAuditMessage(component.Message, "manifest is invalid: "+err.Error())
	}

	return component
}

func validateManifestJSONForAudit(path string) error {
	_, err := LoadManifest(path)
	return err
}

func validateManifestTXTForAudit(path string) error {
	return backupstore.ValidateTXTManifest(path)
}

func domainAuditInfo(info AuditInfo) domainbackup.AuditInfo {
	return domainbackup.AuditInfo{
		BackupRoot:      info.BackupRoot,
		Success:         info.Success,
		VerifyChecksum:  info.VerifyChecksum,
		SelectedSet:     domainbackup.AuditSelectedSet(info.SelectedSet),
		Thresholds:      domainbackup.AuditThresholds(info.Thresholds),
		DBBackup:        domainbackup.AuditComponent(info.DBBackup),
		FilesBackup:     domainbackup.AuditComponent(info.FilesBackup),
		ManifestJSON:    domainbackup.AuditComponent(info.ManifestJSON),
		ManifestTXT:     domainbackup.AuditComponent(info.ManifestTXT),
		Findings:        domainAuditFindings(info.Findings),
		DiscoveredStamp: info.DiscoveredStamp,
	}
}

func auditFindingsFromDomain(findings []domainbackup.AuditFinding) []AuditFinding {
	out := make([]AuditFinding, 0, len(findings))
	for _, finding := range findings {
		out = append(out, AuditFinding(finding))
	}

	return out
}

func domainAuditFindings(findings []AuditFinding) []domainbackup.AuditFinding {
	out := make([]domainbackup.AuditFinding, 0, len(findings))
	for _, finding := range findings {
		out = append(out, domainbackup.AuditFinding(finding))
	}

	return out
}
