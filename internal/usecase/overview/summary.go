package overview

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/lazuale/espocrm-ops/internal/contract/apperr"
	"github.com/lazuale/espocrm-ops/internal/platform/config"
	platformdocker "github.com/lazuale/espocrm-ops/internal/platform/docker"
	platformlocks "github.com/lazuale/espocrm-ops/internal/platform/locks"
	backupusecase "github.com/lazuale/espocrm-ops/internal/usecase/backup"
	doctorusecase "github.com/lazuale/espocrm-ops/internal/usecase/doctor"
	journalusecase "github.com/lazuale/espocrm-ops/internal/usecase/journal"
)

const (
	sectionDoctor           = "doctor"
	sectionRuntime          = "runtime"
	sectionBackup           = "backup"
	sectionRecentOperations = "recent_operations"

	sectionStatusIncluded = "included"
	sectionStatusOmitted  = "omitted"
	sectionStatusFailed   = "failed"

	overviewHistoryLimit = 3
)

var runtimeServices = []string{
	"db",
	"espocrm",
	"espocrm-daemon",
	"espocrm-websocket",
}

type Request struct {
	Scope           string
	ProjectDir      string
	ComposeFile     string
	EnvFileOverride string
	EnvContourHint  string
	JournalDir      string
	Now             time.Time
}

type Info struct {
	Scope            string
	ProjectDir       string
	ComposeFile      string
	EnvFile          string
	BackupRoot       string
	GeneratedAt      string
	IncludedSections []string
	OmittedSections  []string
	FailedSections   []string
	Warnings         []string
	Sections         []Section
}

type Section struct {
	Code             string                `json:"code"`
	Status           string                `json:"status"`
	Summary          string                `json:"summary"`
	Details          string                `json:"details,omitempty"`
	Action           string                `json:"action,omitempty"`
	FailureCode      string                `json:"failure_code,omitempty"`
	Warnings         []string              `json:"warnings,omitempty"`
	Doctor           *DoctorData           `json:"doctor,omitempty"`
	Runtime          *RuntimeData          `json:"runtime,omitempty"`
	Backup           *BackupData           `json:"backup,omitempty"`
	RecentOperations *RecentOperationsData `json:"recent_operations,omitempty"`
}

type DoctorData struct {
	Ready         bool                  `json:"ready"`
	Checks        int                   `json:"checks"`
	Passed        int                   `json:"passed"`
	Warnings      int                   `json:"warnings"`
	Failed        int                   `json:"failed"`
	WarningChecks []doctorusecase.Check `json:"warning_checks,omitempty"`
	FailedChecks  []doctorusecase.Check `json:"failed_checks,omitempty"`
}

type RuntimeData struct {
	ComposeProject      string           `json:"compose_project"`
	EnvFile             string           `json:"env_file"`
	SiteURL             string           `json:"site_url,omitempty"`
	WSPublicURL         string           `json:"ws_public_url,omitempty"`
	Services            []RuntimeService `json:"services"`
	SharedOperationLock LockSnapshot     `json:"shared_operation_lock"`
	MaintenanceLock     LockSnapshot     `json:"maintenance_lock"`
}

type RuntimeService struct {
	Name    string `json:"name"`
	Status  string `json:"status"`
	Details string `json:"details,omitempty"`
}

type LockSnapshot struct {
	State        string   `json:"state"`
	MetadataPath string   `json:"metadata_path,omitempty"`
	PID          string   `json:"pid,omitempty"`
	StalePaths   []string `json:"stale_paths,omitempty"`
	Error        string   `json:"error,omitempty"`
}

type BackupData struct {
	VerifyChecksum  bool                         `json:"verify_checksum"`
	AuditSuccess    bool                         `json:"audit_success"`
	SelectedPrefix  string                       `json:"selected_prefix,omitempty"`
	SelectedStamp   string                       `json:"selected_stamp,omitempty"`
	TotalSets       int                          `json:"total_sets"`
	LatestReady     *backupusecase.CatalogItem   `json:"latest_ready,omitempty"`
	WarningFindings []backupusecase.AuditFinding `json:"warning_findings,omitempty"`
	FailureFindings []backupusecase.AuditFinding `json:"failure_findings,omitempty"`
}

type RecentOperationsData struct {
	Returned       int                               `json:"returned"`
	TotalFilesSeen int                               `json:"total_files_seen"`
	LoadedEntries  int                               `json:"loaded_entries"`
	SkippedCorrupt int                               `json:"skipped_corrupt"`
	Operations     []journalusecase.OperationSummary `json:"operations,omitempty"`
}

