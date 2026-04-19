package healthsummary

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/lazuale/espocrm-ops/internal/contract/apperr"
	"github.com/lazuale/espocrm-ops/internal/platform/locks"
	backupusecase "github.com/lazuale/espocrm-ops/internal/usecase/backup"
	doctorusecase "github.com/lazuale/espocrm-ops/internal/usecase/doctor"
	journalusecase "github.com/lazuale/espocrm-ops/internal/usecase/journal"
	"github.com/lazuale/espocrm-ops/internal/usecase/reporting"
	statusreportusecase "github.com/lazuale/espocrm-ops/internal/usecase/statusreport"
)

const (
	VerdictHealthy  = "healthy"
	VerdictDegraded = "degraded"
	VerdictBlocked  = "blocked"
	VerdictFailed   = "failed"

	SectionStatusIncluded = "included"
	SectionStatusOmitted  = "omitted"
	SectionStatusFailed   = "failed"

	AlertSeverityWarning  = "warning"
	AlertSeverityBlocking = "blocking"
	AlertSeverityFailure  = "failure"
)

type Request struct {
	Scope           string
	ProjectDir      string
	ComposeFile     string
	EnvFileOverride string
	EnvContourHint  string
	JournalDir      string
	VerifyChecksum  bool
	MaxAgeHours     int
	Now             time.Time
}

type Info struct {
	Scope                string
	ProjectDir           string
	ComposeFile          string
	EnvFile              string
	BackupRoot           string
	ReportsDir           string
	SupportDir           string
	GeneratedAt          string
	Verdict              string
	NextAction           string
	DoctorState          string
	RuntimeState         string
	BackupState          string
	LatestOperationState string
	LatestOperationID    string
	LatestOperationCmd   string
	MaintenanceState     string
	IncludedSections     []string
	OmittedSections      []string
	FailedSections       []string
	Warnings             []string
	Sections             []Section
	Alerts               []Alert
}

type Section struct {
	Code          string `json:"code"`
	Status        string `json:"status"`
	State         string `json:"state"`
	SourceCommand string `json:"source_command"`
	Summary       string `json:"summary"`
	Details       string `json:"details,omitempty"`
	CauseCode     string `json:"cause_code,omitempty"`
	NextAction    string `json:"next_action,omitempty"`
}

type Alert struct {
	Code          string `json:"code"`
	Severity      string `json:"severity"`
	Section       string `json:"section"`
	SourceCommand string `json:"source_command"`
	Summary       string `json:"summary"`
	Cause         string `json:"cause,omitempty"`
	NextAction    string `json:"next_action,omitempty"`
}

