package maintenance

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
	platformconfig "github.com/lazuale/espocrm-ops/internal/platform/config"
	journalusecase "github.com/lazuale/espocrm-ops/internal/usecase/journal"
)

const (
	sectionContext      = "context"
	sectionJournal      = "journal"
	sectionReports      = "reports"
	sectionSupport      = "support"
	sectionRestoreDrill = "restore_drill"

	sectionStatusIncluded = "included"
	sectionStatusOmitted  = "omitted"
	sectionStatusFailed   = "failed"

	defaultReportRetentionDays       = 30
	defaultSupportRetentionDays      = 14
	defaultRestoreDrillRetentionDays = 7
)

type Request struct {
	Scope                     string
	ProjectDir                string
	ComposeFile               string
	EnvFileOverride           string
	EnvContourHint            string
	JournalDir                string
	Now                       time.Time
	Apply                     bool
	JournalKeepDays           int
	JournalKeepLast           int
	ReportRetentionDays       *int
	SupportRetentionDays      *int
	RestoreDrillRetentionDays *int
}

type Info struct {
	Scope                  string
	ProjectDir             string
	ComposeFile            string
	EnvFile                string
	JournalDir             string
	BackupRoot             string
	ReportsDir             string
	SupportDir             string
	RestoreDrillEnvDir     string
	RestoreDrillStorageDir string
	RestoreDrillBackupDir  string
	GeneratedAt            string
	DryRun                 bool
	IncludedSections       []string
	OmittedSections        []string
	FailedSections         []string
	Warnings               []string
	Sections               []Section
}

type Section struct {
	Code        string       `json:"code"`
	Status      string       `json:"status"`
	Summary     string       `json:"summary"`
	Details     string       `json:"details,omitempty"`
	Action      string       `json:"action,omitempty"`
	FailureCode string       `json:"failure_code,omitempty"`
	Warnings    []string     `json:"warnings,omitempty"`
	Context     *ContextData `json:"context,omitempty"`
	Cleanup     *CleanupData `json:"cleanup,omitempty"`
}

type ContextData struct {
	Contour                string `json:"contour"`
	ComposeProject         string `json:"compose_project,omitempty"`
	EnvFile                string `json:"env_file"`
	BackupRoot             string `json:"backup_root"`
	ReportsDir             string `json:"reports_dir"`
	SupportDir             string `json:"support_dir"`
	RestoreDrillEnvDir     string `json:"restore_drill_env_dir"`
	RestoreDrillStorageDir string `json:"restore_drill_storage_dir"`
	RestoreDrillBackupDir  string `json:"restore_drill_backup_dir"`
	Mode                   string `json:"mode"`
}

type CleanupData struct {
	DryRun            bool          `json:"dry_run"`
	KeepDays          *int          `json:"keep_days,omitempty"`
	KeepLast          *int          `json:"keep_last,omitempty"`
	RetentionDays     *int          `json:"retention_days,omitempty"`
	Checked           int           `json:"checked"`
	Kept              int           `json:"kept"`
	Protected         int           `json:"protected"`
	Removed           int           `json:"removed"`
	Failed            int           `json:"failed,omitempty"`
	TotalFilesSeen    int           `json:"total_files_seen,omitempty"`
	LoadedEntries     int           `json:"loaded_entries,omitempty"`
	SkippedCorrupt    int           `json:"skipped_corrupt,omitempty"`
	LatestOperationID string        `json:"latest_operation_id,omitempty"`
	Items             []CleanupItem `json:"items,omitempty"`
}

type CleanupItem struct {
	Kind       string                           `json:"kind"`
	Decision   string                           `json:"decision"`
	Path       string                           `json:"path"`
	Reasons    []string                         `json:"reasons,omitempty"`
	ModifiedAt string                           `json:"modified_at,omitempty"`
	SizeBytes  int64                            `json:"size_bytes,omitempty"`
	Operation  *journalusecase.OperationSummary `json:"operation,omitempty"`
}

