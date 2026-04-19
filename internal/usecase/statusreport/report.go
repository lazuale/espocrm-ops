package statusreport

import (
	"errors"
	"fmt"
	fsys "io/fs"
	"os"
	"path/filepath"
	"sort"
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
	"github.com/lazuale/espocrm-ops/internal/usecase/reporting"
)

const (
	sectionContext         = "context"
	sectionDoctor          = "doctor"
	sectionRuntime         = "runtime"
	sectionLatestOperation = "latest_operation"
	sectionArtifacts       = "artifacts"

	sectionStatusIncluded = "included"
	sectionStatusOmitted  = "omitted"
	sectionStatusFailed   = "failed"
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
	ReportsDir       string
	SupportDir       string
	GeneratedAt      string
	IncludedSections []string
	OmittedSections  []string
	FailedSections   []string
	Warnings         []string
	Sections         []Section
}

type Section struct {
	Code            string               `json:"code"`
	Status          string               `json:"status"`
	Summary         string               `json:"summary"`
	Details         string               `json:"details,omitempty"`
	Action          string               `json:"action,omitempty"`
	FailureCode     string               `json:"failure_code,omitempty"`
	Warnings        []string             `json:"warnings,omitempty"`
	Context         *ContextData         `json:"context,omitempty"`
	Doctor          *DoctorData          `json:"doctor,omitempty"`
	Runtime         *RuntimeData         `json:"runtime,omitempty"`
	LatestOperation *LatestOperationData `json:"latest_operation,omitempty"`
	Artifacts       *ArtifactsData       `json:"artifacts,omitempty"`
}

type ContextData struct {
	Contour        string        `json:"contour"`
	ComposeProject string        `json:"compose_project,omitempty"`
	EnvFile        string        `json:"env_file"`
	SiteURL        string        `json:"site_url,omitempty"`
	WSPublicURL    string        `json:"ws_public_url,omitempty"`
	EspoCRMImage   string        `json:"espocrm_image,omitempty"`
	Retention      RetentionData `json:"retention"`
	Storage        StorageData   `json:"storage"`
}

type RetentionData struct {
	BackupDays  *int `json:"backup_days,omitempty"`
	ReportDays  int  `json:"report_days"`
	SupportDays int  `json:"support_days"`
}

type StorageData struct {
	DB         PathData `json:"db"`
	Espo       PathData `json:"espo"`
	BackupRoot PathData `json:"backup_root"`
	ReportsDir PathData `json:"reports_dir"`
	SupportDir PathData `json:"support_dir"`
}

