package update

import (
	"fmt"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/lazuale/espocrm-ops/internal/contract/apperr"
	platformconfig "github.com/lazuale/espocrm-ops/internal/platform/config"
	platformdocker "github.com/lazuale/espocrm-ops/internal/platform/docker"
	doctorusecase "github.com/lazuale/espocrm-ops/internal/usecase/doctor"
	operationusecase "github.com/lazuale/espocrm-ops/internal/usecase/operation"
)

const (
	PlanStatusWouldRun = "would_run"
	PlanStatusSkipped  = "skipped"
	PlanStatusBlocked  = "blocked"
	PlanStatusUnknown  = "unknown"
)

type PlanRequest struct {
	Scope           string
	ProjectDir      string
	ComposeFile     string
	EnvFileOverride string
	EnvContourHint  string
	TimeoutSeconds  int
	SkipDoctor      bool
	SkipBackup      bool
	SkipPull        bool
	SkipHTTPProbe   bool
}

type PlanStep struct {
	Code    string
	Status  string
	Summary string
	Details string
	Action  string
}

type UpdatePlan struct {
	Scope          string
	ProjectDir     string
	ComposeFile    string
	EnvFile        string
	ComposeProject string
	BackupRoot     string
	SiteURL        string
	TimeoutSeconds int
	SkipDoctor     bool
	SkipBackup     bool
	SkipPull       bool
	SkipHTTPProbe  bool
	Warnings       []string
	Steps          []PlanStep
	Recovery       operationusecase.RecoveryInfo
}

type planIssue struct {
	Summary string
	Details string
	Action  string
}

func BuildPlan(req PlanRequest) (UpdatePlan, error) {
	plan := UpdatePlan{
		Scope:          strings.TrimSpace(req.Scope),
		ProjectDir:     filepath.Clean(req.ProjectDir),
		ComposeFile:    filepath.Clean(req.ComposeFile),
		EnvFile:        plannedEnvFilePath(filepath.Clean(req.ProjectDir), strings.TrimSpace(req.Scope), strings.TrimSpace(req.EnvFileOverride)),
		TimeoutSeconds: req.TimeoutSeconds,
		SkipDoctor:     req.SkipDoctor,
		SkipBackup:     req.SkipBackup,
		SkipPull:       req.SkipPull,
		SkipHTTPProbe:  req.SkipHTTPProbe,
	}

	report, err := doctorusecase.Diagnose(doctorusecase.Request{
		Scope:           plan.Scope,
		ProjectDir:      plan.ProjectDir,
		ComposeFile:     plan.ComposeFile,
		EnvFileOverride: strings.TrimSpace(req.EnvFileOverride),
		EnvContourHint:  strings.TrimSpace(req.EnvContourHint),
		PathCheckMode:   doctorusecase.PathCheckModeReadOnly,
	})
	if err != nil {
		return plan, apperr.Wrap(apperr.KindInternal, "update_plan_failed", err)
	}

	env, envErr := platformconfig.LoadOperationEnv(plan.ProjectDir, plan.Scope, strings.TrimSpace(req.EnvFileOverride), strings.TrimSpace(req.EnvContourHint))
	if envErr == nil {
		plan.EnvFile = env.FilePath
		plan.ComposeProject = env.ComposeProject()
		plan.BackupRoot = platformconfig.ResolveProjectPath(plan.ProjectDir, env.BackupRoot())
		plan.SiteURL = strings.TrimSpace(env.Value("SITE_URL"))
	}

	plan.Warnings = collectPlanWarnings(report.Checks, req.SkipDoctor)

	doctorFailures := failingDoctorChecks(report.Checks)
	runtimeBlockers := blockingDoctorChecks(report.Checks)
	backupBlockers := append([]doctorusecase.Check(nil), runtimeBlockers...)
	if envErr == nil {
		backupBlockers = appendIssuesAsChecks(backupBlockers, backupConfigIssues(plan.ProjectDir, env))
		runtimeBlockers = appendIssuesAsChecks(runtimeBlockers, runtimeConfigIssues(env, req.SkipHTTPProbe))
	}

	flowBlockedByDoctor := !req.SkipDoctor && len(doctorFailures) != 0

	plan.Steps = append(plan.Steps, buildDoctorStep(report.Checks, req.SkipDoctor))

	backupStep := buildBackupStep(plan, req, env, envErr, backupBlockers, flowBlockedByDoctor, doctorFailures)
	plan.Steps = append(plan.Steps, backupStep)

	runtimeApplyStep := buildRuntimeApplyStep(plan, runtimeBlockers, flowBlockedByDoctor, doctorFailures)
	plan.Steps = append(plan.Steps, runtimeApplyStep)

	runtimeReadinessStep := buildRuntimeReadinessStep(plan, runtimeApplyStep)
	plan.Steps = append(plan.Steps, runtimeReadinessStep)

	return plan, nil
}