func Summarize(req Request) (Info, error) {
	now := req.Now.UTC()
	if req.Now.IsZero() {
		now = time.Now().UTC()
	}

	info := Info{
		Scope:       strings.TrimSpace(req.Scope),
		ProjectDir:  strings.TrimSpace(req.ProjectDir),
		ComposeFile: strings.TrimSpace(req.ComposeFile),
		GeneratedAt: now.Format(time.RFC3339),
	}

	statusInfo, statusErr := statusreportusecase.Summarize(statusreportusecase.Request{
		Scope:           info.Scope,
		ProjectDir:      info.ProjectDir,
		ComposeFile:     info.ComposeFile,
		EnvFileOverride: strings.TrimSpace(req.EnvFileOverride),
		EnvContourHint:  strings.TrimSpace(req.EnvContourHint),
		JournalDir:      strings.TrimSpace(req.JournalDir),
		Now:             now,
	})
	info.Scope = statusInfo.Scope
	info.ProjectDir = statusInfo.ProjectDir
	info.ComposeFile = statusInfo.ComposeFile
	info.EnvFile = statusInfo.EnvFile
	info.BackupRoot = statusInfo.BackupRoot
	info.ReportsDir = statusInfo.ReportsDir
	info.SupportDir = statusInfo.SupportDir

	var collector reporting.SectionCollector
	baseWarnings := append([]string(nil), statusInfo.Warnings...)

	doctorSection, doctorAlerts := summarizeDoctorSection(findStatusReportSection(statusInfo.Sections, "doctor"))
	info.Sections = append(info.Sections, doctorSection)
	collector.Add(sectionCategory(doctorSection.Status), doctorSection.Code, nil)
	info.DoctorState = doctorSection.State
	info.Alerts = append(info.Alerts, doctorAlerts...)

	runtimeSection := summarizeRuntimeSection(findStatusReportSection(statusInfo.Sections, "runtime"))
	info.Sections = append(info.Sections, runtimeSection)
	collector.Add(sectionCategory(runtimeSection.Status), runtimeSection.Code, nil)
	info.RuntimeState = runtimeSection.State

	backupSection, backupAlerts, backupWarnings, backupErr := summarizeBackupSection(statusInfo.BackupRoot, strings.TrimSpace(req.JournalDir), req.VerifyChecksum, req.MaxAgeHours, now)
	info.Sections = append(info.Sections, backupSection)
	collector.Add(sectionCategory(backupSection.Status), backupSection.Code, backupWarnings)
	info.BackupState = backupSection.State
	info.Alerts = append(info.Alerts, backupAlerts...)
	baseWarnings = append(baseWarnings, backupWarnings...)

	latestOperationSection, latestOperationAlerts, latestOperationErr := summarizeLatestOperationSection(findStatusReportSection(statusInfo.Sections, "latest_operation"))
	info.Sections = append(info.Sections, latestOperationSection)
	collector.Add(sectionCategory(latestOperationSection.Status), latestOperationSection.Code, nil)
	info.LatestOperationState = latestOperationSection.State
	info.Alerts = append(info.Alerts, latestOperationAlerts...)
	info.LatestOperationID = latestOperationSectionID(findStatusReportSection(statusInfo.Sections, "latest_operation"))
	info.LatestOperationCmd = latestOperationSectionCommand(findStatusReportSection(statusInfo.Sections, "latest_operation"))

	maintenanceSection, maintenanceAlerts, maintenanceErr := summarizeMaintenanceSection(findStatusReportSection(statusInfo.Sections, "runtime"))
	info.Sections = append(info.Sections, maintenanceSection)
	collector.Add(sectionCategory(maintenanceSection.Status), maintenanceSection.Code, nil)
	info.MaintenanceState = maintenanceSection.State
	info.Alerts = append(info.Alerts, maintenanceAlerts...)

	sortAlerts(info.Alerts)

	summary := collector.Finalize(baseWarnings)
	info.IncludedSections = summary.IncludedSections
	info.OmittedSections = summary.OmittedSections
	info.FailedSections = summary.FailedSections
	info.Warnings = summary.Warnings
	info.Verdict = determineVerdict(info.Sections)
	info.NextAction = determineNextAction(info.Alerts, info.Sections)

	if info.Verdict != VerdictFailed {
		return info, nil
	}

	return info, primaryHealthSummaryFailure([]error{
		statusErr,
		backupErr,
		latestOperationErr,
		maintenanceErr,
	})
}