func Summarize(req Request) (Info, error) {
	now := req.Now.UTC()
	if req.Now.IsZero() {
		now = time.Now().UTC()
	}

	info := Info{
		Scope:       strings.TrimSpace(req.Scope),
		ProjectDir:  filepath.Clean(req.ProjectDir),
		ComposeFile: filepath.Clean(req.ComposeFile),
		GeneratedAt: now.Format(time.RFC3339),
	}

	failedErrs := []error{}

	doctorSection, doctorErr := buildDoctorSection(doctorusecase.Diagnose(doctorusecase.Request{
		Scope:           info.Scope,
		ProjectDir:      info.ProjectDir,
		ComposeFile:     info.ComposeFile,
		EnvFileOverride: strings.TrimSpace(req.EnvFileOverride),
		EnvContourHint:  strings.TrimSpace(req.EnvContourHint),
		PathCheckMode:   doctorusecase.PathCheckModeReadOnly,
	}))
	info.Sections = append(info.Sections, doctorSection)
	if doctorErr != nil {
		failedErrs = append(failedErrs, doctorErr)
	}

	env, envErr := config.LoadOperationEnv(
		info.ProjectDir,
		info.Scope,
		strings.TrimSpace(req.EnvFileOverride),
		strings.TrimSpace(req.EnvContourHint),
	)
	if envErr == nil {
		info.EnvFile = env.FilePath
		info.BackupRoot = config.ResolveProjectPath(info.ProjectDir, env.BackupRoot())
	}

	runtimeSection := buildRuntimeSection(info.ProjectDir, info.ComposeFile, env, envErr)
	info.Sections = append(info.Sections, runtimeSection)

	backupSection, backupErr := buildBackupSection(info.BackupRoot, env, envErr, strings.TrimSpace(req.JournalDir), now)
	info.Sections = append(info.Sections, backupSection)
	if backupErr != nil {
		failedErrs = append(failedErrs, backupErr)
	}

	recentSection := buildRecentOperationsSection(strings.TrimSpace(req.JournalDir), info.Scope)
	info.Sections = append(info.Sections, recentSection)

	finalizeSectionLists(&info)
	info.Warnings = dedupeStrings(info.Warnings)

	if len(info.FailedSections) == 0 {
		return info, nil
	}

	return info, primaryOverviewFailure(failedErrs, info.FailedSections)
}

func buildDoctorSection(report doctorusecase.Report, _ error) (Section, error) {
	passed, warnings, failed := report.Counts()
	warningChecks := filterDoctorChecks(report.Checks, "warn")
	failedChecks := filterDoctorChecks(report.Checks, "fail")

	section := Section{
		Code:     sectionDoctor,
		Status:   sectionStatusIncluded,
		Summary:  "Doctor checks passed",
		Details:  fmt.Sprintf("Ready with %d checks: %d passed, %d warnings, %d failed.", len(report.Checks), passed, warnings, failed),
		Warnings: doctorWarningMessages(warningChecks),
		Doctor: &DoctorData{
			Ready:         report.Ready(),
			Checks:        len(report.Checks),
			Passed:        passed,
			Warnings:      warnings,
			Failed:        failed,
			WarningChecks: append([]doctorusecase.Check(nil), warningChecks...),
			FailedChecks:  append([]doctorusecase.Check(nil), failedChecks...),
		},
	}

	if report.Ready() {
		return section, nil
	}

	section.Status = sectionStatusFailed
	section.Summary = "Doctor found readiness failures"
	section.Details = fmt.Sprintf("Doctor reported %d blocking readiness failure(s) and %d warning(s).", failed, warnings)
	section.Action = firstDoctorFailureAction(failedChecks)
	section.FailureCode = "doctor_failed"

	return section, apperr.Wrap(apperr.KindValidation, "doctor_failed", errors.New("doctor found readiness failures"))
}