type cleanupCandidate struct {
	Path         string
	Kind         string
	ProtectGroup string
	ModifiedAt   time.Time
	SizeBytes    int64
	Remove       func() error
}

type cleanupSectionConfig struct {
	Code          string
	Label         string
	EmptySummary  string
	EmptyDetails  string
	Action        string
	FailureCode   string
	RetentionDays int
	DryRun        bool
	Candidates    []cleanupCandidate
}

type cleanupPattern struct {
	Pattern      string
	Kind         string
	ProtectGroup string
}

func Run(req Request) (Info, error) {
	now := req.Now.UTC()
	if req.Now.IsZero() {
		now = time.Now().UTC()
	}

	info := Info{
		Scope:       strings.TrimSpace(req.Scope),
		ProjectDir:  filepath.Clean(req.ProjectDir),
		ComposeFile: filepath.Clean(req.ComposeFile),
		JournalDir:  filepath.Clean(strings.TrimSpace(req.JournalDir)),
		GeneratedAt: now.Format(time.RFC3339),
		DryRun:      !req.Apply,
	}

	failedErrs := []error{}

	env, envErr := platformconfig.LoadOperationEnv(
		info.ProjectDir,
		info.Scope,
		strings.TrimSpace(req.EnvFileOverride),
		strings.TrimSpace(req.EnvContourHint),
	)
	if envErr != nil {
		info.Sections = append(info.Sections, failedContextEnvSection(envErr, info.DryRun))
		info.Sections = append(info.Sections,
			omittedPreflightSection(sectionJournal, "Journal cleanup omitted", envErr),
			omittedPreflightSection(sectionReports, "Report cleanup omitted", envErr),
			omittedPreflightSection(sectionSupport, "Support bundle cleanup omitted", envErr),
			omittedPreflightSection(sectionRestoreDrill, "Restore-drill cleanup omitted", envErr),
		)
		finalizeMaintenanceInfo(&info)
		return info, primaryMaintenanceFailure(
			[]error{apperr.Wrap(apperr.KindValidation, "context_env_resolution_failed", envErr)},
			info.FailedSections,
		)
	}

	info.Scope = env.ResolvedContour
	info.EnvFile = env.FilePath
	info.BackupRoot = platformconfig.ResolveProjectPath(info.ProjectDir, env.BackupRoot())
	info.ReportsDir = filepath.Join(info.BackupRoot, "reports")
	info.SupportDir = filepath.Join(info.BackupRoot, "support")
	info.RestoreDrillEnvDir = filepath.Join(info.ProjectDir, ".cache", "env")
	info.RestoreDrillStorageDir = filepath.Join(info.ProjectDir, "storage", "restore-drill", info.Scope)
	info.RestoreDrillBackupDir = filepath.Join(info.ProjectDir, "backups", "restore-drill", info.Scope)

	contextSection := buildContextSection(env, info)
	info.Sections = append(info.Sections, contextSection)

	ctx, prepErr := PrepareOperation(OperationContextRequest{
		Scope:           info.Scope,
		Operation:       "maintenance",
		ProjectDir:      info.ProjectDir,
		EnvFileOverride: strings.TrimSpace(req.EnvFileOverride),
		EnvContourHint:  strings.TrimSpace(req.EnvContourHint),
	})
	if prepErr != nil {
		info.Sections[0] = failedContextPreflightSection(info.Sections[0], prepErr)
		info.Sections = append(info.Sections,
			omittedPreflightSection(sectionJournal, "Journal cleanup omitted", prepErr),
			omittedPreflightSection(sectionReports, "Report cleanup omitted", prepErr),
			omittedPreflightSection(sectionSupport, "Support bundle cleanup omitted", prepErr),
			omittedPreflightSection(sectionRestoreDrill, "Restore-drill cleanup omitted", prepErr),
		)
		finalizeMaintenanceInfo(&info)
		return info, primaryMaintenanceFailure([]error{prepErr}, info.FailedSections)
	}

	journalSection, journalErr := buildJournalSection(info.JournalDir, req.JournalKeepDays, req.JournalKeepLast, info.DryRun)
	info.Sections = append(info.Sections, journalSection)
	if journalErr != nil {
		failedErrs = append(failedErrs, journalErr)
	}

	reportSection, reportErr := buildReportsSection(info, env, req.ReportRetentionDays, now)
	info.Sections = append(info.Sections, reportSection)
	if reportErr != nil {
		failedErrs = append(failedErrs, reportErr)
	}

	supportSection, supportErr := buildSupportSection(info, env, req.SupportRetentionDays, now)
	info.Sections = append(info.Sections, supportSection)
	if supportErr != nil {
		failedErrs = append(failedErrs, supportErr)
	}

	restoreDrillSection, restoreDrillErr := buildRestoreDrillSection(info, req.RestoreDrillRetentionDays, now)
	info.Sections = append(info.Sections, restoreDrillSection)
	if restoreDrillErr != nil {
		failedErrs = append(failedErrs, restoreDrillErr)
	}

	if releaseErr := ctx.Release(); releaseErr != nil {
		info.Warnings = append(info.Warnings, fmt.Sprintf("maintenance release warning: %v", releaseErr))
	}

	finalizeMaintenanceInfo(&info)
	if len(info.FailedSections) == 0 {
		return info, nil
	}

	return info, primaryMaintenanceFailure(failedErrs, info.FailedSections)
}