func (p UpdatePlan) Ready() bool {
	for _, step := range p.Steps {
		switch step.Status {
		case PlanStatusBlocked, PlanStatusUnknown:
			return false
		}
	}

	return true
}

func (p UpdatePlan) Counts() (wouldRun, skipped, blocked, unknown int) {
	for _, step := range p.Steps {
		switch step.Status {
		case PlanStatusWouldRun:
			wouldRun++
		case PlanStatusSkipped:
			skipped++
		case PlanStatusBlocked:
			blocked++
		case PlanStatusUnknown:
			unknown++
		}
	}

	return wouldRun, skipped, blocked, unknown
}

func buildDoctorStep(checks []doctorusecase.Check, skip bool) PlanStep {
	if skip {
		return PlanStep{
			Code:    "doctor",
			Status:  PlanStatusSkipped,
			Summary: "Pre-update doctor would be skipped",
			Details: "The update would not run the canonical doctor checks because of --skip-doctor.",
		}
	}

	failures := failingDoctorChecks(checks)
	if len(failures) != 0 {
		return PlanStep{
			Code:    "doctor",
			Status:  PlanStatusBlocked,
			Summary: "Pre-update doctor would stop the update",
			Details: formatDoctorChecks(failures),
			Action:  firstDoctorAction(failures, "Resolve the reported doctor failures before running update."),
		}
	}

	return PlanStep{
		Code:    "doctor",
		Status:  PlanStatusWouldRun,
		Summary: "Pre-update doctor would run",
		Details: "Would validate env/config resolution, runtime paths, lock readiness, Docker/Compose access, compose config, and current runtime health.",
	}
}

func buildBackupStep(plan UpdatePlan, req PlanRequest, env platformconfig.OperationEnv, envErr error, blockers []doctorusecase.Check, flowBlockedByDoctor bool, doctorFailures []doctorusecase.Check) PlanStep {
	if req.SkipBackup {
		return PlanStep{
			Code:    "backup_recovery_point",
			Status:  PlanStatusSkipped,
			Summary: "Pre-update recovery point would be skipped",
			Details: "The update would skip recovery-point creation because of --skip-backup.",
		}
	}

	if flowBlockedByDoctor {
		return PlanStep{
			Code:    "backup_recovery_point",
			Status:  PlanStatusBlocked,
			Summary: "The recovery point step would not run",
			Details: "Doctor would fail before any stateful step: " + formatDoctorChecks(doctorFailures),
			Action:  firstDoctorAction(doctorFailures, "Resolve the doctor failures or rerun intentionally with --skip-doctor."),
		}
	}

	if len(blockers) != 0 {
		return PlanStep{
			Code:    "backup_recovery_point",
			Status:  PlanStatusBlocked,
			Summary: "The recovery point step is blocked",
			Details: formatDoctorChecks(blockers),
			Action:  firstDoctorAction(blockers, "Resolve the blocking update prerequisites before creating a recovery point."),
		}
	}

	composeCfg := platformdocker.ComposeConfig{
		ProjectDir:  plan.ProjectDir,
		ComposeFile: plan.ComposeFile,
		EnvFile:     env.FilePath,
	}
	dbState, err := platformdocker.ComposeServiceStateFor(composeCfg, "db")
	if err != nil {
		return PlanStep{
			Code:    "backup_recovery_point",
			Status:  PlanStatusUnknown,
			Summary: "The recovery point step could not be planned completely",
			Details: fmt.Sprintf("Could not inspect the current db container state: %v", err),
			Action:  "Check Docker access for this contour and rerun the dry-run plan.",
		}
	}

	startMode := "would reuse the running db container"
	switch dbState.Status {
	case "", "exited", "dead", "created", "removing":
		startMode = "would start the db container temporarily before taking the recovery point"
	}

	namePrefix := strings.TrimSpace(env.Value("BACKUP_NAME_PREFIX"))
	if namePrefix == "" {
		namePrefix = strings.TrimSpace(env.ComposeProject())
	}

	return PlanStep{
		Code:    "backup_recovery_point",
		Status:  PlanStatusWouldRun,
		Summary: "A pre-update recovery point would be created",
		Details: fmt.Sprintf("Scope %s would write a recovery point under %s using prefix %s and %s.", plan.Scope, plan.BackupRoot, namePrefix, startMode),
	}
}