func buildRuntimeSection(projectDir, composeFile string, env config.OperationEnv, envErr error) Section {
	lockWarnings := []string{}
	shared := inspectSharedOperationLock(projectDir, &lockWarnings)

	section := Section{
		Code:     sectionRuntime,
		Status:   sectionStatusIncluded,
		Summary:  "Runtime status collected",
		Warnings: append([]string(nil), lockWarnings...),
	}

	if envErr != nil {
		section.Status = sectionStatusOmitted
		section.Summary = "Runtime status omitted"
		section.Details = envErr.Error()
		section.Action = "Resolve env resolution before rerunning overview to collect runtime status."
		section.Warnings = append(section.Warnings, fmt.Sprintf("runtime omitted: %v", envErr))
		return section
	}

	maintenanceWarnings := []string{}
	maintenance := inspectMaintenanceLock(config.ResolveProjectPath(projectDir, env.BackupRoot()), &maintenanceWarnings)
	section.Warnings = append(section.Warnings, maintenanceWarnings...)

	cfg := platformdocker.ComposeConfig{
		ProjectDir:  projectDir,
		ComposeFile: composeFile,
		EnvFile:     env.FilePath,
	}

	services, runtimeErr := collectRuntimeServices(cfg)
	if runtimeErr != nil {
		section.Status = sectionStatusOmitted
		section.Summary = "Runtime status omitted"
		section.Details = runtimeErr.Error()
		section.Action = "Restore Docker and Docker Compose access before rerunning overview to collect runtime status."
		section.Warnings = append(section.Warnings, fmt.Sprintf("runtime omitted: %v", runtimeErr))
		section.Runtime = &RuntimeData{
			ComposeProject:      env.ComposeProject(),
			EnvFile:             env.FilePath,
			SiteURL:             env.Value("SITE_URL"),
			WSPublicURL:         env.Value("WS_PUBLIC_URL"),
			SharedOperationLock: shared,
			MaintenanceLock:     maintenance,
		}
		section.Warnings = dedupeStrings(section.Warnings)
		return section
	}

	section.Runtime = &RuntimeData{
		ComposeProject:      env.ComposeProject(),
		EnvFile:             env.FilePath,
		SiteURL:             env.Value("SITE_URL"),
		WSPublicURL:         env.Value("WS_PUBLIC_URL"),
		Services:            services,
		SharedOperationLock: shared,
		MaintenanceLock:     maintenance,
	}
	section.Details = runtimeDetails(services)
	section.Warnings = append(section.Warnings, runtimeServiceWarnings(services)...)
	section.Warnings = dedupeStrings(section.Warnings)

	return section
}