func buildContextSection(env platformconfig.OperationEnv, info Info) Section {
	return Section{
		Code:    sectionContext,
		Status:  sectionStatusIncluded,
		Summary: "Resolved maintenance context",
		Details: fmt.Sprintf("Using %s with compose project %s in %s mode.", env.FilePath, env.ComposeProject(), maintenanceMode(info.DryRun)),
		Action:  "Use history, status-report, or show-operation when you need deeper detail about retained artifacts.",
		Context: &ContextData{
			Contour:                env.ResolvedContour,
			ComposeProject:         env.ComposeProject(),
			EnvFile:                env.FilePath,
			BackupRoot:             info.BackupRoot,
			ReportsDir:             info.ReportsDir,
			SupportDir:             info.SupportDir,
			RestoreDrillEnvDir:     info.RestoreDrillEnvDir,
			RestoreDrillStorageDir: info.RestoreDrillStorageDir,
			RestoreDrillBackupDir:  info.RestoreDrillBackupDir,
			Mode:                   maintenanceMode(info.DryRun),
		},
	}
}

func failedContextEnvSection(err error, dryRun bool) Section {
	return Section{
		Code:        sectionContext,
		Status:      sectionStatusFailed,
		Summary:     "Maintenance context unavailable",
		Details:     err.Error(),
		Action:      "Resolve the contour env file before rerunning maintenance.",
		FailureCode: "context_env_resolution_failed",
		Context: &ContextData{
			Mode: maintenanceMode(dryRun),
		},
	}
}

func failedContextPreflightSection(section Section, err error) Section {
	section.Status = sectionStatusFailed
	section.Summary = "Maintenance preflight failed"
	section.Details = err.Error()
	section.Action = "Resolve maintenance lock, shared operation lock, or runtime filesystem readiness before rerunning maintenance."
	section.FailureCode = maintenanceFailureCode(err, "maintenance_preflight_failed")
	return section
}

func omittedPreflightSection(code, summary string, err error) Section {
	return Section{
		Code:    code,
		Status:  sectionStatusOmitted,
		Summary: summary,
		Details: err.Error(),
		Action:  "Resolve the failed maintenance preflight before rerunning maintenance.",
	}
}