func buildRuntimeApplyStep(plan UpdatePlan, blockers []doctorusecase.Check, flowBlockedByDoctor bool, doctorFailures []doctorusecase.Check) PlanStep {
	if flowBlockedByDoctor {
		return PlanStep{
			Code:    "runtime_apply",
			Status:  PlanStatusBlocked,
			Summary: "Runtime apply would not run",
			Details: "Doctor would fail before runtime apply: " + formatDoctorChecks(doctorFailures),
			Action:  firstDoctorAction(doctorFailures, "Resolve the doctor failures or rerun intentionally with --skip-doctor."),
		}
	}

	if len(blockers) != 0 {
		return PlanStep{
			Code:    "runtime_apply",
			Status:  PlanStatusBlocked,
			Summary: "Runtime apply is blocked",
			Details: formatDoctorChecks(blockers),
			Action:  firstDoctorAction(blockers, "Resolve the blocking runtime prerequisites before applying the update."),
		}
	}

	details := "Would pull updated images, then restart the stack with the current configuration."
	if plan.SkipPull {
		details = "Would restart the stack with the current configuration; image pull would be skipped because of --skip-pull."
	}

	return PlanStep{
		Code:    "runtime_apply",
		Status:  PlanStatusWouldRun,
		Summary: "Runtime apply would run",
		Details: details,
	}
}

func buildRuntimeReadinessStep(plan UpdatePlan, runtimeApplyStep PlanStep) PlanStep {
	if runtimeApplyStep.Status != PlanStatusWouldRun {
		return PlanStep{
			Code:    "runtime_readiness",
			Status:  PlanStatusBlocked,
			Summary: "Runtime readiness checks would not run",
			Details: "Runtime apply is not ready, so the post-update readiness checks cannot proceed.",
			Action:  runtimeApplyStep.Action,
		}
	}

	details := fmt.Sprintf("Would wait up to %d seconds for db, espocrm, espocrm-daemon, and espocrm-websocket.", plan.TimeoutSeconds)
	if plan.SkipHTTPProbe {
		details += " The final HTTP probe would be skipped because of --skip-http-probe."
	} else {
		details += fmt.Sprintf(" The final HTTP probe would check %s.", plan.SiteURL)
	}

	return PlanStep{
		Code:    "runtime_readiness",
		Status:  PlanStatusWouldRun,
		Summary: "Post-update readiness checks would run",
		Details: details,
	}
}

func plannedEnvFilePath(projectDir, scope, override string) string {
	if strings.TrimSpace(override) != "" {
		return filepath.Clean(override)
	}

	return filepath.Join(projectDir, ".env."+scope)
}

func collectPlanWarnings(checks []doctorusecase.Check, skipDoctor bool) []string {
	warnings := []string{}
	for _, check := range checks {
		switch check.Status {
		case "warn":
			warnings = append(warnings, formatCheckWarning("", check))
		case "fail":
			if skipDoctor && !doctorCheckBlocksStatefulUpdate(check) {
				warnings = append(warnings, formatCheckWarning("Doctor would fail if it ran: ", check))
			}
		}
	}

	return warnings
}

func blockingDoctorChecks(checks []doctorusecase.Check) []doctorusecase.Check {
	items := make([]doctorusecase.Check, 0, len(checks))
	for _, check := range checks {
		if check.Status != "fail" {
			continue
		}
		if doctorCheckBlocksStatefulUpdate(check) {
			items = append(items, check)
		}
	}

	return items
}

func failingDoctorChecks(checks []doctorusecase.Check) []doctorusecase.Check {
	items := make([]doctorusecase.Check, 0, len(checks))
	for _, check := range checks {
		if check.Status == "fail" {
			items = append(items, check)
		}
	}

	return items
}

func doctorCheckBlocksStatefulUpdate(check doctorusecase.Check) bool {
	switch check.Code {
	case "compose_file", "env_resolution", "db_storage_dir", "espo_storage_dir", "backup_root", "shared_operation_lock", "maintenance_lock", "docker_cli", "docker_daemon", "docker_compose", "compose_config":
		return true
	default:
		return false
	}
}

func appendIssuesAsChecks(checks []doctorusecase.Check, issues []planIssue) []doctorusecase.Check {
	for _, issue := range issues {
		checks = append(checks, doctorusecase.Check{
			Code:    "update_plan",
			Status:  "fail",
			Summary: issue.Summary,
			Details: issue.Details,
			Action:  issue.Action,
		})
	}

	return checks
}