func summarizeDoctorSection(raw *statusreportusecase.Section) (Section, []Alert) {
	section := Section{
		Code:          "doctor",
		Status:        SectionStatusIncluded,
		State:         VerdictFailed,
		SourceCommand: "doctor",
		Summary:       "Doctor summary unavailable",
		NextAction:    "Rerun health-summary after restoring the canonical doctor path.",
		CauseCode:     "doctor_section_missing",
	}
	if raw == nil || raw.Doctor == nil {
		section.Status = SectionStatusFailed
		return section, []Alert{newFailureAlert("doctor_section_missing", "doctor", "doctor", section.Summary, "status-report did not return the doctor summary", section.NextAction)}
	}

	section.Summary = raw.Summary
	section.Details = raw.Details
	section.CauseCode = raw.FailureCode
	section.NextAction = raw.Action

	switch {
	case !raw.Doctor.Ready:
		section.State = VerdictBlocked
	case raw.Doctor.Warnings > 0:
		section.State = VerdictDegraded
	default:
		section.State = VerdictHealthy
	}

	alerts := doctorCheckAlerts(raw.Doctor.WarningChecks, AlertSeverityWarning)
	alerts = append(alerts, doctorCheckAlerts(raw.Doctor.FailedChecks, AlertSeverityBlocking)...)
	if !raw.Doctor.Ready && len(raw.Doctor.FailedChecks) == 0 {
		alerts = append(alerts, Alert{
			Code:          "doctor_failed",
			Severity:      AlertSeverityBlocking,
			Section:       "doctor",
			SourceCommand: "doctor",
			Summary:       raw.Summary,
			Cause:         raw.Details,
			NextAction:    raw.Action,
		})
	}
	if raw.Doctor.Ready && raw.Doctor.Warnings > 0 && len(raw.Doctor.WarningChecks) == 0 {
		alerts = append(alerts, Alert{
			Code:          "doctor_warnings_present",
			Severity:      AlertSeverityWarning,
			Section:       "doctor",
			SourceCommand: "doctor",
			Summary:       raw.Summary,
			Cause:         raw.Details,
			NextAction:    raw.Action,
		})
	}

	return section, alerts
}

func summarizeRuntimeSection(raw *statusreportusecase.Section) Section {
	section := Section{
		Code:          "runtime",
		Status:        SectionStatusFailed,
		State:         VerdictFailed,
		SourceCommand: "status-report",
		Summary:       "Runtime summary unavailable",
		NextAction:    "Restore Docker and runtime access before rerunning health-summary.",
		CauseCode:     "runtime_section_missing",
	}
	if raw == nil {
		section.Details = "status-report did not return the runtime summary"
		return section
	}

	section.Status = raw.Status
	section.Summary = raw.Summary
	section.Details = raw.Details
	section.CauseCode = raw.FailureCode
	section.NextAction = raw.Action

	switch raw.Status {
	case "included":
		section.State = runtimeSectionState(raw)
	case "omitted":
		section.Status = SectionStatusOmitted
		section.State = VerdictFailed
	case "failed":
		section.Status = SectionStatusFailed
		section.State = VerdictFailed
	default:
		section.Status = SectionStatusFailed
		section.State = VerdictFailed
	}

	return section
}

func summarizeBackupSection(backupRoot, journalDir string, verifyChecksum bool, maxAgeHours int, now time.Time) (Section, []Alert, []string, error) {
	section := Section{
		Code:          "backup",
		SourceCommand: "backup-health",
	}
	if strings.TrimSpace(backupRoot) == "" {
		section.Status = SectionStatusOmitted
		section.State = VerdictFailed
		section.Summary = "Backup posture omitted"
		section.Details = "Contour backup root is unavailable because the env context did not resolve cleanly."
		section.CauseCode = "backup_root_unavailable"
		section.NextAction = "Resolve the contour env file before rerunning health-summary."
		return section, nil, nil, nil
	}

	info, err := backupusecase.Health(backupusecase.HealthRequest{
		BackupRoot:     backupRoot,
		JournalDir:     journalDir,
		VerifyChecksum: verifyChecksum,
		MaxAgeHours:    maxAgeHours,
		Now:            now,
	})
	if err != nil {
		section.Status = SectionStatusFailed
		section.State = VerdictFailed
		section.Summary = "Backup posture unavailable"
		section.Details = err.Error()
		section.CauseCode = healthSummaryFailureCode(err, "backup_health_unavailable")
		section.NextAction = "Repair backup catalog access before rerunning health-summary."
		return section, []Alert{newFailureAlert(section.CauseCode, "backup", "backup-health", section.Summary, section.Details, section.NextAction)}, nil, err
	}

	section.Status = SectionStatusIncluded
	section.State = backupSectionState(info.Verdict)
	section.Summary = "Backup posture " + info.Verdict
	section.Details = backupDetailsText(info)
	section.NextAction = info.NextAction

	alerts := make([]Alert, 0, len(info.Alerts))
	for _, alert := range info.Alerts {
		alerts = append(alerts, Alert{
			Code:          alert.Code,
			Severity:      backupAlertSeverity(alert.Level),
			Section:       "backup",
			SourceCommand: "backup-health",
			Summary:       alert.Summary,
			Cause:         alert.Details,
			NextAction:    alert.Action,
		})
	}

	return section, alerts, backupJournalWarnings(info.JournalRead), nil
}