func buildJournalSection(journalDir string, keepDays, keepLast int, dryRun bool) (Section, error) {
	prune, err := journalusecase.Prune(journalusecase.PruneInput{
		JournalDir: journalDir,
		KeepDays:   keepDays,
		KeepLast:   keepLast,
		DryRun:     dryRun,
	})

	items := make([]CleanupItem, 0, len(prune.Items))
	failed := 0
	for _, item := range prune.Items {
		decision := item.Decision
		if prune.FailedPath != "" && item.Path == prune.FailedPath {
			decision = "failed"
			failed++
		}

		cleanupItem := CleanupItem{
			Kind:     journalItemKind(item.Kind),
			Decision: decision,
			Path:     item.Path,
			Reasons:  append([]string(nil), item.Reasons...),
		}
		if item.Operation != nil {
			operation := *item.Operation
			cleanupItem.Operation = &operation
		}
		items = append(items, cleanupItem)
	}

	kept := prune.Retained - prune.Protected
	if kept < 0 {
		kept = 0
	}

	section := Section{
		Code:     sectionJournal,
		Status:   sectionStatusIncluded,
		Summary:  maintenanceActionSummary("Journal retention", dryRun),
		Details:  cleanupCountsText("journal entries", kept, prune.Protected, prune.Deleted, failed, dryRun),
		Action:   "Use history or show-operation to inspect journal entries before pruning more aggressively.",
		Warnings: dedupeStrings(journalusecase.WarningsFromReadStats(prune.ReadStats)),
		Cleanup: &CleanupData{
			DryRun:            dryRun,
			KeepDays:          intPtr(keepDays),
			KeepLast:          intPtr(keepLast),
			Checked:           prune.Checked,
			Kept:              kept,
			Protected:         prune.Protected,
			Removed:           prune.Deleted,
			Failed:            failed,
			TotalFilesSeen:    prune.ReadStats.TotalFilesSeen,
			LoadedEntries:     prune.ReadStats.LoadedEntries,
			SkippedCorrupt:    prune.ReadStats.SkippedCorrupt,
			LatestOperationID: prune.LatestOperationID,
			Items:             items,
		},
	}

	if err == nil {
		if prune.Checked == 0 {
			section.Summary = "No journal entries required cleanup"
			section.Details = "The resolved journal directory does not currently contain recorded operations."
		}
		return section, nil
	}

	section.Status = sectionStatusFailed
	section.Summary = "Journal cleanup failed"
	section.Details = cleanupCountsText("journal entries", kept, prune.Protected, prune.Deleted, failed, dryRun)
	if prune.FailedPath != "" {
		section.Details = fmt.Sprintf("%s First failure: %s.", section.Details, prune.FailedPath)
	}
	section.Action = "Repair journal access or prune lock conflicts before rerunning maintenance."
	section.FailureCode = maintenanceFailureCode(err, "journal_cleanup_failed")
	return section, err
}

func buildReportsSection(info Info, env platformconfig.OperationEnv, override *int, now time.Time) (Section, error) {
	retentionDays, err := resolveEnvRetentionDays(env, "REPORT_RETENTION_DAYS", defaultReportRetentionDays, override)
	if err != nil {
		return failedCleanupConfigSection(
			sectionReports,
			"Report cleanup failed",
			err,
			"Set REPORT_RETENTION_DAYS to a non-negative integer before rerunning maintenance.",
			"reports_retention_invalid",
			info.DryRun,
			retentionDays,
		), apperr.Wrap(apperr.KindValidation, "reports_retention_invalid", err)
	}

	candidates, err := collectGlobCleanupCandidates(info.ReportsDir, []cleanupPattern{
		{Pattern: "*.txt", Kind: "report_txt", ProtectGroup: "report_txt"},
		{Pattern: "*.json", Kind: "report_json", ProtectGroup: "report_json"},
	})
	if err != nil {
		return failedCleanupSection(
			sectionReports,
			"Report cleanup failed",
			err,
			"Repair report directory access before rerunning maintenance.",
			"reports_cleanup_failed",
			info.DryRun,
			retentionDays,
		), apperr.Wrap(apperr.KindIO, "reports_cleanup_failed", err)
	}

	return executeCleanupSection(cleanupSectionConfig{
		Code:          sectionReports,
		Label:         "Report cleanup",
		EmptySummary:  "No report artifacts found",
		EmptyDetails:  fmt.Sprintf("No report artifacts were found under %s.", info.ReportsDir),
		Action:        "Use status-report to inspect the newest report artifacts before tightening retention further.",
		FailureCode:   "reports_cleanup_failed",
		RetentionDays: retentionDays,
		DryRun:        info.DryRun,
		Candidates:    candidates,
	}, now)
}