func buildBackupSection(backupRoot string, env config.OperationEnv, envErr error, journalDir string, now time.Time) (Section, error) {
	section := Section{
		Code: sectionBackup,
		Backup: &BackupData{
			VerifyChecksum: true,
		},
	}

	if envErr != nil {
		section.Status = sectionStatusFailed
		section.Summary = "Backup summary unavailable"
		section.Details = envErr.Error()
		section.Action = "Resolve env resolution before rerunning overview to inspect backup state."
		section.FailureCode = "backup_env_resolution_failed"
		return section, apperr.Wrap(apperr.KindValidation, "backup_env_resolution_failed", envErr)
	}

	dbMaxAge, err := requiredIntEnv(env, "BACKUP_MAX_DB_AGE_HOURS")
	if err != nil {
		section.Status = sectionStatusFailed
		section.Summary = "Backup summary unavailable"
		section.Details = err.Error()
		section.Action = "Set BACKUP_MAX_DB_AGE_HOURS to a non-negative integer before rerunning overview."
		section.FailureCode = "backup_threshold_invalid"
		return section, apperr.Wrap(apperr.KindValidation, "backup_threshold_invalid", err)
	}
	filesMaxAge, err := requiredIntEnv(env, "BACKUP_MAX_FILES_AGE_HOURS")
	if err != nil {
		section.Status = sectionStatusFailed
		section.Summary = "Backup summary unavailable"
		section.Details = err.Error()
		section.Action = "Set BACKUP_MAX_FILES_AGE_HOURS to a non-negative integer before rerunning overview."
		section.FailureCode = "backup_threshold_invalid"
		return section, apperr.Wrap(apperr.KindValidation, "backup_threshold_invalid", err)
	}

	audit, auditErr := backupusecase.Audit(backupusecase.AuditRequest{
		BackupRoot:       backupRoot,
		VerifyChecksum:   true,
		DBMaxAgeHours:    dbMaxAge,
		FilesMaxAgeHours: filesMaxAge,
		Now:              now,
	})
	if auditErr == nil {
		section.Backup.AuditSuccess = audit.Success
		section.Backup.SelectedPrefix = audit.SelectedSet.Prefix
		section.Backup.SelectedStamp = audit.SelectedSet.Stamp
		section.Backup.WarningFindings = filterAuditFindings(audit.Findings, backupusecase.AuditStatusWarn)
		section.Backup.FailureFindings = filterAuditFindings(audit.Findings, backupusecase.AuditStatusFail)
		section.Warnings = append(section.Warnings, backupFindingWarnings(section.Backup.WarningFindings)...)
	}

	catalog, catalogErr := backupusecase.Catalog(backupusecase.CatalogRequest{
		BackupRoot:     backupRoot,
		JournalDir:     journalDir,
		VerifyChecksum: true,
		ReadyOnly:      true,
		Limit:          1,
		Now:            now,
	})
	if catalogErr == nil {
		section.Backup.TotalSets = catalog.TotalSets
		section.Warnings = append(section.Warnings, backupJournalWarnings(catalog.JournalRead)...)
		if len(catalog.Items) > 0 {
			item := catalog.Items[0]
			section.Backup.LatestReady = &item
		}
	}

	section.Warnings = dedupeStrings(section.Warnings)

	switch {
	case auditErr != nil:
		section.Status = sectionStatusFailed
		section.Summary = "Backup summary unavailable"
		section.Details = auditErr.Error()
		section.Action = "Repair backup-root access before rerunning overview to inspect backup integrity."
		section.FailureCode = "backup_audit_failed"
		return section, apperr.Wrap(apperr.KindValidation, "backup_audit_failed", auditErr)
	case !audit.Success:
		section.Status = sectionStatusFailed
		section.Summary = "Backup summary found issues"
		section.Details = backupFailureSummary(audit.Findings)
		section.Action = "Resolve the reported backup freshness or integrity failures before relying on contour recovery."
		section.FailureCode = "backup_audit_failed"
		return section, apperr.Wrap(apperr.KindValidation, "backup_audit_failed", errors.New("backup audit found failures"))
	case catalogErr != nil:
		section.Status = sectionStatusFailed
		section.Summary = "Latest ready backup lookup failed"
		section.Details = catalogErr.Error()
		section.Action = "Repair backup catalog access before rerunning overview to inspect the latest ready backup set."
		section.FailureCode = "backup_catalog_failed"
		return section, apperr.Wrap(apperr.KindValidation, "backup_catalog_failed", catalogErr)
	case section.Backup.LatestReady == nil:
		section.Status = sectionStatusFailed
		section.Summary = "No restore-ready backup set is currently available"
		section.Details = "The backup catalog did not find a latest restore-ready backup set for this contour."
		section.Action = "Create or repair a coherent backup set before relying on contour recovery."
		section.FailureCode = "latest_ready_backup_missing"
		return section, apperr.Wrap(apperr.KindValidation, "latest_ready_backup_missing", errors.New("no restore-ready backup set is currently available"))
	default:
		section.Status = sectionStatusIncluded
		section.Summary = "Backup summary collected"
		section.Details = backupSuccessDetails(audit, section.Backup.LatestReady)
		return section, nil
	}
}

func buildRecentOperationsSection(journalDir, scope string) Section {
	section := Section{
		Code: sectionRecentOperations,
	}

	history, err := journalusecase.History(journalusecase.HistoryInput{
		JournalDir: journalDir,
		Filters: journalusecase.Filters{
			Scope: scope,
			Limit: overviewHistoryLimit,
		},
	})
	if err != nil {
		section.Status = sectionStatusOmitted
		section.Summary = "Recent operations omitted"
		section.Details = err.Error()
		section.Action = "Repair journal access before rerunning overview to inspect recent operations."
		section.Warnings = []string{fmt.Sprintf("recent operations omitted: %v", err)}
		return section
	}

	section.Status = sectionStatusIncluded
	section.Summary = fmt.Sprintf("Found %d recent operation(s)", len(history.Operations))
	if len(history.Operations) == 0 {
		section.Summary = "No recent operations recorded"
	}
	section.Details = fmt.Sprintf(
		"Scanned %d journal file(s), loaded %d entries, and skipped %d corrupt entries.",
		history.Stats.TotalFilesSeen,
		history.Stats.LoadedEntries,
		history.Stats.SkippedCorrupt,
	)
	section.Warnings = journalusecase.WarningsFromReadStats(history.Stats)
	section.RecentOperations = &RecentOperationsData{
		Returned:       len(history.Operations),
		TotalFilesSeen: history.Stats.TotalFilesSeen,
		LoadedEntries:  history.Stats.LoadedEntries,
		SkippedCorrupt: history.Stats.SkippedCorrupt,
		Operations:     append([]journalusecase.OperationSummary(nil), history.Operations...),
	}

	return section
}