func summarizeLatestOperationSection(raw *statusreportusecase.Section) (Section, []Alert, error) {
	section := Section{
		Code:          "latest_operation",
		SourceCommand: "status-report",
	}
	if raw == nil || raw.LatestOperation == nil {
		section.Status = SectionStatusFailed
		section.State = VerdictFailed
		section.Summary = "Latest operation summary unavailable"
		section.Details = "status-report did not return the latest operation summary"
		section.CauseCode = "latest_operation_missing"
		section.NextAction = "Repair journal access before rerunning health-summary."
		err := apperr.Wrap(apperr.KindValidation, "latest_operation_missing", errors.New("latest operation summary missing"))
		return section, []Alert{newFailureAlert("latest_operation_missing", "latest_operation", "status-report", section.Summary, section.Details, section.NextAction)}, err
	}

	section.Status = raw.Status
	section.Summary = raw.Summary
	section.Details = raw.Details
	section.CauseCode = raw.FailureCode
	section.NextAction = raw.Action

	if raw.Status == "omitted" {
		section.Status = SectionStatusOmitted
		section.State = VerdictFailed
		err := apperr.Wrap(apperr.KindValidation, healthSummaryFailureCodeFromRaw(raw, "latest_operation_unavailable"), errors.New("latest operation summary unavailable"))
		return section, []Alert{newFailureAlert(healthSummaryFailureCodeFromRaw(raw, "latest_operation_unavailable"), "latest_operation", "status-report", section.Summary, section.Details, section.NextAction)}, err
	}
	if raw.Status == "failed" {
		section.Status = SectionStatusFailed
		section.State = VerdictFailed
		err := apperr.Wrap(apperr.KindValidation, healthSummaryFailureCodeFromRaw(raw, "latest_operation_failed"), errors.New("latest operation summary failed"))
		return section, []Alert{newFailureAlert(healthSummaryFailureCodeFromRaw(raw, "latest_operation_failed"), "latest_operation", "status-report", section.Summary, section.Details, section.NextAction)}, err
	}

	section.Status = SectionStatusIncluded
	operation := raw.LatestOperation.Operation
	if operation == nil {
		section.State = VerdictHealthy
		return section, nil, nil
	}

	switch operation.Status {
	case journalusecase.OperationStatusCompleted:
		section.State = VerdictHealthy
		return section, nil, nil
	case journalusecase.OperationStatusFailed, journalusecase.OperationStatusBlocked, journalusecase.OperationStatusRunning, journalusecase.OperationStatusUnknown:
		section.State = VerdictDegraded
	default:
		section.State = VerdictDegraded
	}

	alert := Alert{
		Code:          "latest_operation_" + operation.Status,
		Severity:      AlertSeverityWarning,
		Section:       "latest_operation",
		SourceCommand: "status-report",
		Summary:       latestOperationSummary(operation),
		Cause:         latestOperationCause(operation),
		NextAction:    latestOperationAction(operation),
	}

	return section, []Alert{alert}, nil
}