func buildSupportSection(info Info, env platformconfig.OperationEnv, override *int, now time.Time) (Section, error) {
	retentionDays, err := resolveEnvRetentionDays(env, "SUPPORT_RETENTION_DAYS", defaultSupportRetentionDays, override)
	if err != nil {
		return failedCleanupConfigSection(
			sectionSupport,
			"Support bundle cleanup failed",
			err,
			"Set SUPPORT_RETENTION_DAYS to a non-negative integer before rerunning maintenance.",
			"support_retention_invalid",
			info.DryRun,
			retentionDays,
		), apperr.Wrap(apperr.KindValidation, "support_retention_invalid", err)
	}

	candidates, err := collectGlobCleanupCandidates(info.SupportDir, []cleanupPattern{
		{Pattern: "*.tar.gz", Kind: "support_bundle", ProtectGroup: "support_bundle"},
	})
	if err != nil {
		return failedCleanupSection(
			sectionSupport,
			"Support bundle cleanup failed",
			err,
			"Repair support bundle directory access before rerunning maintenance.",
			"support_cleanup_failed",
			info.DryRun,
			retentionDays,
		), apperr.Wrap(apperr.KindIO, "support_cleanup_failed", err)
	}

	return executeCleanupSection(cleanupSectionConfig{
		Code:          sectionSupport,
		Label:         "Support bundle cleanup",
		EmptySummary:  "No support bundles found",
		EmptyDetails:  fmt.Sprintf("No support bundles were found under %s.", info.SupportDir),
		Action:        "Use support-bundle to regenerate a fresh archive when you need a new diagnostic bundle.",
		FailureCode:   "support_cleanup_failed",
		RetentionDays: retentionDays,
		DryRun:        info.DryRun,
		Candidates:    candidates,
	}, now)
}

func buildRestoreDrillSection(info Info, override *int, now time.Time) (Section, error) {
	retentionDays, err := resolveStaticRetentionDays(defaultRestoreDrillRetentionDays, override)
	if err != nil {
		return failedCleanupConfigSection(
			sectionRestoreDrill,
			"Restore-drill cleanup failed",
			err,
			"Set --restore-drill-retention-days to a non-negative integer before rerunning maintenance.",
			"restore_drill_retention_invalid",
			info.DryRun,
			retentionDays,
		), apperr.Wrap(apperr.KindValidation, "restore_drill_retention_invalid", err)
	}

	candidates, err := collectRestoreDrillCandidates(info)
	if err != nil {
		return failedCleanupSection(
			sectionRestoreDrill,
			"Restore-drill cleanup failed",
			err,
			"Repair restore-drill artifact access before rerunning maintenance.",
			"restore_drill_cleanup_failed",
			info.DryRun,
			retentionDays,
		), apperr.Wrap(apperr.KindIO, "restore_drill_cleanup_failed", err)
	}

	return executeCleanupSection(cleanupSectionConfig{
		Code:          sectionRestoreDrill,
		Label:         "Restore-drill cleanup",
		EmptySummary:  "No restore-drill artifacts found",
		EmptyDetails:  "No leftover restore-drill env files, temporary storage, or backup roots were found.",
		Action:        "Use restore-drill --keep-artifacts only when you intentionally want to preserve temporary drill state.",
		FailureCode:   "restore_drill_cleanup_failed",
		RetentionDays: retentionDays,
		DryRun:        info.DryRun,
		Candidates:    candidates,
	}, now)
}