func inspectSharedOperationLock(projectDir string, warnings *[]string) LockSnapshot {
	readiness, err := platformlocks.CheckSharedOperationReadiness(projectDir)
	if err != nil {
		*warnings = append(*warnings, fmt.Sprintf("could not inspect the shared operations lock: %v", err))
		return LockSnapshot{
			State: "unknown",
			Error: err.Error(),
		}
	}

	snapshot := LockSnapshot{
		State:        readiness.State,
		MetadataPath: readiness.MetadataPath,
		PID:          readiness.PID,
		StalePaths:   append([]string(nil), readiness.StalePaths...),
	}
	if readiness.State == platformlocks.LockActive {
		*warnings = append(*warnings, "the shared operations lock is currently active")
	}
	if readiness.State == platformlocks.LockStale {
		*warnings = append(*warnings, "the shared operations lock metadata is stale")
	}
	if readiness.State == platformlocks.LockLegacy {
		*warnings = append(*warnings, "a legacy shared operations lock blocks safe inspection")
	}

	return snapshot
}

func inspectMaintenanceLock(backupRoot string, warnings *[]string) LockSnapshot {
	readiness, err := platformlocks.CheckMaintenanceReadiness(backupRoot)
	if err != nil {
		*warnings = append(*warnings, fmt.Sprintf("could not inspect the maintenance lock: %v", err))
		return LockSnapshot{
			State: "unknown",
			Error: err.Error(),
		}
	}

	snapshot := LockSnapshot{
		State:        readiness.State,
		MetadataPath: readiness.MetadataPath,
		PID:          readiness.PID,
		StalePaths:   append([]string(nil), readiness.StalePaths...),
	}
	if readiness.State == platformlocks.LockActive {
		*warnings = append(*warnings, "a maintenance lock is currently active for this contour")
	}
	if readiness.State == platformlocks.LockStale {
		*warnings = append(*warnings, "stale maintenance lock metadata is present")
	}
	if readiness.State == platformlocks.LockLegacy {
		*warnings = append(*warnings, "a legacy maintenance lock blocks safe inspection")
	}

	return snapshot
}

func collectRuntimeServices(cfg platformdocker.ComposeConfig) ([]RuntimeService, error) {
	services := make([]RuntimeService, 0, len(runtimeServices))
	for _, service := range runtimeServices {
		state, err := platformdocker.ComposeServiceStateFor(cfg, service)
		if err != nil {
			return nil, err
		}

		status := strings.TrimSpace(state.Status)
		if status == "" {
			status = "not_created"
		}

		item := RuntimeService{
			Name:   service,
			Status: status,
		}
		if strings.TrimSpace(state.HealthMessage) != "" {
			item.Details = strings.TrimSpace(state.HealthMessage)
		}
		services = append(services, item)
	}

	return services, nil
}

func runtimeDetails(services []RuntimeService) string {
	running := 0
	for _, service := range services {
		if service.Status == "running" || service.Status == "healthy" {
			running++
		}
	}
	if running == 0 {
		return "No runtime containers currently report healthy or running status."
	}

	return fmt.Sprintf("%d service(s) currently report healthy or running status.", running)
}

func runtimeServiceWarnings(services []RuntimeService) []string {
	warnings := []string{}
	for _, service := range services {
		switch service.Status {
		case "healthy", "running", "not_created", "exited":
			continue
		case "unhealthy":
			if service.Details != "" {
				warnings = append(warnings, fmt.Sprintf("service %s is unhealthy: %s", service.Name, service.Details))
			} else {
				warnings = append(warnings, fmt.Sprintf("service %s is unhealthy", service.Name))
			}
		default:
			warnings = append(warnings, fmt.Sprintf("service %s reports %s", service.Name, service.Status))
		}
	}

	return warnings
}

func requiredIntEnv(env config.OperationEnv, key string) (int, error) {
	value := strings.TrimSpace(env.Value(key))
	if value == "" {
		return 0, fmt.Errorf("%s is required in %s", key, env.FilePath)
	}

	parsed, err := strconv.Atoi(value)
	if err != nil || parsed < 0 {
		return 0, fmt.Errorf("%s must be a non-negative integer in %s", key, env.FilePath)
	}

	return parsed, nil
}

func backupSuccessDetails(audit backupusecase.AuditInfo, latestReady *backupusecase.CatalogItem) string {
	if latestReady == nil {
		return "Backup audit passed and no latest ready backup was recorded."
	}

	selected := "n/a"
	if audit.SelectedSet.Prefix != "" || audit.SelectedSet.Stamp != "" {
		selected = strings.Trim(strings.Join([]string{audit.SelectedSet.Prefix, audit.SelectedSet.Stamp}, " | "), " |")
	}

	return fmt.Sprintf("Backup audit passed for %s and the latest restore-ready backup is %s.", selected, latestReady.ID)
}