func summarizeMaintenanceSection(runtime *statusreportusecase.Section) (Section, []Alert, error) {
	section := Section{
		Code:          "maintenance",
		SourceCommand: "status-report",
	}
	if runtime == nil || runtime.Runtime == nil {
		section.Status = SectionStatusOmitted
		section.State = VerdictFailed
		section.Summary = "Maintenance posture omitted"
		section.Details = "Runtime summary did not expose maintenance lock state."
		section.CauseCode = "maintenance_state_unavailable"
		section.NextAction = "Restore contour runtime access before rerunning health-summary."
		return section, nil, nil
	}

	lockState := strings.TrimSpace(runtime.Runtime.MaintenanceLock.State)
	section.Status = SectionStatusIncluded
	section.Details = runtimeLockDetails(runtime.Runtime.MaintenanceLock)

	switch lockState {
	case locks.LockReady:
		section.State = VerdictHealthy
		section.Summary = "Maintenance lock is ready"
		return section, nil, nil
	case locks.LockActive:
		section.State = VerdictBlocked
		section.Summary = "Maintenance lock is active"
		section.NextAction = "Wait for the active maintenance operation to finish before relying on another stateful run."
		return section, []Alert{{
			Code:          "maintenance_lock_active",
			Severity:      AlertSeverityBlocking,
			Section:       "maintenance",
			SourceCommand: "status-report",
			Summary:       section.Summary,
			Cause:         section.Details,
			NextAction:    section.NextAction,
		}}, nil
	case locks.LockLegacy:
		section.State = VerdictBlocked
		section.Summary = "Legacy maintenance lock blocks safe operations"
		section.NextAction = "Remove the legacy maintenance lock only after verifying that no maintenance process still owns it."
		return section, []Alert{{
			Code:          "maintenance_lock_legacy",
			Severity:      AlertSeverityBlocking,
			Section:       "maintenance",
			SourceCommand: "status-report",
			Summary:       section.Summary,
			Cause:         section.Details,
			NextAction:    section.NextAction,
		}}, nil
	case locks.LockStale:
		section.State = VerdictDegraded
		section.Summary = "Stale maintenance lock metadata is present"
		section.NextAction = "Clean up the stale maintenance lock metadata after confirming that no maintenance run is still active."
		return section, []Alert{{
			Code:          "maintenance_lock_stale",
			Severity:      AlertSeverityWarning,
			Section:       "maintenance",
			SourceCommand: "status-report",
			Summary:       section.Summary,
			Cause:         section.Details,
			NextAction:    section.NextAction,
		}}, nil
	default:
		section.Status = SectionStatusFailed
		section.State = VerdictFailed
		section.Summary = "Maintenance posture unavailable"
		section.CauseCode = "maintenance_state_unavailable"
		if strings.TrimSpace(section.Details) == "" {
			section.Details = "Maintenance lock inspection returned an unknown state."
		}
		section.NextAction = "Repair maintenance lock inspection before rerunning health-summary."
		err := apperr.Wrap(apperr.KindValidation, "maintenance_state_unavailable", errors.New("maintenance lock inspection unavailable"))
		return section, []Alert{newFailureAlert("maintenance_state_unavailable", "maintenance", "status-report", section.Summary, section.Details, section.NextAction)}, err
	}
}

func doctorCheckAlerts(checks []doctorusecase.Check, severity string) []Alert {
	alerts := make([]Alert, 0, len(checks))
	for _, check := range checks {
		alerts = append(alerts, Alert{
			Code:          "doctor_" + strings.TrimSpace(check.Code),
			Severity:      severity,
			Section:       "doctor",
			SourceCommand: "doctor",
			Summary:       doctorCheckSummary(check),
			Cause:         doctorCheckCause(check),
			NextAction:    strings.TrimSpace(check.Action),
		})
	}

	return alerts
}

func determineVerdict(sections []Section) string {
	verdict := VerdictHealthy
	for _, section := range sections {
		if section.Status != SectionStatusIncluded || section.State == VerdictFailed {
			return VerdictFailed
		}
		if section.State == VerdictBlocked {
			verdict = VerdictBlocked
			continue
		}
		if verdict == VerdictHealthy && section.State == VerdictDegraded {
			verdict = VerdictDegraded
		}
	}

	return verdict
}

func determineNextAction(alerts []Alert, sections []Section) string {
	for _, alert := range alerts {
		if strings.TrimSpace(alert.NextAction) != "" {
			return alert.NextAction
		}
	}
	for _, section := range sections {
		if section.Status != SectionStatusIncluded && strings.TrimSpace(section.NextAction) != "" {
			return section.NextAction
		}
	}
	for _, section := range sections {
		if section.State != VerdictHealthy && strings.TrimSpace(section.NextAction) != "" {
			return section.NextAction
		}
	}
	return ""
}