func failedCleanupConfigSection(code, summary string, err error, action string, failureCode string, dryRun bool, retentionDays int) Section {
	return Section{
		Code:        code,
		Status:      sectionStatusFailed,
		Summary:     summary,
		Details:     err.Error(),
		Action:      action,
		FailureCode: failureCode,
		Cleanup: &CleanupData{
			DryRun:        dryRun,
			RetentionDays: intPtr(retentionDays),
		},
	}
}

func failedCleanupSection(code, summary string, err error, action string, failureCode string, dryRun bool, retentionDays int) Section {
	return Section{
		Code:        code,
		Status:      sectionStatusFailed,
		Summary:     summary,
		Details:     err.Error(),
		Action:      action,
		FailureCode: failureCode,
		Cleanup: &CleanupData{
			DryRun:        dryRun,
			RetentionDays: intPtr(retentionDays),
		},
	}
}

func executeCleanupSection(cfg cleanupSectionConfig, now time.Time) (Section, error) {
	data := &CleanupData{
		DryRun:        cfg.DryRun,
		RetentionDays: intPtr(cfg.RetentionDays),
	}
	section := Section{
		Code:    cfg.Code,
		Status:  sectionStatusIncluded,
		Summary: maintenanceActionSummary(cfg.Label, cfg.DryRun),
		Action:  cfg.Action,
		Cleanup: data,
	}

	if len(cfg.Candidates) == 0 {
		section.Summary = cfg.EmptySummary
		section.Details = cfg.EmptyDetails
		return section, nil
	}

	protected := protectedCandidatePaths(cfg.Candidates)
	sort.Slice(cfg.Candidates, func(i, j int) bool {
		switch {
		case cfg.Candidates[i].ModifiedAt.Equal(cfg.Candidates[j].ModifiedAt):
			return cfg.Candidates[i].Path < cfg.Candidates[j].Path
		default:
			return cfg.Candidates[i].ModifiedAt.After(cfg.Candidates[j].ModifiedAt)
		}
	})

	cutoff := now.UTC().Add(-time.Duration(cfg.RetentionDays+1) * 24 * time.Hour)
	failures := []string{}
	items := make([]CleanupItem, 0, len(cfg.Candidates))
	for _, candidate := range cfg.Candidates {
		item := CleanupItem{
			Kind:       candidate.Kind,
			Decision:   "keep",
			Path:       candidate.Path,
			ModifiedAt: candidate.ModifiedAt.UTC().Format(time.RFC3339),
			SizeBytes:  candidate.SizeBytes,
		}

		switch {
		case protected[candidate.Path]:
			item.Decision = "protect"
			item.Reasons = []string{"safety_floor"}
			data.Protected++
		case !candidate.ModifiedAt.Before(cutoff):
			item.Decision = "keep"
			item.Reasons = []string{"within_retention"}
			data.Kept++
		default:
			item.Decision = "remove"
			item.Reasons = []string{"older_than_retention"}
			if cfg.DryRun {
				data.Removed++
			} else if err := candidate.Remove(); err != nil {
				item.Decision = "failed"
				item.Reasons = append(item.Reasons, err.Error())
				data.Failed++
				failures = append(failures, err.Error())
			} else {
				data.Removed++
			}
		}

		items = append(items, item)
	}

	data.Checked = len(cfg.Candidates)
	data.Items = items
	section.Details = cleanupCountsText("artifacts", data.Kept, data.Protected, data.Removed, data.Failed, cfg.DryRun)
	if len(failures) == 0 {
		return section, nil
	}

	section.Status = sectionStatusFailed
	section.Summary = cfg.Label + " failed"
	section.Details = fmt.Sprintf("%s First failure: %s.", section.Details, failures[0])
	section.FailureCode = cfg.FailureCode
	return section, apperr.Wrap(apperr.KindIO, cfg.FailureCode, errors.New(failures[0]))
}

