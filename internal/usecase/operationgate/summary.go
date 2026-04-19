package operationgate

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/lazuale/espocrm-ops/internal/contract/apperr"
	healthsummaryusecase "github.com/lazuale/espocrm-ops/internal/usecase/healthsummary"
	migrateusecase "github.com/lazuale/espocrm-ops/internal/usecase/migrate"
	"github.com/lazuale/espocrm-ops/internal/usecase/reporting"
	restoreusecase "github.com/lazuale/espocrm-ops/internal/usecase/restore"
	rollbackusecase "github.com/lazuale/espocrm-ops/internal/usecase/rollback"
	updateusecase "github.com/lazuale/espocrm-ops/internal/usecase/update"
)

const (
	DecisionAllowed = "allowed"
	DecisionRisky   = "risky"
	DecisionBlocked = "blocked"

	SectionStatusIncluded = "included"
	SectionStatusOmitted  = "omitted"
	SectionStatusFailed   = "failed"

	AlertSeverityWarning  = "warning"
	AlertSeverityBlocking = "blocking"
	AlertSeverityFailure  = "failure"
)

type Request struct {
	Action          string
	Scope           string
	SourceScope     string
	TargetScope     string
	ProjectDir      string
	ComposeFile     string
	EnvFileOverride string
	JournalDir      string
	VerifyChecksum  bool
	MaxAgeHours     int
	Now             time.Time

	SkipDoctor    bool
	SkipBackup    bool
	SkipPull      bool
	SkipHTTPProbe bool

	DBBackup     string
	FilesBackup  string
	ManifestPath string
	SkipDB       bool
	SkipFiles    bool
	NoSnapshot   bool
	NoStop       bool
	NoStart      bool

	DrillAppPort  int
	DrillWSPort   int
	KeepArtifacts bool
}

type Info struct {
	Action              string
	Scope               string
	SourceScope         string
	TargetScope         string
	ProjectDir          string
	ComposeFile         string
	EnvFile             string
	BackupRoot          string
	SourceEnvFile       string
	SourceBackupRoot    string
	TargetEnvFile       string
	TargetBackupRoot    string
	GeneratedAt         string
	Decision            string
	NextAction          string
	HealthVerdict       string
	SourceHealthVerdict string
	TargetHealthVerdict string
	IncludedSections    []string
	OmittedSections     []string
	FailedSections      []string
	Warnings            []string
	Reasons             []string
	Sections            []Section
	Alerts              []Alert
}