func backupFailureSummary(findings []backupusecase.AuditFinding) string {
	failures := filterAuditFindings(findings, backupusecase.AuditStatusFail)
	if len(failures) == 0 {
		return "Backup audit reported failures."
	}

	parts := make([]string, 0, len(failures))
	for _, finding := range failures {
		parts = append(parts, fmt.Sprintf("%s: %s", finding.Subject, finding.Message))
	}
	return strings.Join(parts, "; ")
}

func filterAuditFindings(findings []backupusecase.AuditFinding, level string) []backupusecase.AuditFinding {
	items := make([]backupusecase.AuditFinding, 0, len(findings))
	for _, finding := range findings {
		if finding.Level == level {
			items = append(items, finding)
		}
	}
	return items
}

func filterDoctorChecks(checks []doctorusecase.Check, status string) []doctorusecase.Check {
	items := make([]doctorusecase.Check, 0, len(checks))
	for _, check := range checks {
		if check.Status == status {
			items = append(items, check)
		}
	}
	return items
}

func doctorWarningMessages(checks []doctorusecase.Check) []string {
	warnings := make([]string, 0, len(checks))
	for _, check := range checks {
		warnings = append(warnings, formatDoctorCheck(check))
	}
	return warnings
}

func firstDoctorFailureAction(checks []doctorusecase.Check) string {
	for _, check := range checks {
		if strings.TrimSpace(check.Action) != "" {
			return check.Action
		}
	}
	return "Resolve the reported doctor failures before relying on this contour."
}

func formatDoctorCheck(check doctorusecase.Check) string {
	if strings.TrimSpace(check.Scope) == "" {
		return check.Summary
	}
	return fmt.Sprintf("%s: %s", check.Scope, check.Summary)
}

func backupFindingWarnings(findings []backupusecase.AuditFinding) []string {
	warnings := make([]string, 0, len(findings))
	for _, finding := range findings {
		warnings = append(warnings, fmt.Sprintf("%s: %s", finding.Subject, finding.Message))
	}
	return warnings
}

func backupJournalWarnings(stats backupusecase.JournalReadStats) []string {
	return journalusecase.WarningsFromReadStats(journalusecase.ReadStats{
		TotalFilesSeen: stats.TotalFilesSeen,
		LoadedEntries:  stats.LoadedEntries,
		SkippedCorrupt: stats.SkippedCorrupt,
	})
}

func finalizeSectionLists(info *Info) {
	info.IncludedSections = nil
	info.OmittedSections = nil
	info.FailedSections = nil

	for _, section := range info.Sections {
		info.Warnings = append(info.Warnings, section.Warnings...)
		switch section.Status {
		case sectionStatusIncluded:
			info.IncludedSections = append(info.IncludedSections, section.Code)
		case sectionStatusOmitted:
			info.OmittedSections = append(info.OmittedSections, section.Code)
		case sectionStatusFailed:
			info.FailedSections = append(info.FailedSections, section.Code)
		}
	}
}

func primaryOverviewFailure(failures []error, failedSections []string) error {
	for _, err := range failures {
		if err == nil {
			continue
		}
		if kind, ok := apperr.KindOf(err); ok && kind != apperr.KindValidation {
			return wrapOverviewFailure(err)
		}
	}
	for _, err := range failures {
		if err != nil {
			return wrapOverviewFailure(err)
		}
	}
	return apperr.Wrap(apperr.KindValidation, "overview_failed", fmt.Errorf("overview found issues in %s", strings.Join(failedSections, ", ")))
}

func wrapOverviewFailure(err error) error {
	if err == nil {
		return nil
	}
	if code, ok := apperr.CodeOf(err); ok && code == "overview_failed" {
		return err
	}
	if kind, ok := apperr.KindOf(err); ok {
		return apperr.Wrap(kind, "overview_failed", err)
	}
	var pathErr *os.PathError
	if errors.As(err, &pathErr) {
		return apperr.Wrap(apperr.KindIO, "overview_failed", err)
	}
	return apperr.Wrap(apperr.KindValidation, "overview_failed", err)
}

func dedupeStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}

	seen := map[string]struct{}{}
	items := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		items = append(items, value)
	}

	return items
}