func collectGlobCleanupCandidates(dir string, patterns []cleanupPattern) ([]cleanupCandidate, error) {
	candidates := []cleanupCandidate{}
	for _, pattern := range patterns {
		matches, err := filepath.Glob(filepath.Join(dir, pattern.Pattern))
		if err != nil {
			return nil, fmt.Errorf("glob %s: %w", filepath.Join(dir, pattern.Pattern), err)
		}
		for _, match := range matches {
			info, err := os.Stat(match)
			if err != nil {
				if os.IsNotExist(err) {
					continue
				}
				return nil, fmt.Errorf("stat %s: %w", match, err)
			}
			if info.IsDir() {
				continue
			}
			path := match
			candidates = append(candidates, cleanupCandidate{
				Path:         path,
				Kind:         pattern.Kind,
				ProtectGroup: pattern.ProtectGroup,
				ModifiedAt:   info.ModTime().UTC(),
				SizeBytes:    info.Size(),
				Remove: func() error {
					if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
						return fmt.Errorf("remove %s: %w", path, err)
					}
					return nil
				},
			})
		}
	}
	return candidates, nil
}

func collectRestoreDrillCandidates(info Info) ([]cleanupCandidate, error) {
	candidates, err := collectGlobCleanupCandidates(info.RestoreDrillEnvDir, []cleanupPattern{
		{Pattern: fmt.Sprintf("restore-drill.%s.*.env", info.Scope), Kind: "restore_drill_env_file"},
	})
	if err != nil {
		return nil, err
	}

	if storageCandidate, err := existingPathCandidate(info.ProjectDir, info.RestoreDrillStorageDir, "restore_drill_storage_dir"); err != nil {
		return nil, err
	} else if storageCandidate != nil {
		candidates = append(candidates, *storageCandidate)
	}
	if backupCandidate, err := existingPathCandidate(info.ProjectDir, info.RestoreDrillBackupDir, "restore_drill_backup_dir"); err != nil {
		return nil, err
	} else if backupCandidate != nil {
		candidates = append(candidates, *backupCandidate)
	}

	return candidates, nil
}

func existingPathCandidate(projectDir, path, kind string) (*cleanupCandidate, error) {
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("stat %s: %w", path, err)
	}

	size, err := pathSizeBytes(path, info)
	if err != nil {
		return nil, err
	}

	return &cleanupCandidate{
		Path:       path,
		Kind:       kind,
		ModifiedAt: info.ModTime().UTC(),
		SizeBytes:  size,
		Remove: func() error {
			return removeMaintenanceTree(projectDir, path)
		},
	}, nil
}