func sectionCategory(status string) reporting.SectionCategory {
	switch status {
	case SectionStatusIncluded:
		return reporting.SectionIncluded
	case SectionStatusOmitted:
		return reporting.SectionOmitted
	case SectionStatusFailed:
		return reporting.SectionFailed
	default:
		return reporting.SectionIgnored
	}
}

func findStatusReportSection(sections []statusreportusecase.Section, code string) *statusreportusecase.Section {
	for idx := range sections {
		if sections[idx].Code == code {
			return &sections[idx]
		}
	}
	return nil
}

func runtimeSectionState(raw *statusreportusecase.Section) string {
	if raw == nil || raw.Runtime == nil {
		return VerdictFailed
	}
	if len(raw.Warnings) != 0 {
		return VerdictDegraded
	}
	for _, service := range raw.Runtime.Services {
		switch service.Status {
		case "healthy", "running", "not_created", "exited":
			continue
		default:
			return VerdictDegraded
		}
	}
	return VerdictHealthy
}

func backupSectionState(verdict string) string {
	switch verdict {
	case backupusecase.HealthVerdictHealthy:
		return VerdictHealthy
	case backupusecase.HealthVerdictDegraded:
		return VerdictDegraded
	case backupusecase.HealthVerdictBlocked:
		return VerdictBlocked
	default:
		return VerdictFailed
	}
}

func backupDetailsText(info backupusecase.HealthInfo) string {
	parts := []string{
		fmt.Sprintf("total sets=%d", info.TotalSets),
		fmt.Sprintf("ready sets=%d", info.ReadySets),
		fmt.Sprintf("restore ready=%t", info.RestoreReady),
	}
	if info.LatestReadySet != nil {
		parts = append(parts, "latest ready set="+info.LatestReadySet.ID)
	}
	if info.LatestReadyAgeHours != nil {
		parts = append(parts, fmt.Sprintf("latest ready age=%dh", *info.LatestReadyAgeHours))
	}
	return strings.Join(parts, ", ")
}

func backupAlertSeverity(level string) string {
	switch level {
	case backupusecase.HealthAlertBreach:
		return AlertSeverityBlocking
	default:
		return AlertSeverityWarning
	}
}

func backupJournalWarnings(stats backupusecase.JournalReadStats) []string {
	if stats.SkippedCorrupt == 0 {
		return nil
	}
	return []string{fmt.Sprintf("skipped %d corrupt journal entrie(s)", stats.SkippedCorrupt)}
}

func latestOperationSectionID(raw *statusreportusecase.Section) string {
	if raw == nil || raw.LatestOperation == nil || raw.LatestOperation.Operation == nil {
		return ""
	}
	return raw.LatestOperation.Operation.OperationID
}

func latestOperationSectionCommand(raw *statusreportusecase.Section) string {
	if raw == nil || raw.LatestOperation == nil || raw.LatestOperation.Operation == nil {
		return ""
	}
	return raw.LatestOperation.Operation.Command
}

func latestOperationSummary(operation *journalusecase.OperationSummary) string {
	if operation == nil {
		return "Latest operation reported an issue"
	}
	return fmt.Sprintf("Latest %s operation is %s", operation.Command, operation.Status)
}

func latestOperationCause(operation *journalusecase.OperationSummary) string {
	if operation == nil {
		return ""
	}
	if operation.Failure != nil {
		switch {
		case strings.TrimSpace(operation.Failure.Code) != "" && strings.TrimSpace(operation.Failure.StepSummary) != "":
			return fmt.Sprintf("%s: %s", operation.Failure.Code, operation.Failure.StepSummary)
		case strings.TrimSpace(operation.Failure.Code) != "" && strings.TrimSpace(operation.Failure.Message) != "":
			return fmt.Sprintf("%s: %s", operation.Failure.Code, operation.Failure.Message)
		case strings.TrimSpace(operation.Failure.StepSummary) != "":
			return operation.Failure.StepSummary
		case strings.TrimSpace(operation.Failure.Message) != "":
			return operation.Failure.Message
		}
	}
	if strings.TrimSpace(operation.Summary) != "" {
		return operation.Summary
	}
	if strings.TrimSpace(operation.ErrorCode) != "" {
		return operation.ErrorCode
	}
	return fmt.Sprintf("operation %s reported status %s", operation.OperationID, operation.Status)
}