type Section struct {
	Code          string `json:"code"`
	Status        string `json:"status"`
	Decision      string `json:"decision"`
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

type artifactsSnapshot struct {
	EnvFile          string
	BackupRoot       string
	SourceEnvFile    string
	SourceBackupRoot string
	TargetEnvFile    string
	TargetBackupRoot string
}

func Summarize(req Request) (Info, error) {
	now := req.Now.UTC()
	if req.Now.IsZero() {
		now = time.Now().UTC()
	}

	info := Info{
		Action:      strings.TrimSpace(req.Action),
		Scope:       strings.TrimSpace(req.Scope),
		SourceScope: strings.TrimSpace(req.SourceScope),
		TargetScope: strings.TrimSpace(req.TargetScope),
		ProjectDir:  strings.TrimSpace(req.ProjectDir),
		ComposeFile: strings.TrimSpace(req.ComposeFile),
		GeneratedAt: now.Format(time.RFC3339),
	}

	var (
		collector    reporting.SectionCollector
		baseWarnings []string
		errs         []error
	)

	addSection := func(section Section, alerts []Alert, warnings []string, artifacts artifactsSnapshot, err error) {
		info.Sections = append(info.Sections, section)
		collector.Add(sectionCategory(section.Status), section.Code, warnings)
		baseWarnings = append(baseWarnings, warnings...)
		info.Alerts = append(info.Alerts, alerts...)
		info.mergeArtifacts(artifacts)
		if err != nil {
			errs = append(errs, err)
		}
	}

	switch info.Action {
	case "update":
		healthSection, healthAlerts, healthWarnings, healthInfo, healthErr := summarizeHealthSection("health", info.Scope, req, now)
		info.HealthVerdict = healthInfo.Verdict
		addSection(healthSection, healthAlerts, healthWarnings, artifactsSnapshot{
			EnvFile:    healthInfo.EnvFile,
			BackupRoot: healthInfo.BackupRoot,
		}, healthErr)

		actionSection, actionAlerts, actionWarnings, actionArtifacts, actionErr := summarizeUpdateAction(req)
		addSection(actionSection, actionAlerts, actionWarnings, actionArtifacts, actionErr)
	case "rollback":
		healthSection, healthAlerts, healthWarnings, healthInfo, healthErr := summarizeHealthSection("health", info.Scope, req, now)
		info.HealthVerdict = healthInfo.Verdict
		addSection(healthSection, healthAlerts, healthWarnings, artifactsSnapshot{
			EnvFile:    healthInfo.EnvFile,
			BackupRoot: healthInfo.BackupRoot,
		}, healthErr)

		actionSection, actionAlerts, actionWarnings, actionArtifacts, actionErr := summarizeRollbackAction(req)
		addSection(actionSection, actionAlerts, actionWarnings, actionArtifacts, actionErr)
	case "restore":
		healthSection, healthAlerts, healthWarnings, healthInfo, healthErr := summarizeHealthSection("health", info.Scope, req, now)
		info.HealthVerdict = healthInfo.Verdict
		addSection(healthSection, healthAlerts, healthWarnings, artifactsSnapshot{
			EnvFile:    healthInfo.EnvFile,
			BackupRoot: healthInfo.BackupRoot,
		}, healthErr)

		actionSection, actionAlerts, actionWarnings, actionArtifacts, actionErr := summarizeRestoreAction(req)
		addSection(actionSection, actionAlerts, actionWarnings, actionArtifacts, actionErr)
	case "restore-drill":
		healthSection, healthAlerts, healthWarnings, healthInfo, healthErr := summarizeHealthSection("health", info.Scope, req, now)
		info.HealthVerdict = healthInfo.Verdict
		addSection(healthSection, healthAlerts, healthWarnings, artifactsSnapshot{
			EnvFile:    healthInfo.EnvFile,
			BackupRoot: healthInfo.BackupRoot,
		}, healthErr)

		actionSection, actionAlerts, actionWarnings, actionArtifacts, actionErr := summarizeRestoreDrillAction(req)
		addSection(actionSection, actionAlerts, actionWarnings, actionArtifacts, actionErr)
	case "migrate-backup":
		sourceSection, sourceAlerts, sourceWarnings, sourceHealth, sourceErr := summarizeHealthSection("source_health", info.SourceScope, Request{
			Scope:          info.SourceScope,
			ProjectDir:     info.ProjectDir,
			ComposeFile:    info.ComposeFile,
			JournalDir:     req.JournalDir,
			VerifyChecksum: req.VerifyChecksum,
			MaxAgeHours:    req.MaxAgeHours,
		}, now)
		info.SourceHealthVerdict = sourceHealth.Verdict
		addSection(sourceSection, sourceAlerts, sourceWarnings, artifactsSnapshot{
			SourceEnvFile:    sourceHealth.EnvFile,
			SourceBackupRoot: sourceHealth.BackupRoot,
		}, sourceErr)

		targetSection, targetAlerts, targetWarnings, targetHealth, targetErr := summarizeHealthSection("target_health", info.TargetScope, Request{
			Scope:          info.TargetScope,
			ProjectDir:     info.ProjectDir,
			ComposeFile:    info.ComposeFile,
			JournalDir:     req.JournalDir,
			VerifyChecksum: req.VerifyChecksum,
			MaxAgeHours:    req.MaxAgeHours,
		}, now)
		info.TargetHealthVerdict = targetHealth.Verdict
		addSection(targetSection, targetAlerts, targetWarnings, artifactsSnapshot{
			TargetEnvFile:    targetHealth.EnvFile,
			TargetBackupRoot: targetHealth.BackupRoot,
		}, targetErr)

		actionSection, actionAlerts, actionWarnings, actionArtifacts, actionErr := summarizeMigrateAction(req)
		addSection(actionSection, actionAlerts, actionWarnings, actionArtifacts, actionErr)
	default:
		err := apperr.Wrap(apperr.KindValidation, "operation_gate_failed", fmt.Errorf("unsupported action %q", info.Action))
		addSection(Section{
			Code:          "action_readiness",
			Status:        SectionStatusFailed,
			Decision:      DecisionBlocked,
			SourceCommand: "operation-gate",
			Summary:       "Operation gate action is unsupported",
			Details:       err.Error(),
			CauseCode:     "operation_gate_failed",
			NextAction:    "Select a supported action before rerunning operation-gate.",
		}, []Alert{newFailureAlert("operation_gate_failed", "action_readiness", "operation-gate", "Operation gate action is unsupported", err.Error(), "Select a supported action before rerunning operation-gate.")}, nil, artifactsSnapshot{}, err)
	}

	sortAlerts(info.Alerts)
	summary := collector.Finalize(baseWarnings)
	info.IncludedSections = summary.IncludedSections
	info.OmittedSections = summary.OmittedSections
	info.FailedSections = summary.FailedSections
	info.Warnings = summary.Warnings
	info.Decision = determineDecision(info.Sections)
	info.NextAction = determineNextAction(info.Alerts, info.Sections, info.Action, info.Scope, info.TargetScope)
	info.Reasons = determineReasons(info.Alerts, info.Sections)

	if len(info.FailedSections) != 0 {
		return info, primaryOperationGateFailure(errs)
	}
	if info.Decision == DecisionBlocked {
		return info, apperr.Wrap(apperr.KindValidation, "operation_gate_blocked", errors.New("operation gate reported blocking conditions"))
	}

	return info, nil
}

func summarizeHealthSection(code, scope string, req Request, now time.Time) (Section, []Alert, []string, healthsummaryusecase.Info, error) {
	info, err := healthsummaryusecase.Summarize(healthsummaryusecase.Request{
		Scope:           strings.TrimSpace(scope),
		ProjectDir:      strings.TrimSpace(req.ProjectDir),
		ComposeFile:     strings.TrimSpace(req.ComposeFile),
		EnvFileOverride: strings.TrimSpace(req.EnvFileOverride),
		JournalDir:      strings.TrimSpace(req.JournalDir),
		VerifyChecksum:  req.VerifyChecksum,
		MaxAgeHours:     req.MaxAgeHours,
		Now:             now,
	})

	label := scopeHealthLabel(code)
	section := Section{
		Code:          code,
		SourceCommand: "health-summary",
	}
	if err != nil || info.Verdict == healthsummaryusecase.VerdictFailed {
		section.Status = SectionStatusFailed
		section.Decision = DecisionBlocked
		section.Summary = label + " health summary unavailable"
		section.Details = errorDetails(err, info.Verdict, info.NextAction)
		section.CauseCode = operationGateFailureCode(err, "operation_gate_failed")
		section.NextAction = fallbackText(info.NextAction, "Restore the canonical health-summary signal before rerunning operation-gate.")
		return section, []Alert{newFailureAlert(section.CauseCode, code, "health-summary", section.Summary, section.Details, section.NextAction)}, nil, info, err
	}

	section.Status = SectionStatusIncluded
	section.Summary = label + " health is " + info.Verdict
	section.Details = healthDecisionDetails(info)
	section.NextAction = info.NextAction
	section.Decision = DecisionAllowed

	alerts := []Alert{}
	warnings := append([]string(nil), info.Warnings...)

	if info.MaintenanceState == healthsummaryusecase.VerdictBlocked {
		section.Decision = DecisionBlocked
		alerts = append(alerts, Alert{
			Code:          code + "_maintenance_blocked",
			Severity:      AlertSeverityBlocking,
			Section:       code,
			SourceCommand: "health-summary",
			Summary:       label + " maintenance lock blocks safe operations",
			Cause:         healthDecisionDetails(info),
			NextAction:    fallbackText(info.NextAction, "Wait for the active maintenance operation to finish before running the requested action."),
		})
		return section, alerts, warnings, info, nil
	}

	if info.LatestOperationState == healthsummaryusecase.VerdictDegraded {
		section.Decision = DecisionRisky
		alerts = append(alerts, Alert{
			Code:          code + "_latest_operation_review",
			Severity:      AlertSeverityWarning,
			Section:       code,
			SourceCommand: "health-summary",
			Summary:       latestOperationRiskSummary(label, info.LatestOperationCmd, info.LatestOperationID),
			Cause:         healthDecisionDetails(info),
			NextAction:    fallbackText(info.NextAction, "Inspect the latest recorded operation before relying on automation."),
		})
	}

	if info.Verdict == healthsummaryusecase.VerdictDegraded || info.Verdict == healthsummaryusecase.VerdictBlocked || len(info.Warnings) != 0 {
		section.Decision = DecisionRisky
		alerts = append(alerts, Alert{
			Code:          code + "_health_risky",
			Severity:      AlertSeverityWarning,
			Section:       code,
			SourceCommand: "health-summary",
			Summary:       label + " health requires operator review",
			Cause:         healthDecisionDetails(info),
			NextAction:    fallbackText(info.NextAction, "Review the health summary before running the requested action."),
		})
	}

	return section, alerts, warnings, info, nil
}

func summarizeUpdateAction(req Request) (Section, []Alert, []string, artifactsSnapshot, error) {
	plan, err := updateusecase.BuildPlan(updateusecase.PlanRequest{
		Scope:           strings.TrimSpace(req.Scope),
		ProjectDir:      strings.TrimSpace(req.ProjectDir),
		ComposeFile:     strings.TrimSpace(req.ComposeFile),
		EnvFileOverride: strings.TrimSpace(req.EnvFileOverride),
		TimeoutSeconds:  600,
		SkipDoctor:      req.SkipDoctor,
		SkipBackup:      req.SkipBackup,
		SkipPull:        req.SkipPull,
		SkipHTTPProbe:   req.SkipHTTPProbe,
	})
	artifacts := artifactsSnapshot{
		EnvFile:    plan.EnvFile,
		BackupRoot: plan.BackupRoot,
	}
	section := Section{
		Code:          "action_readiness",
		SourceCommand: "update-plan",
	}
	if err != nil {
		section.Status = SectionStatusFailed
		section.Decision = DecisionBlocked
		section.Summary = "Update readiness unavailable"
		section.Details = err.Error()
		section.CauseCode = operationGateFailureCode(err, "operation_gate_failed")
		section.NextAction = "Repair update readiness inspection before rerunning operation-gate."
		return section, []Alert{newFailureAlert(section.CauseCode, "action_readiness", "update-plan", section.Summary, section.Details, section.NextAction)}, nil, artifacts, err
	}

	section.Status = SectionStatusIncluded
	section.Details = planCountsText(plan.Counts())
	section.NextAction = defaultActionNextAction("update", req.Scope, "")

	alerts := []Alert{}
	if !plan.Ready() {
		section.Decision = DecisionBlocked
		section.Summary = "Update dry-run plan found blocking conditions"
		section.NextAction = firstUpdateAction(plan.Steps, "Resolve the blocking update prerequisites before running update.")
		alerts = append(alerts, updateStepAlerts(plan.Steps)...)
		return section, alerts, append([]string(nil), plan.Warnings...), artifacts, nil
	}

	section.Decision = DecisionAllowed
	section.Summary = "Update dry-run plan is ready"
	alerts = append(alerts, warningAlertsFromMessages("action_readiness", "update-plan", "update_warning", plan.Warnings)...)
	alerts = append(alerts, updateFlagAlerts(plan)...)
	if len(alerts) != 0 {
		section.Decision = DecisionRisky
		section.Summary = "Update dry-run plan is ready with reduced safeguards"
	}

	return section, alerts, append([]string(nil), plan.Warnings...), artifacts, nil
}

func summarizeRollbackAction(req Request) (Section, []Alert, []string, artifactsSnapshot, error) {
	plan, err := rollbackusecase.BuildPlan(rollbackusecase.PlanRequest{
		Scope:           strings.TrimSpace(req.Scope),
		ProjectDir:      strings.TrimSpace(req.ProjectDir),
		ComposeFile:     strings.TrimSpace(req.ComposeFile),
		EnvFileOverride: strings.TrimSpace(req.EnvFileOverride),
		DBBackup:        strings.TrimSpace(req.DBBackup),
		FilesBackup:     strings.TrimSpace(req.FilesBackup),
		NoSnapshot:      req.NoSnapshot,
		NoStart:         req.NoStart,
		SkipHTTPProbe:   req.SkipHTTPProbe,
		TimeoutSeconds:  600,
	})
	artifacts := artifactsSnapshot{
		EnvFile:    plan.EnvFile,
		BackupRoot: plan.BackupRoot,
	}
	section := Section{
		Code:          "action_readiness",
		SourceCommand: "rollback-plan",
	}
	if err != nil {
		section.Status = SectionStatusFailed
		section.Decision = DecisionBlocked
		section.Summary = "Rollback readiness unavailable"
		section.Details = err.Error()
		section.CauseCode = operationGateFailureCode(err, "operation_gate_failed")
		section.NextAction = "Repair rollback readiness inspection before rerunning operation-gate."
		return section, []Alert{newFailureAlert(section.CauseCode, "action_readiness", "rollback-plan", section.Summary, section.Details, section.NextAction)}, nil, artifacts, err
	}

	section.Status = SectionStatusIncluded
	section.Details = planCountsText(plan.Counts())
	section.NextAction = defaultActionNextAction("rollback", req.Scope, "")

	alerts := []Alert{}
	if !plan.Ready() {
		section.Decision = DecisionBlocked
		section.Summary = "Rollback dry-run plan found blocking conditions"
		section.NextAction = firstRollbackAction(plan.Steps, "Resolve the blocking rollback prerequisites before running rollback.")
		alerts = append(alerts, rollbackStepAlerts(plan.Steps)...)
		return section, alerts, append([]string(nil), plan.Warnings...), artifacts, nil
	}

	section.Decision = DecisionAllowed
	section.Summary = "Rollback dry-run plan is ready"
	alerts = append(alerts, warningAlertsFromMessages("action_readiness", "rollback-plan", "rollback_warning", plan.Warnings)...)
	alerts = append(alerts, rollbackFlagAlerts(plan)...)
	if len(alerts) != 0 {
		section.Decision = DecisionRisky
		section.Summary = "Rollback dry-run plan is ready with reduced safeguards"
	}

	return section, alerts, append([]string(nil), plan.Warnings...), artifacts, nil
}

func summarizeRestoreAction(req Request) (Section, []Alert, []string, artifactsSnapshot, error) {
	info, err := restoreusecase.Execute(restoreusecase.ExecuteRequest{
		Scope:           strings.TrimSpace(req.Scope),
		ProjectDir:      strings.TrimSpace(req.ProjectDir),
		ComposeFile:     strings.TrimSpace(req.ComposeFile),
		EnvFileOverride: strings.TrimSpace(req.EnvFileOverride),
		ManifestPath:    strings.TrimSpace(req.ManifestPath),
		DBBackup:        strings.TrimSpace(req.DBBackup),
		FilesBackup:     strings.TrimSpace(req.FilesBackup),
		SkipDB:          req.SkipDB,
		SkipFiles:       req.SkipFiles,
		NoSnapshot:      req.NoSnapshot,
		NoStop:          req.NoStop,
		NoStart:         req.NoStart,
		DryRun:          true,
	})
	artifacts := artifactsSnapshot{
		EnvFile:    info.EnvFile,
		BackupRoot: info.BackupRoot,
	}
	section := Section{
		Code:          "action_readiness",
		SourceCommand: "restore",
		Status:        SectionStatusIncluded,
		Details:       restoreCountsText(info),
		NextAction:    defaultActionNextAction("restore", req.Scope, ""),
	}

	if err != nil {
		section.Status = gateSectionStatusForError(err)
		section.Decision = DecisionBlocked
		section.Summary = "Restore dry-run gate found blocking conditions"
		if section.Status == SectionStatusFailed {
			section.Summary = "Restore dry-run readiness unavailable"
		}
		section.Details = mergeDetailText(section.Details, err.Error())
		section.CauseCode = operationGateFailureCode(err, "operation_gate_failed")
		section.NextAction = firstRestoreAction(info.Steps, "Resolve the restore dry-run failure before rerunning operation-gate.")
		return section, restoreStepAlerts(info.Steps, section.Status), append([]string(nil), info.Warnings...), artifacts, err
	}

	if !info.Ready() {
		section.Decision = DecisionBlocked
		section.Summary = "Restore dry-run gate found blocking conditions"
		section.NextAction = firstRestoreAction(info.Steps, "Resolve the restore dry-run blockers before running restore.")
		return section, restoreStepAlerts(info.Steps, section.Status), append([]string(nil), info.Warnings...), artifacts, nil
	}

	section.Decision = DecisionAllowed
	section.Summary = "Restore dry-run gate is ready"
	alerts := warningAlertsFromMessages("action_readiness", "restore", "restore_warning", info.Warnings)
	alerts = append(alerts, restoreFlagAlerts(info)...)
	if len(alerts) != 0 {
		section.Decision = DecisionRisky
		section.Summary = "Restore dry-run gate is ready with reduced safeguards"
	}

	return section, alerts, append([]string(nil), info.Warnings...), artifacts, nil
}

func summarizeRestoreDrillAction(req Request) (Section, []Alert, []string, artifactsSnapshot, error) {
	info, err := restoreusecase.PreflightDrill(restoreusecase.DrillGateRequest{
		Scope:           strings.TrimSpace(req.Scope),
		ProjectDir:      strings.TrimSpace(req.ProjectDir),
		ComposeFile:     strings.TrimSpace(req.ComposeFile),
		EnvFileOverride: strings.TrimSpace(req.EnvFileOverride),
		DBBackup:        strings.TrimSpace(req.DBBackup),
		FilesBackup:     strings.TrimSpace(req.FilesBackup),
		DrillAppPort:    req.DrillAppPort,
		DrillWSPort:     req.DrillWSPort,
		SkipHTTPProbe:   req.SkipHTTPProbe,
		KeepArtifacts:   req.KeepArtifacts,
	})
	artifacts := artifactsSnapshot{
		EnvFile:    info.EnvFile,
		BackupRoot: info.BackupRoot,
	}
	section := Section{
		Code:          "action_readiness",
		SourceCommand: "restore-drill",
		Status:        SectionStatusIncluded,
		Details:       restoreDrillGateDetails(info),
		NextAction:    defaultActionNextAction("restore-drill", req.Scope, ""),
	}

	if err != nil {
		section.Status = gateSectionStatusForError(err)
		section.Decision = DecisionBlocked
		section.Summary = "Restore-drill preflight found blocking conditions"
		if section.Status == SectionStatusFailed {
			section.Summary = "Restore-drill preflight unavailable"
		}
		section.Details = mergeDetailText(section.Details, err.Error())
		section.CauseCode = operationGateFailureCode(err, "operation_gate_failed")
		section.NextAction = firstRestoreDrillAction(info.Checks, "Resolve the restore-drill readiness failure before rerunning operation-gate.")
		return section, restoreDrillCheckAlerts(info.Checks, section.Status), append([]string(nil), info.Warnings...), artifacts, err
	}

	section.Decision = DecisionAllowed
	section.Summary = "Restore-drill preflight is ready"
	alerts := warningAlertsFromMessages("action_readiness", "restore-drill", "restore_drill_warning", info.Warnings)
	if len(alerts) != 0 {
		section.Decision = DecisionRisky
		section.Summary = "Restore-drill preflight is ready with operator warnings"
	}

	return section, alerts, append([]string(nil), info.Warnings...), artifacts, nil
}

func summarizeMigrateAction(req Request) (Section, []Alert, []string, artifactsSnapshot, error) {
	info, err := migrateusecase.Preflight(migrateusecase.GateRequest{
		SourceScope: strings.TrimSpace(req.SourceScope),
		TargetScope: strings.TrimSpace(req.TargetScope),
		ProjectDir:  strings.TrimSpace(req.ProjectDir),
		ComposeFile: strings.TrimSpace(req.ComposeFile),
		DBBackup:    strings.TrimSpace(req.DBBackup),
		FilesBackup: strings.TrimSpace(req.FilesBackup),
		SkipDB:      req.SkipDB,
		SkipFiles:   req.SkipFiles,
		NoStart:     req.NoStart,
	})
	artifacts := artifactsSnapshot{
		SourceEnvFile:    info.SourceEnvFile,
		SourceBackupRoot: info.SourceBackupRoot,
		TargetEnvFile:    info.TargetEnvFile,
		TargetBackupRoot: info.TargetBackupRoot,
	}
	section := Section{
		Code:          "action_readiness",
		SourceCommand: "migrate-backup",
		Status:        SectionStatusIncluded,
		Details:       migrateGateDetails(info),
		NextAction:    defaultActionNextAction("migrate-backup", "", req.TargetScope),
	}

	if err != nil {
		section.Status = gateSectionStatusForError(err)
		section.Decision = DecisionBlocked
		section.Summary = "Backup migration preflight found blocking conditions"
		if section.Status == SectionStatusFailed {
			section.Summary = "Backup migration preflight unavailable"
		}
		section.Details = mergeDetailText(section.Details, err.Error())
		section.CauseCode = operationGateFailureCode(err, "operation_gate_failed")
		section.NextAction = firstMigrateAction(info.Checks, "Resolve the migration readiness failure before rerunning operation-gate.")
		return section, migrateCheckAlerts(info.Checks, section.Status), append([]string(nil), info.Warnings...), artifacts, err
	}

	section.Decision = DecisionAllowed
	section.Summary = "Backup migration preflight is ready"
	alerts := warningAlertsFromMessages("action_readiness", "migrate-backup", "migrate_warning", info.Warnings)
	if len(alerts) != 0 {
		section.Decision = DecisionRisky
		section.Summary = "Backup migration preflight is ready with reduced safeguards"
	}

	return section, alerts, append([]string(nil), info.Warnings...), artifacts, nil
}

func (i *Info) mergeArtifacts(snapshot artifactsSnapshot) {
	if i == nil {
		return
	}
	if i.EnvFile == "" {
		i.EnvFile = snapshot.EnvFile
	}
	if i.BackupRoot == "" {
		i.BackupRoot = snapshot.BackupRoot
	}
	if i.SourceEnvFile == "" {
		i.SourceEnvFile = snapshot.SourceEnvFile
	}
	if i.SourceBackupRoot == "" {
		i.SourceBackupRoot = snapshot.SourceBackupRoot
	}
	if i.TargetEnvFile == "" {
		i.TargetEnvFile = snapshot.TargetEnvFile
	}
	if i.TargetBackupRoot == "" {
		i.TargetBackupRoot = snapshot.TargetBackupRoot
	}
}

func determineDecision(sections []Section) string {
	decision := DecisionAllowed
	for _, section := range sections {
		if section.Status == SectionStatusFailed || section.Decision == DecisionBlocked {
			return DecisionBlocked
		}
		if section.Decision == DecisionRisky {
			decision = DecisionRisky
		}
	}
	return decision
}

func determineNextAction(alerts []Alert, sections []Section, action, scope, targetScope string) string {
	for _, alert := range alerts {
		if strings.TrimSpace(alert.NextAction) != "" {
			return alert.NextAction
		}
	}
	for _, section := range sections {
		if strings.TrimSpace(section.NextAction) != "" {
			return section.NextAction
		}
	}
	return defaultActionNextAction(action, scope, targetScope)
}

func determineReasons(alerts []Alert, sections []Section) []string {
	reasons := []string{}
	seen := map[string]struct{}{}
	appendReason := func(value string) {
		value = strings.TrimSpace(value)
		if value == "" {
			return
		}
		if _, ok := seen[value]; ok {
			return
		}
		seen[value] = struct{}{}
		reasons = append(reasons, value)
	}

	for _, alert := range alerts {
		appendReason(alert.Summary)
		if len(reasons) == 3 {
			return reasons
		}
	}
	for _, section := range sections {
		if section.Decision != DecisionAllowed || section.Status == SectionStatusFailed {
			appendReason(section.Summary)
		}
		if len(reasons) == 3 {
			return reasons
		}
	}
	return reasons
}

func scopeHealthLabel(code string) string {
	switch code {
	case "source_health":
		return "Source contour"
	case "target_health":
		return "Target contour"
	default:
		return "Contour"
	}
}

func healthDecisionDetails(info healthsummaryusecase.Info) string {
	parts := []string{}
	if strings.TrimSpace(info.Verdict) != "" {
		parts = append(parts, "verdict="+info.Verdict)
	}
	if strings.TrimSpace(info.DoctorState) != "" {
		parts = append(parts, "doctor="+info.DoctorState)
	}
	if strings.TrimSpace(info.RuntimeState) != "" {
		parts = append(parts, "runtime="+info.RuntimeState)
	}
	if strings.TrimSpace(info.BackupState) != "" {
		parts = append(parts, "backup="+info.BackupState)
	}
	if strings.TrimSpace(info.LatestOperationState) != "" {
		parts = append(parts, "latest_operation="+info.LatestOperationState)
	}
	if strings.TrimSpace(info.MaintenanceState) != "" {
		parts = append(parts, "maintenance="+info.MaintenanceState)
	}
	return strings.Join(parts, ", ")
}

func latestOperationRiskSummary(label, command, id string) string {
	command = strings.TrimSpace(command)
	id = strings.TrimSpace(id)
	switch {
	case command != "" && id != "":
		return fmt.Sprintf("%s latest operation %s %s should be reviewed", label, command, id)
	case command != "":
		return fmt.Sprintf("%s latest %s operation should be reviewed", label, command)
	default:
		return label + " latest operation should be reviewed"
	}
}

func planCountsText(wouldRun, skipped, blocked, unknown int) string {
	return fmt.Sprintf("would_run=%d, skipped=%d, blocked=%d, unknown=%d", wouldRun, skipped, blocked, unknown)
}

func restoreCountsText(info restoreusecase.ExecuteInfo) string {
	wouldRun, completed, skipped, blocked, failed := info.Counts()
	return fmt.Sprintf("would_run=%d, completed=%d, skipped=%d, blocked=%d, failed=%d", wouldRun, completed, skipped, blocked, failed)
}

func restoreDrillGateDetails(info restoreusecase.DrillGateInfo) string {
	parts := []string{}
	if strings.TrimSpace(info.SelectionMode) != "" {
		parts = append(parts, "selection_mode="+info.SelectionMode)
	}
	if strings.TrimSpace(info.ManifestJSONPath) != "" {
		parts = append(parts, "manifest="+info.ManifestJSONPath)
	}
	if strings.TrimSpace(info.DBBackupPath) != "" {
		parts = append(parts, "db_backup="+info.DBBackupPath)
	}
	if strings.TrimSpace(info.FilesBackupPath) != "" {
		parts = append(parts, "files_backup="+info.FilesBackupPath)
	}
	if info.DrillAppPort > 0 {
		parts = append(parts, fmt.Sprintf("app_port=%d", info.DrillAppPort))
	}
	if info.DrillWSPort > 0 {
		parts = append(parts, fmt.Sprintf("ws_port=%d", info.DrillWSPort))
	}
	return strings.Join(parts, ", ")
}

func migrateGateDetails(info migrateusecase.GateInfo) string {
	parts := []string{}
	if strings.TrimSpace(info.SelectionMode) != "" {
		parts = append(parts, "selection_mode="+info.SelectionMode)
	}
	if strings.TrimSpace(info.ManifestJSONPath) != "" {
		parts = append(parts, "manifest="+info.ManifestJSONPath)
	}
	if strings.TrimSpace(info.DBBackupPath) != "" {
		parts = append(parts, "db_backup="+info.DBBackupPath)
	}
	if strings.TrimSpace(info.FilesBackupPath) != "" {
		parts = append(parts, "files_backup="+info.FilesBackupPath)
	}
	return strings.Join(parts, ", ")
}

func gateSectionStatusForError(err error) string {
	if err == nil {
		return SectionStatusIncluded
	}
	kind, ok := apperr.KindOf(err)
	if !ok {
		return SectionStatusFailed
	}
	switch kind {
	case apperr.KindValidation, apperr.KindConflict, apperr.KindNotFound, apperr.KindCorrupted:
		return SectionStatusIncluded
	default:
		return SectionStatusFailed
	}
}

func warningAlertsFromMessages(section, source, codePrefix string, warnings []string) []Alert {
	alerts := make([]Alert, 0, len(warnings))
	for idx, warning := range warnings {
		alerts = append(alerts, Alert{
			Code:          fmt.Sprintf("%s_%d", codePrefix, idx+1),
			Severity:      AlertSeverityWarning,
			Section:       section,
			SourceCommand: source,
			Summary:       warning,
			Cause:         warning,
		})
	}
	return alerts
}

func updateFlagAlerts(plan updateusecase.UpdatePlan) []Alert {
	alerts := []Alert{}
	if plan.SkipDoctor {
		alerts = append(alerts, warningAlert("update_skip_doctor", "action_readiness", "update-plan", "Update would skip doctor checks", "Skipping doctor reduces readiness coverage.", "Remove --skip-doctor if you need the canonical readiness gate."))
	}
	if plan.SkipBackup {
		alerts = append(alerts, warningAlert("update_skip_backup", "action_readiness", "update-plan", "Update would skip the recovery point", "Skipping the recovery point reduces rollback safety.", "Remove --skip-backup if you need the canonical recovery point."))
	}
	if plan.SkipPull {
		alerts = append(alerts, warningAlert("update_skip_pull", "action_readiness", "update-plan", "Update would skip image pull", "Skipping image pull can leave stale runtime images in place.", "Remove --skip-pull if you need fresh image fetch before update."))
	}
	if plan.SkipHTTPProbe {
		alerts = append(alerts, warningAlert("update_skip_http_probe", "action_readiness", "update-plan", "Update would skip the final HTTP probe", "Skipping the final HTTP probe reduces runtime confirmation.", "Remove --skip-http-probe if you need the canonical readiness confirmation."))
	}
	return alerts
}

func rollbackFlagAlerts(plan rollbackusecase.RollbackPlan) []Alert {
	alerts := []Alert{}
	if !plan.SnapshotEnabled {
		alerts = append(alerts, warningAlert("rollback_no_snapshot", "action_readiness", "rollback-plan", "Rollback would skip the emergency recovery point", "Skipping the recovery point reduces rollback safety.", "Remove --no-snapshot if you need the canonical recovery point."))
	}
	if plan.NoStart {
		alerts = append(alerts, warningAlert("rollback_no_start", "action_readiness", "rollback-plan", "Rollback would leave the contour stopped", "Leaving the contour stopped requires a separate runtime start step.", "Remove --no-start if the contour should return to service automatically."))
	}
	if plan.SkipHTTPProbe {
		alerts = append(alerts, warningAlert("rollback_skip_http_probe", "action_readiness", "rollback-plan", "Rollback would skip the final HTTP probe", "Skipping the final HTTP probe reduces runtime confirmation.", "Remove --skip-http-probe if you need the canonical readiness confirmation."))
	}
	return alerts
}

func restoreFlagAlerts(info restoreusecase.ExecuteInfo) []Alert {
	alerts := []Alert{}
	if !info.SnapshotEnabled {
		alerts = append(alerts, warningAlert("restore_no_snapshot", "action_readiness", "restore", "Restore would skip the emergency recovery point", "Skipping the recovery point reduces restore safety.", "Remove --no-snapshot if you need the canonical recovery point."))
	}
	if info.SkipDB {
		alerts = append(alerts, warningAlert("restore_skip_db", "action_readiness", "restore", "Restore would skip the database step", "Partial restore runs require extra operator verification.", "Remove --skip-db if you need the full restore flow."))
	}
	if info.SkipFiles {
		alerts = append(alerts, warningAlert("restore_skip_files", "action_readiness", "restore", "Restore would skip the files step", "Partial restore runs require extra operator verification.", "Remove --skip-files if you need the full restore flow."))
	}
	if info.NoStop {
		alerts = append(alerts, warningAlert("restore_no_stop", "action_readiness", "restore", "Restore would keep app services running during runtime preparation", "Keeping application services running can increase restore risk.", "Remove --no-stop if you need the canonical stop-before-restore behavior."))
	}
	if info.NoStart {
		alerts = append(alerts, warningAlert("restore_no_start", "action_readiness", "restore", "Restore would leave the contour stopped", "Leaving the contour stopped requires a separate runtime start step.", "Remove --no-start if the contour should return to service automatically."))
	}
	return alerts
}

func warningAlert(code, section, source, summary, cause, nextAction string) Alert {
	return Alert{
		Code:          code,
		Severity:      AlertSeverityWarning,
		Section:       section,
		SourceCommand: source,
		Summary:       summary,
		Cause:         cause,
		NextAction:    nextAction,
	}
}

func updateStepAlerts(steps []updateusecase.PlanStep) []Alert {
	alerts := []Alert{}
	for _, step := range steps {
		if step.Status != updateusecase.PlanStatusBlocked && step.Status != updateusecase.PlanStatusUnknown {
			continue
		}
		alerts = append(alerts, Alert{
			Code:          "update_" + strings.TrimSpace(step.Code),
			Severity:      AlertSeverityBlocking,
			Section:       "action_readiness",
			SourceCommand: "update-plan",
			Summary:       step.Summary,
			Cause:         step.Details,
			NextAction:    step.Action,
		})
	}
	return alerts
}

func rollbackStepAlerts(steps []rollbackusecase.PlanStep) []Alert {
	alerts := []Alert{}
	for _, step := range steps {
		if step.Status != rollbackusecase.PlanStatusBlocked && step.Status != rollbackusecase.PlanStatusUnknown {
			continue
		}
		alerts = append(alerts, Alert{
			Code:          "rollback_" + strings.TrimSpace(step.Code),
			Severity:      AlertSeverityBlocking,
			Section:       "action_readiness",
			SourceCommand: "rollback-plan",
			Summary:       step.Summary,
			Cause:         step.Details,
			NextAction:    step.Action,
		})
	}
	return alerts
}

func restoreStepAlerts(steps []restoreusecase.ExecuteStep, status string) []Alert {
	alerts := []Alert{}
	for _, step := range steps {
		if step.Status != restoreusecase.RestoreStepStatusBlocked && step.Status != restoreusecase.RestoreStepStatusFailed {
			continue
		}
		severity := AlertSeverityBlocking
		if status == SectionStatusFailed || step.Status == restoreusecase.RestoreStepStatusFailed {
			severity = AlertSeverityFailure
		}
		alerts = append(alerts, Alert{
			Code:          "restore_" + strings.TrimSpace(step.Code),
			Severity:      severity,
			Section:       "action_readiness",
			SourceCommand: "restore",
			Summary:       step.Summary,
			Cause:         step.Details,
			NextAction:    step.Action,
		})
	}
	return alerts
}

func restoreDrillCheckAlerts(checks []restoreusecase.DrillGateCheck, status string) []Alert {
	alerts := []Alert{}
	for _, check := range checks {
		if check.Status != restoreusecase.GateCheckBlocked {
			continue
		}
		severity := AlertSeverityBlocking
		if status == SectionStatusFailed {
			severity = AlertSeverityFailure
		}
		alerts = append(alerts, Alert{
			Code:          "restore_drill_" + strings.TrimSpace(check.Code),
			Severity:      severity,
			Section:       "action_readiness",
			SourceCommand: "restore-drill",
			Summary:       check.Summary,
			Cause:         check.Details,
			NextAction:    check.Action,
		})
	}
	return alerts
}

func migrateCheckAlerts(checks []migrateusecase.GateCheck, status string) []Alert {
	alerts := []Alert{}
	for _, check := range checks {
		if check.Status != restoreusecase.GateCheckBlocked {
			continue
		}
		severity := AlertSeverityBlocking
		if status == SectionStatusFailed {
			severity = AlertSeverityFailure
		}
		alerts = append(alerts, Alert{
			Code:          "migrate_" + strings.TrimSpace(check.Code),
			Severity:      severity,
			Section:       "action_readiness",
			SourceCommand: "migrate-backup",
			Summary:       check.Summary,
			Cause:         check.Details,
			NextAction:    check.Action,
		})
	}
	return alerts
}

func firstUpdateAction(steps []updateusecase.PlanStep, fallback string) string {
	for _, step := range steps {
		if step.Status == updateusecase.PlanStatusBlocked || step.Status == updateusecase.PlanStatusUnknown {
			return fallbackText(step.Action, fallback)
		}
	}
	return fallback
}

func firstRollbackAction(steps []rollbackusecase.PlanStep, fallback string) string {
	for _, step := range steps {
		if step.Status == rollbackusecase.PlanStatusBlocked || step.Status == rollbackusecase.PlanStatusUnknown {
			return fallbackText(step.Action, fallback)
		}
	}
	return fallback
}

func firstRestoreAction(steps []restoreusecase.ExecuteStep, fallback string) string {
	for _, step := range steps {
		if (step.Status == restoreusecase.RestoreStepStatusBlocked || step.Status == restoreusecase.RestoreStepStatusFailed) && strings.TrimSpace(step.Action) != "" {
			return step.Action
		}
	}
	return fallback
}

func firstRestoreDrillAction(checks []restoreusecase.DrillGateCheck, fallback string) string {
	for _, check := range checks {
		if check.Status == restoreusecase.GateCheckBlocked && strings.TrimSpace(check.Action) != "" {
			return check.Action
		}
	}
	return fallback
}

func firstMigrateAction(checks []migrateusecase.GateCheck, fallback string) string {
	for _, check := range checks {
		if check.Status == restoreusecase.GateCheckBlocked && strings.TrimSpace(check.Action) != "" {
			return check.Action
		}
	}
	return fallback
}

func defaultActionNextAction(action, scope, targetScope string) string {
	switch action {
	case "rollback", "restore":
		if scope == "prod" {
			return "When executing, pass --force and --confirm-prod prod."
		}
		return "When executing, pass --force."
	case "migrate-backup":
		if targetScope == "prod" {
			return "When executing, pass --force and --confirm-prod prod."
		}
		return "When executing, pass --force."
	case "update":
		return "Run update when ready."
	case "restore-drill":
		return "Run restore-drill when ready."
	default:
		return ""
	}
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

func primaryOperationGateFailure(errs []error) error {
	for _, err := range errs {
		if err == nil {
			continue
		}
		if kind, ok := apperr.KindOf(err); ok && kind != apperr.KindValidation && kind != apperr.KindConflict {
			return wrapOperationGateFailure(err)
		}
	}
	for _, err := range errs {
		if err != nil {
			return wrapOperationGateFailure(err)
		}
	}
	return apperr.Wrap(apperr.KindValidation, "operation_gate_failed", errors.New("operation gate could not produce a complete decision"))
}

func wrapOperationGateFailure(err error) error {
	if err == nil {
		return nil
	}
	if code, ok := apperr.CodeOf(err); ok && code == "operation_gate_failed" {
		return err
	}
	if kind, ok := apperr.KindOf(err); ok {
		return apperr.Wrap(kind, "operation_gate_failed", err)
	}
	return apperr.Wrap(apperr.KindValidation, "operation_gate_failed", err)
}

func operationGateFailureCode(err error, fallback string) string {
	if err == nil {
		return fallback
	}
	if code, ok := apperr.CodeOf(err); ok {
		return code
	}
	return fallback
}

func errorDetails(err error, verdict, nextAction string) string {
	if err != nil {
		return err.Error()
	}
	parts := []string{}
	if strings.TrimSpace(verdict) != "" {
		parts = append(parts, "verdict="+verdict)
	}
	if strings.TrimSpace(nextAction) != "" {
		parts = append(parts, "next_action="+nextAction)
	}
	return strings.Join(parts, ", ")
}

func mergeDetailText(parts ...string) string {
	items := []string{}
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			items = append(items, part)
		}
	}
	return strings.Join(items, " | ")
}

func fallbackText(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}