func protectedCandidatePaths(candidates []cleanupCandidate) map[string]bool {
	protected := map[string]bool{}
	newest := map[string]cleanupCandidate{}
	for _, candidate := range candidates {
		if strings.TrimSpace(candidate.ProtectGroup) == "" {
			continue
		}
		current, ok := newest[candidate.ProtectGroup]
		if !ok || candidate.ModifiedAt.After(current.ModifiedAt) || (candidate.ModifiedAt.Equal(current.ModifiedAt) && candidate.Path > current.Path) {
			newest[candidate.ProtectGroup] = candidate
		}
	}
	for _, candidate := range newest {
		protected[candidate.Path] = true
	}
	return protected
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

func removeMaintenanceTree(projectDir, target string) error {
	target = strings.TrimSpace(target)
	if target == "" {
		return nil
	}
	rel, err := filepath.Rel(projectDir, target)
	if err != nil {
		return fmt.Errorf("resolve maintenance cleanup path %s: %w", target, err)
	}
	if rel == "." || rel == "" {
		return fmt.Errorf("refusing to remove the project root %s", target)
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return fmt.Errorf("refusing to remove a maintenance path outside the project directory: %s", target)
	}
	if err := os.RemoveAll(target); err != nil {
		return fmt.Errorf("remove maintenance path %s: %w", target, err)
	}
	return nil
}

func resolveEnvRetentionDays(env platformconfig.OperationEnv, key string, fallback int, override *int) (int, error) {
	if override != nil {
		if *override < 0 {
			return 0, fmt.Errorf("%s override must be non-negative", key)
		}
		return *override, nil
	}

	raw := strings.TrimSpace(env.Value(key))
	if raw == "" {
		return fallback, nil
	}

	days, err := strconv.Atoi(raw)
	if err != nil || days < 0 {
		return fallback, fmt.Errorf("%s must be a non-negative integer in %s", key, env.FilePath)
	}
	return days, nil
}

func resolveStaticRetentionDays(fallback int, override *int) (int, error) {
	if override == nil {
		return fallback, nil
	}
	if *override < 0 {
		return 0, fmt.Errorf("retention override must be non-negative")
	}
	return *override, nil
}

func cleanupCountsText(label string, kept, protected, removed, failed int, dryRun bool) string {
	removeLabel := "removed"
	if dryRun {
		removeLabel = "would remove"
	}

	text := fmt.Sprintf("%s: kept=%d protected=%d %s=%d", label, kept, protected, removeLabel, removed)
	if failed > 0 {
		text += fmt.Sprintf(" failed=%d", failed)
	}
	return text + "."
}

func maintenanceActionSummary(label string, dryRun bool) string {
	if dryRun {
		return label + " preview ready"
	}
	return label + " applied"
}

func maintenanceMode(dryRun bool) string {
	if dryRun {
		return "preview"
	}
	return "apply"
}

func maintenanceFailureCode(err error, fallback string) string {
	if code, ok := apperr.CodeOf(err); ok {
		return code
	}
	return fallback
}

func journalItemKind(kind string) string {
	switch kind {
	case journalusecase.PruneItemKindOperation:
		return "journal_operation"
	case journalusecase.PruneItemKindDir:
		return "journal_dir"
	default:
		return kind
	}
}

func finalizeMaintenanceInfo(info *Info) {
	warnings := append([]string(nil), info.Warnings...)
	info.IncludedSections = nil
	info.OmittedSections = nil
	info.FailedSections = nil
	for _, section := range info.Sections {
		switch section.Status {
		case sectionStatusIncluded:
			info.IncludedSections = append(info.IncludedSections, section.Code)
		case sectionStatusOmitted:
			info.OmittedSections = append(info.OmittedSections, section.Code)
		case sectionStatusFailed:
			info.FailedSections = append(info.FailedSections, section.Code)
		}
		warnings = append(warnings, section.Warnings...)
	}
	info.Warnings = dedupeStrings(warnings)
}

func primaryMaintenanceFailure(failures []error, failedSections []string) error {
	for _, err := range failures {
		if err == nil {
			continue
		}
		if kind, ok := apperr.KindOf(err); ok && kind != apperr.KindValidation {
			return wrapMaintenanceFailure(err)
		}
	}
	for _, err := range failures {
		if err != nil {
			return wrapMaintenanceFailure(err)
		}
	}
	return apperr.Wrap(apperr.KindValidation, "maintenance_failed", fmt.Errorf("maintenance found issues in %s", strings.Join(failedSections, ", ")))
}

func wrapMaintenanceFailure(err error) error {
	if err == nil {
		return nil
	}
	if code, ok := apperr.CodeOf(err); ok && code == "maintenance_failed" {
		return err
	}
	if kind, ok := apperr.KindOf(err); ok {
		return apperr.Wrap(kind, "maintenance_failed", err)
	}
	var pathErr *os.PathError
	if errors.As(err, &pathErr) {
		return apperr.Wrap(apperr.KindIO, "maintenance_failed", err)
	}
	return apperr.Wrap(apperr.KindValidation, "maintenance_failed", err)
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

func intPtr(value int) *int {
	return &value
}