func latestOperationAction(operation *journalusecase.OperationSummary) string {
	if operation == nil || strings.TrimSpace(operation.OperationID) == "" {
		return "Use history or show-operation to inspect the latest recorded operation."
	}
	return fmt.Sprintf("Use show-operation --id %s to inspect the latest run before relying on automation.", operation.OperationID)
}

func runtimeLockDetails(lock statusreportusecase.LockSnapshot) string {
	parts := []string{}
	if strings.TrimSpace(lock.State) != "" {
		parts = append(parts, "state="+lock.State)
	}
	if strings.TrimSpace(lock.MetadataPath) != "" {
		parts = append(parts, "path="+lock.MetadataPath)
	}
	if strings.TrimSpace(lock.PID) != "" {
		parts = append(parts, "pid="+lock.PID)
	}
	if strings.TrimSpace(lock.Error) != "" {
		parts = append(parts, "error="+lock.Error)
	}
	if len(lock.StalePaths) != 0 {
		parts = append(parts, "stale="+strings.Join(lock.StalePaths, ", "))
	}
	return strings.Join(parts, " | ")
}

func doctorCheckSummary(check doctorusecase.Check) string {
	summary := strings.TrimSpace(check.Summary)
	if strings.TrimSpace(check.Scope) == "" {
		return summary
	}
	return check.Scope + ": " + summary
}

func doctorCheckCause(check doctorusecase.Check) string {
	if strings.TrimSpace(check.Details) != "" {
		return check.Details
	}
	return strings.TrimSpace(check.Summary)
}

func sortAlerts(alerts []Alert) {
	sort.SliceStable(alerts, func(i, j int) bool {
		return alertSeverityRank(alerts[i].Severity) < alertSeverityRank(alerts[j].Severity)
	})
}

func alertSeverityRank(severity string) int {
	switch severity {
	case AlertSeverityFailure:
		return 0
	case AlertSeverityBlocking:
		return 1
	case AlertSeverityWarning:
		return 2
	default:
		return 3
	}
}

func newFailureAlert(code, section, source, summary, cause, nextAction string) Alert {
	return Alert{
		Code:          code,
		Severity:      AlertSeverityFailure,
		Section:       section,
		SourceCommand: source,
		Summary:       summary,
		Cause:         cause,
		NextAction:    nextAction,
	}
}

func primaryHealthSummaryFailure(errs []error) error {
	for _, err := range errs {
		if err == nil {
			continue
		}
		if kind, ok := apperr.KindOf(err); ok && kind != apperr.KindValidation {
			return wrapHealthSummaryFailure(err)
		}
	}
	for _, err := range errs {
		if err != nil {
			return wrapHealthSummaryFailure(err)
		}
	}
	return apperr.Wrap(apperr.KindValidation, "health_summary_failed", errors.New("health summary could not produce a complete verdict"))
}

func wrapHealthSummaryFailure(err error) error {
	if err == nil {
		return nil
	}
	if code, ok := apperr.CodeOf(err); ok && code == "health_summary_failed" {
		return err
	}
	if kind, ok := apperr.KindOf(err); ok {
		return apperr.Wrap(kind, "health_summary_failed", err)
	}
	return apperr.Wrap(apperr.KindValidation, "health_summary_failed", err)
}

func healthSummaryFailureCode(err error, fallback string) string {
	if code, ok := apperr.CodeOf(err); ok {
		return code
	}
	return fallback
}

func healthSummaryFailureCodeFromRaw(raw *statusreportusecase.Section, fallback string) string {
	if raw == nil || strings.TrimSpace(raw.FailureCode) == "" {
		return fallback
	}
	return raw.FailureCode
}