func backupConfigIssues(projectDir string, env platformconfig.OperationEnv) []planIssue {
	issues := []planIssue{}

	backupRoot := platformconfig.ResolveProjectPath(projectDir, env.BackupRoot())
	if strings.TrimSpace(backupRoot) == "" {
		issues = append(issues, planIssue{
			Summary: "BACKUP_ROOT did not resolve to a usable path",
			Details: "The recovery point step needs BACKUP_ROOT to resolve to a directory path.",
			Action:  "Set BACKUP_ROOT in the env file before running update.",
		})
	}

	storageDir := platformconfig.ResolveProjectPath(projectDir, env.ESPOStorageDir())
	if strings.TrimSpace(storageDir) == "" {
		issues = append(issues, planIssue{
			Summary: "ESPO_STORAGE_DIR did not resolve to a usable path",
			Details: "The recovery point step needs ESPO_STORAGE_DIR to resolve to a directory path.",
			Action:  "Set ESPO_STORAGE_DIR in the env file before running update.",
		})
	}

	namePrefix := strings.TrimSpace(env.Value("BACKUP_NAME_PREFIX"))
	if namePrefix == "" {
		namePrefix = strings.TrimSpace(env.ComposeProject())
	}
	if namePrefix == "" {
		issues = append(issues, planIssue{
			Summary: "BACKUP_NAME_PREFIX resolved to blank",
			Details: "The recovery point step needs BACKUP_NAME_PREFIX or COMPOSE_PROJECT_NAME to build artifact names.",
			Action:  "Set BACKUP_NAME_PREFIX explicitly or restore COMPOSE_PROJECT_NAME in the env file.",
		})
	}

	if raw := strings.TrimSpace(env.Value("BACKUP_RETENTION_DAYS")); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil {
			issues = append(issues, planIssue{
				Summary: "BACKUP_RETENTION_DAYS is invalid",
				Details: "BACKUP_RETENTION_DAYS must be a non-negative integer.",
				Action:  "Set BACKUP_RETENTION_DAYS to a non-negative integer before running update.",
			})
		} else if value < 0 {
			issues = append(issues, planIssue{
				Summary: "BACKUP_RETENTION_DAYS is negative",
				Details: "BACKUP_RETENTION_DAYS must be non-negative.",
				Action:  "Set BACKUP_RETENTION_DAYS to 0 or a positive integer before running update.",
			})
		}
	}

	for _, field := range []struct {
		Name   string
		Action string
	}{
		{Name: "DB_USER", Action: "Set DB_USER in the env file before running update."},
		{Name: "DB_PASSWORD", Action: "Set DB_PASSWORD in the env file before running update."},
		{Name: "DB_NAME", Action: "Set DB_NAME in the env file before running update."},
	} {
		if strings.TrimSpace(env.Value(field.Name)) == "" {
			issues = append(issues, planIssue{
				Summary: field.Name + " is required for the recovery point step",
				Details: "The backup command cannot connect to the database without " + field.Name + ".",
				Action:  field.Action,
			})
		}
	}

	return issues
}

func runtimeConfigIssues(env platformconfig.OperationEnv, skipHTTPProbe bool) []planIssue {
	if skipHTTPProbe {
		return nil
	}

	siteURL := strings.TrimSpace(env.Value("SITE_URL"))
	if siteURL == "" {
		return []planIssue{{
			Summary: "SITE_URL is required for the final HTTP probe",
			Details: "Runtime readiness includes an HTTP probe unless --skip-http-probe is used.",
			Action:  "Set SITE_URL in the env file or rerun intentionally with --skip-http-probe.",
		}}
	}

	if _, err := url.ParseRequestURI(siteURL); err != nil {
		return []planIssue{{
			Summary: "SITE_URL is invalid for the final HTTP probe",
			Details: err.Error(),
			Action:  "Fix SITE_URL in the env file or rerun intentionally with --skip-http-probe.",
		}}
	}

	return nil
}

func formatDoctorChecks(checks []doctorusecase.Check) string {
	parts := make([]string, 0, len(checks))
	for _, check := range checks {
		part := check.Summary
		if strings.TrimSpace(check.Details) != "" {
			part += ": " + strings.TrimSpace(check.Details)
		}
		parts = append(parts, part)
	}

	return strings.Join(parts, "; ")
}

func firstDoctorAction(checks []doctorusecase.Check, fallback string) string {
	for _, check := range checks {
		if strings.TrimSpace(check.Action) != "" {
			return check.Action
		}
	}

	return fallback
}

func formatCheckWarning(prefix string, check doctorusecase.Check) string {
	text := prefix + check.Summary
	if strings.TrimSpace(check.Details) != "" {
		text += ": " + strings.TrimSpace(check.Details)
	}

	return text
}