type PathData struct {
	Path      string `json:"path"`
	Exists    bool   `json:"exists"`
	SizeBytes int64  `json:"size_bytes"`
	SizeHuman string `json:"size_human"`
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

type LatestOperationData struct {
	Returned       int                              `json:"returned"`
	TotalFilesSeen int                              `json:"total_files_seen"`
	LoadedEntries  int                              `json:"loaded_entries"`
	SkippedCorrupt int                              `json:"skipped_corrupt"`
	Operation      *journalusecase.OperationSummary `json:"operation,omitempty"`
}

type ArtifactsData struct {
	VerifyChecksum      bool                       `json:"verify_checksum"`
	TotalBackupSets     int                        `json:"total_backup_sets"`
	LatestReadyBackup   *backupusecase.CatalogItem `json:"latest_ready_backup,omitempty"`
	LatestDBBackup      *ArtifactFile              `json:"latest_db_backup,omitempty"`
	LatestFilesBackup   *ArtifactFile              `json:"latest_files_backup,omitempty"`
	LatestManifestJSON  *ArtifactFile              `json:"latest_manifest_json,omitempty"`
	LatestManifestTXT   *ArtifactFile              `json:"latest_manifest_txt,omitempty"`
	LatestReportTXT     *ArtifactFile              `json:"latest_report_txt,omitempty"`
	LatestReportJSON    *ArtifactFile              `json:"latest_report_json,omitempty"`
	LatestSupportBundle *ArtifactFile              `json:"latest_support_bundle,omitempty"`
}

type ArtifactFile struct {
	Path       string `json:"path"`
	SizeBytes  int64  `json:"size_bytes"`
	ModifiedAt string `json:"modified_at"`
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

	env, envErr := config.LoadOperationEnv(
		info.ProjectDir,
		info.Scope,
		strings.TrimSpace(req.EnvFileOverride),
		strings.TrimSpace(req.EnvContourHint),
	)
	if envErr == nil {
		info.EnvFile = env.FilePath
		info.BackupRoot = config.ResolveProjectPath(info.ProjectDir, env.BackupRoot())
		info.ReportsDir = filepath.Join(info.BackupRoot, "reports")
		info.SupportDir = filepath.Join(info.BackupRoot, "support")
	}

	contextSection, contextErr := buildContextSection(info.ProjectDir, env, envErr)
	info.Sections = append(info.Sections, contextSection)
	if contextErr != nil {
		failedErrs = append(failedErrs, contextErr)
	}

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

	runtimeSection, runtimeErr := buildRuntimeSection(info.ProjectDir, info.ComposeFile, env, envErr)
	info.Sections = append(info.Sections, runtimeSection)
	if runtimeErr != nil {
		failedErrs = append(failedErrs, runtimeErr)
	}

	latestOperationSection := buildLatestOperationSection(strings.TrimSpace(req.JournalDir), info.Scope)
	info.Sections = append(info.Sections, latestOperationSection)

	artifactsSection, artifactsErr := buildArtifactsSection(info.BackupRoot, info.ReportsDir, info.SupportDir, env, envErr, strings.TrimSpace(req.JournalDir), now)
	info.Sections = append(info.Sections, artifactsSection)
	if artifactsErr != nil {
		failedErrs = append(failedErrs, artifactsErr)
	}

	finalizeSectionLists(&info)

	if len(info.FailedSections) == 0 {
		return info, nil
	}

	return info, primaryStatusReportFailure(failedErrs, info.FailedSections)
}

func buildContextSection(projectDir string, env config.OperationEnv, envErr error) (Section, error) {
	section := Section{
		Code:    sectionContext,
		Status:  sectionStatusIncluded,
		Summary: "Resolved contour env and storage context",
	}

	if envErr != nil {
		section.Status = sectionStatusFailed
		section.Summary = "Contour context unavailable"
		section.Details = envErr.Error()
		section.Action = "Resolve the contour env file before rerunning status-report."
		section.FailureCode = "context_env_resolution_failed"
		return section, apperr.Wrap(apperr.KindValidation, "context_env_resolution_failed", envErr)
	}

	backupRetention, err := optionalIntEnv(env, "BACKUP_RETENTION_DAYS", nil)
	if err != nil {
		section.Status = sectionStatusFailed
		section.Summary = "Contour context unavailable"
		section.Details = err.Error()
		section.Action = "Set BACKUP_RETENTION_DAYS to a non-negative integer before rerunning status-report."
		section.FailureCode = "context_retention_invalid"
		return section, apperr.Wrap(apperr.KindValidation, "context_retention_invalid", err)
	}
	reportRetention, err := optionalIntEnv(env, "REPORT_RETENTION_DAYS", intPtr(30))
	if err != nil {
		section.Status = sectionStatusFailed
		section.Summary = "Contour context unavailable"
		section.Details = err.Error()
		section.Action = "Set REPORT_RETENTION_DAYS to a non-negative integer before rerunning status-report."
		section.FailureCode = "context_retention_invalid"
		return section, apperr.Wrap(apperr.KindValidation, "context_retention_invalid", err)
	}
	supportRetention, err := optionalIntEnv(env, "SUPPORT_RETENTION_DAYS", intPtr(14))
	if err != nil {
		section.Status = sectionStatusFailed
		section.Summary = "Contour context unavailable"
		section.Details = err.Error()
		section.Action = "Set SUPPORT_RETENTION_DAYS to a non-negative integer before rerunning status-report."
		section.FailureCode = "context_retention_invalid"
		return section, apperr.Wrap(apperr.KindValidation, "context_retention_invalid", err)
	}

	backupRoot := config.ResolveProjectPath(projectDir, env.BackupRoot())
	reportsDir := filepath.Join(backupRoot, "reports")
	supportDir := filepath.Join(backupRoot, "support")

	dbPath, err := buildPathData(config.ResolveProjectPath(projectDir, env.DBStorageDir()))
	if err != nil {
		return failedContextPathSection(section, "db storage", err)
	}
	espoPath, err := buildPathData(config.ResolveProjectPath(projectDir, env.ESPOStorageDir()))
	if err != nil {
		return failedContextPathSection(section, "EspoCRM storage", err)
	}
	backupPath, err := buildPathData(backupRoot)
	if err != nil {
		return failedContextPathSection(section, "backup root", err)
	}
	reportsPath, err := buildPathData(reportsDir)
	if err != nil {
		return failedContextPathSection(section, "reports directory", err)
	}
	supportPath, err := buildPathData(supportDir)
	if err != nil {
		return failedContextPathSection(section, "support directory", err)
	}

	section.Context = &ContextData{
		Contour:        env.ResolvedContour,
		ComposeProject: env.ComposeProject(),
		EnvFile:        env.FilePath,
		SiteURL:        env.Value("SITE_URL"),
		WSPublicURL:    env.Value("WS_PUBLIC_URL"),
		EspoCRMImage:   env.Value("ESPOCRM_IMAGE"),
		Retention: RetentionData{
			BackupDays:  backupRetention,
			ReportDays:  derefInt(reportRetention),
			SupportDays: derefInt(supportRetention),
		},
		Storage: StorageData{
			DB:         dbPath,
			Espo:       espoPath,
			BackupRoot: backupPath,
			ReportsDir: reportsPath,
			SupportDir: supportPath,
		},
	}
	section.Details = fmt.Sprintf("Using %s with compose project %s.", env.FilePath, env.ComposeProject())

	return section, nil
}

func failedContextPathSection(section Section, label string, err error) (Section, error) {
	section.Status = sectionStatusFailed
	section.Summary = "Contour context unavailable"
	section.Details = err.Error()
	section.Action = fmt.Sprintf("Restore %s access before rerunning status-report.", label)
	section.FailureCode = "context_path_inspection_failed"
	return section, apperr.Wrap(apperr.KindIO, "context_path_inspection_failed", err)
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

func buildRuntimeSection(projectDir, composeFile string, env config.OperationEnv, envErr error) (Section, error) {
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
		section.Action = "Resolve env resolution before rerunning status-report to inspect runtime state."
		section.Warnings = append(section.Warnings, fmt.Sprintf("runtime omitted: %v", envErr))
		section.Warnings = reporting.DedupeStrings(section.Warnings)
		return section, nil
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
	section.Runtime = &RuntimeData{
		ComposeProject:      env.ComposeProject(),
		EnvFile:             env.FilePath,
		SiteURL:             env.Value("SITE_URL"),
		WSPublicURL:         env.Value("WS_PUBLIC_URL"),
		SharedOperationLock: shared,
		MaintenanceLock:     maintenance,
	}
	if runtimeErr != nil {
		section.Status = sectionStatusFailed
		section.Summary = "Runtime status unavailable"
		section.Details = runtimeErr.Error()
		section.Action = "Restore Docker and Docker Compose access before rerunning status-report."
		section.FailureCode = "runtime_collection_failed"
		section.Warnings = reporting.DedupeStrings(section.Warnings)
		return section, apperr.Wrap(apperr.KindValidation, "runtime_collection_failed", runtimeErr)
	}

	section.Runtime.Services = services
	section.Details = runtimeDetails(services)
	section.Warnings = append(section.Warnings, runtimeServiceWarnings(services)...)
	section.Warnings = reporting.DedupeStrings(section.Warnings)

	return section, nil
}

func buildLatestOperationSection(journalDir, scope string) Section {
	section := Section{
		Code: sectionLatestOperation,
	}

	history, err := journalusecase.History(journalusecase.HistoryInput{
		JournalDir: journalDir,
		Filters: journalusecase.Filters{
			Scope: scope,
			Limit: 1,
		},
	})
	if err != nil {
		section.Status = sectionStatusOmitted
		section.Summary = "Latest operation omitted"
		section.Details = err.Error()
		section.Action = "Repair journal access before rerunning status-report to inspect the latest operation."
		section.Warnings = []string{fmt.Sprintf("latest operation omitted: %v", err)}
		return section
	}

	section.Status = sectionStatusIncluded
	section.Details = fmt.Sprintf(
		"Scanned %d journal file(s), loaded %d entries, and skipped %d corrupt entries.",
		history.Stats.TotalFilesSeen,
		history.Stats.LoadedEntries,
		history.Stats.SkippedCorrupt,
	)
	section.Warnings = journalusecase.WarningsFromReadStats(history.Stats)
	section.LatestOperation = &LatestOperationData{
		Returned:       len(history.Operations),
		TotalFilesSeen: history.Stats.TotalFilesSeen,
		LoadedEntries:  history.Stats.LoadedEntries,
		SkippedCorrupt: history.Stats.SkippedCorrupt,
	}

	if len(history.Operations) == 0 {
		section.Summary = "No recorded operations"
		return section
	}

	operation := history.Operations[0]
	section.Summary = fmt.Sprintf("Latest operation: %s (%s)", operation.Command, operation.Status)
	section.LatestOperation.Operation = &operation
	return section
}

func buildArtifactsSection(backupRoot, reportsDir, supportDir string, env config.OperationEnv, envErr error, journalDir string, now time.Time) (Section, error) {
	section := Section{
		Code:    sectionArtifacts,
		Status:  sectionStatusIncluded,
		Summary: "Artifact summary collected",
		Artifacts: &ArtifactsData{
			VerifyChecksum: true,
		},
	}

	if envErr != nil {
		section.Status = sectionStatusOmitted
		section.Summary = "Artifact summary omitted"
		section.Details = envErr.Error()
		section.Action = "Resolve env resolution before rerunning status-report to inspect backup, report, and support artifacts."
		section.Warnings = []string{fmt.Sprintf("artifacts omitted: %v", envErr)}
		return section, nil
	}

	var err error
	section.Artifacts.LatestDBBackup, err = latestFileSummary(filepath.Join(backupRoot, "db"), "*.sql.gz")
	if err != nil {
		return failedArtifactsSection(section, err)
	}
	section.Artifacts.LatestFilesBackup, err = latestFileSummary(filepath.Join(backupRoot, "files"), "*.tar.gz")
	if err != nil {
		return failedArtifactsSection(section, err)
	}
	section.Artifacts.LatestManifestJSON, err = latestFileSummary(filepath.Join(backupRoot, "manifests"), "*.manifest.json")
	if err != nil {
		return failedArtifactsSection(section, err)
	}
	section.Artifacts.LatestManifestTXT, err = latestFileSummary(filepath.Join(backupRoot, "manifests"), "*.manifest.txt")
	if err != nil {
		return failedArtifactsSection(section, err)
	}
	section.Artifacts.LatestReportTXT, err = latestFileSummary(reportsDir, "*.txt")
	if err != nil {
		return failedArtifactsSection(section, err)
	}
	section.Artifacts.LatestReportJSON, err = latestFileSummary(reportsDir, "*.json")
	if err != nil {
		return failedArtifactsSection(section, err)
	}
	section.Artifacts.LatestSupportBundle, err = latestFileSummary(supportDir, "*.tar.gz")
	if err != nil {
		return failedArtifactsSection(section, err)
	}

	catalog, catalogErr := backupusecase.Catalog(backupusecase.CatalogRequest{
		BackupRoot:     backupRoot,
		JournalDir:     journalDir,
		VerifyChecksum: true,
		ReadyOnly:      true,
		Limit:          1,
		Now:            now,
	})
	if catalogErr != nil {
		return failedArtifactsSection(section, catalogErr)
	}
	section.Artifacts.TotalBackupSets = catalog.TotalSets
	section.Warnings = append(section.Warnings, backupJournalWarnings(catalog.JournalRead)...)
	if len(catalog.Items) > 0 {
		item := catalog.Items[0]
		section.Artifacts.LatestReadyBackup = &item
	}

	found := 0
	for _, item := range []*ArtifactFile{
		section.Artifacts.LatestDBBackup,
		section.Artifacts.LatestFilesBackup,
		section.Artifacts.LatestManifestJSON,
		section.Artifacts.LatestManifestTXT,
		section.Artifacts.LatestReportTXT,
		section.Artifacts.LatestReportJSON,
		section.Artifacts.LatestSupportBundle,
	} {
		if item != nil {
			found++
		}
	}
	if section.Artifacts.LatestReadyBackup == nil && found == 0 {
		section.Summary = "No backup, report, or support artifacts found"
		section.Details = "The resolved backup, report, and support directories do not currently contain recorded artifacts."
		section.Warnings = reporting.DedupeStrings(section.Warnings)
		return section, nil
	}

	section.Details = fmt.Sprintf(
		"Detected %d latest artifact file(s) and %d restore-ready backup set(s).",
		found,
		minInt(1, len(catalog.Items)),
	)
	section.Warnings = reporting.DedupeStrings(section.Warnings)
	return section, nil
}

func failedArtifactsSection(section Section, err error) (Section, error) {
	section.Status = sectionStatusFailed
	section.Summary = "Artifact summary unavailable"
	section.Details = err.Error()
	section.Action = "Repair backup, report, or support artifact access before rerunning status-report."
	section.FailureCode = "artifacts_collection_failed"
	section.Warnings = reporting.DedupeStrings(section.Warnings)
	return section, apperr.Wrap(apperr.KindIO, "artifacts_collection_failed", err)
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

func buildPathData(path string) (PathData, error) {
	cleanPath := filepath.Clean(path)
	item := PathData{
		Path:      cleanPath,
		SizeHuman: "0 B",
	}

	info, err := os.Stat(cleanPath)
	if err != nil {
		if os.IsNotExist(err) {
			return item, nil
		}
		return item, fmt.Errorf("stat %s: %w", cleanPath, err)
	}

	size, err := pathSizeBytes(cleanPath, info)
	if err != nil {
		return item, err
	}

	item.Exists = true
	item.SizeBytes = size
	item.SizeHuman = humanSize(size)
	return item, nil
}

func pathSizeBytes(path string, info os.FileInfo) (int64, error) {
	if !info.IsDir() {
		return info.Size(), nil
	}

	var total int64
	err := filepath.WalkDir(path, func(current string, entry fsys.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		entryInfo, err := entry.Info()
		if err != nil {
			return err
		}
		total += entryInfo.Size()
		return nil
	})
	if err != nil {
		return 0, fmt.Errorf("walk %s: %w", path, err)
	}

	return total, nil
}

func humanSize(size int64) string {
	const unit = 1024
	if size < unit {
		return fmt.Sprintf("%d B", size)
	}

	value := float64(size)
	suffixes := []string{"KiB", "MiB", "GiB", "TiB"}
	for _, suffix := range suffixes {
		value /= unit
		if value < unit {
			return fmt.Sprintf("%.1f %s", value, suffix)
		}
	}

	return fmt.Sprintf("%.1f PiB", value/unit)
}

func latestFileSummary(dir, pattern string) (*ArtifactFile, error) {
	path, err := latestFileInDir(dir, pattern)
	if err != nil || path == "" {
		return nil, err
	}

	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("stat %s: %w", path, err)
	}

	return &ArtifactFile{
		Path:       path,
		SizeBytes:  info.Size(),
		ModifiedAt: info.ModTime().UTC().Format(time.RFC3339),
	}, nil
}

func latestFileInDir(dir, pattern string) (string, error) {
	matches, err := filepath.Glob(filepath.Join(dir, pattern))
	if err != nil {
		return "", fmt.Errorf("glob %s: %w", filepath.Join(dir, pattern), err)
	}
	if len(matches) == 0 {
		return "", nil
	}

	sort.Slice(matches, func(i, j int) bool {
		iInfo, iErr := os.Stat(matches[i])
		jInfo, jErr := os.Stat(matches[j])
		switch {
		case iErr != nil && jErr != nil:
			return matches[i] > matches[j]
		case iErr != nil:
			return false
		case jErr != nil:
			return true
		case iInfo.ModTime().Equal(jInfo.ModTime()):
			return matches[i] > matches[j]
		default:
			return iInfo.ModTime().After(jInfo.ModTime())
		}
	})

	return matches[0], nil
}

func optionalIntEnv(env config.OperationEnv, key string, fallback *int) (*int, error) {
	value := strings.TrimSpace(env.Value(key))
	if value == "" {
		if fallback == nil {
			return nil, nil
		}
		return intPtr(*fallback), nil
	}

	parsed, err := strconv.Atoi(value)
	if err != nil || parsed < 0 {
		return nil, fmt.Errorf("%s must be a non-negative integer in %s", key, env.FilePath)
	}
	return intPtr(parsed), nil
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

func backupJournalWarnings(stats backupusecase.JournalReadStats) []string {
	return journalusecase.WarningsFromReadStats(journalusecase.ReadStats{
		TotalFilesSeen: stats.TotalFilesSeen,
		LoadedEntries:  stats.LoadedEntries,
		SkippedCorrupt: stats.SkippedCorrupt,
	})
}

func finalizeSectionLists(info *Info) {
	collector := reporting.SectionCollector{}
	for _, section := range info.Sections {
		collector.Add(sectionCategory(section.Status), section.Code, section.Warnings)
	}
	summary := collector.Finalize(info.Warnings)
	info.IncludedSections = summary.IncludedSections
	info.OmittedSections = summary.OmittedSections
	info.FailedSections = summary.FailedSections
	info.Warnings = summary.Warnings
}

func sectionCategory(status string) reporting.SectionCategory {
	switch status {
	case sectionStatusIncluded:
		return reporting.SectionIncluded
	case sectionStatusOmitted:
		return reporting.SectionOmitted
	case sectionStatusFailed:
		return reporting.SectionFailed
	default:
		return reporting.SectionIgnored
	}
}

func primaryStatusReportFailure(failures []error, failedSections []string) error {
	for _, err := range failures {
		if err == nil {
			continue
		}
		if kind, ok := apperr.KindOf(err); ok && kind != apperr.KindValidation {
			return wrapStatusReportFailure(err)
		}
	}
	for _, err := range failures {
		if err != nil {
			return wrapStatusReportFailure(err)
		}
	}
	return apperr.Wrap(apperr.KindValidation, "status_report_failed", fmt.Errorf("status report found issues in %s", strings.Join(failedSections, ", ")))
}

func wrapStatusReportFailure(err error) error {
	if err == nil {
		return nil
	}
	if code, ok := apperr.CodeOf(err); ok && code == "status_report_failed" {
		return err
	}
	if kind, ok := apperr.KindOf(err); ok {
		return apperr.Wrap(kind, "status_report_failed", err)
	}
	var pathErr *os.PathError
	if errors.As(err, &pathErr) {
		return apperr.Wrap(apperr.KindIO, "status_report_failed", err)
	}
	return apperr.Wrap(apperr.KindValidation, "status_report_failed", err)
}

func intPtr(value int) *int {
	return &value
}

func derefInt(value *int) int {
	if value == nil {
		return 0
	}
	return *value
}

func minInt(left, right int) int {
	if left < right {
		return left
	}
	return right
}
