package rollback

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/lazuale/espocrm-ops/internal/contract/apperr"
	domainbackup "github.com/lazuale/espocrm-ops/internal/domain/backup"
	"github.com/lazuale/espocrm-ops/internal/platform/backupstore"
	platformconfig "github.com/lazuale/espocrm-ops/internal/platform/config"
	platformdocker "github.com/lazuale/espocrm-ops/internal/platform/docker"
	platformfs "github.com/lazuale/espocrm-ops/internal/platform/fs"
	platformlocks "github.com/lazuale/espocrm-ops/internal/platform/locks"
	backupusecase "github.com/lazuale/espocrm-ops/internal/usecase/backup"
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
	DBBackup        string
	FilesBackup     string
	NoSnapshot      bool
	NoStart         bool
	SkipHTTPProbe   bool
	TimeoutSeconds  int
}

type PlanStep struct {
	Code    string
	Status  string
	Summary string
	Details string
	Action  string
}

type RollbackPlan struct {
	Scope           string
	ProjectDir      string
	ComposeFile     string
	EnvFile         string
	ComposeProject  string
	BackupRoot      string
	SiteURL         string
	TimeoutSeconds  int
	SnapshotEnabled bool
	NoStart         bool
	SkipHTTPProbe   bool
	SelectionMode   string
	SelectedPrefix  string
	SelectedStamp   string
	ManifestTXT     string
	ManifestJSON    string
	DBBackup        string
	FilesBackup     string
	Warnings        []string
	Steps           []PlanStep
	Recovery        operationusecase.RecoveryInfo
}

type resolvedTarget struct {
	SelectionMode string
	Prefix        string
	Stamp         string
	ManifestTXT   string
	ManifestJSON  string
	DBBackup      string
	FilesBackup   string
}

type planIssue struct {
	Summary string
	Details string
	Action  string
}

func BuildPlan(req PlanRequest) (RollbackPlan, error) {
	plan := RollbackPlan{
		Scope:           strings.TrimSpace(req.Scope),
		ProjectDir:      filepath.Clean(req.ProjectDir),
		ComposeFile:     filepath.Clean(req.ComposeFile),
		EnvFile:         plannedEnvFilePath(filepath.Clean(req.ProjectDir), strings.TrimSpace(req.Scope), strings.TrimSpace(req.EnvFileOverride)),
		TimeoutSeconds:  req.TimeoutSeconds,
		SnapshotEnabled: !req.NoSnapshot,
		NoStart:         req.NoStart,
		SkipHTTPProbe:   req.SkipHTTPProbe,
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
		return plan, apperr.Wrap(apperr.KindInternal, "rollback_plan_failed", err)
	}

	env, envErr := platformconfig.LoadOperationEnv(plan.ProjectDir, plan.Scope, strings.TrimSpace(req.EnvFileOverride), strings.TrimSpace(req.EnvContourHint))
	if envErr == nil {
		plan.EnvFile = env.FilePath
		plan.ComposeProject = env.ComposeProject()
		plan.BackupRoot = platformconfig.ResolveProjectPath(plan.ProjectDir, env.BackupRoot())
		plan.SiteURL = strings.TrimSpace(env.Value("SITE_URL"))
	}

	target, selectionIssues, selectionWarnings := resolveTarget(plan.BackupRoot, envErr, req)
	if len(selectionIssues) == 0 {
		plan.SelectionMode = target.SelectionMode
		plan.SelectedPrefix = target.Prefix
		plan.SelectedStamp = target.Stamp
		plan.ManifestTXT = target.ManifestTXT
		plan.ManifestJSON = target.ManifestJSON
		plan.DBBackup = target.DBBackup
		plan.FilesBackup = target.FilesBackup
	}

	plan.Warnings = collectPlanWarnings(report.Checks, plan, selectionWarnings)

	targetStep := buildTargetSelectionStep(plan, selectionIssues)
	plan.Steps = append(plan.Steps, targetStep)

	runtimeStep, runtimeWarnings := buildRuntimePrepareStep(plan, report.Checks)
	plan.Steps = append(plan.Steps, runtimeStep)
	plan.Warnings = append(plan.Warnings, runtimeWarnings...)

	snapshotStep := buildSnapshotStep(plan, env, envErr, runtimeStep)
	plan.Steps = append(plan.Steps, snapshotStep)

	dbStep, dbWarnings := buildDBRestoreStep(plan, env, envErr, runtimeStep, targetStep)
	plan.Steps = append(plan.Steps, dbStep)
	plan.Warnings = append(plan.Warnings, dbWarnings...)

	filesStep, filesWarnings := buildFilesRestoreStep(plan, env, envErr, runtimeStep, targetStep)
	plan.Steps = append(plan.Steps, filesStep)
	plan.Warnings = append(plan.Warnings, filesWarnings...)

	runtimeReturnStep := buildRuntimeReturnStep(plan, env, envErr, runtimeStep)
	plan.Steps = append(plan.Steps, runtimeReturnStep)
	plan.Warnings = dedupeStrings(plan.Warnings)

	return plan, nil
}

func (p RollbackPlan) Ready() bool {
	for _, step := range p.Steps {
		switch step.Status {
		case PlanStatusBlocked, PlanStatusUnknown:
			return false
		}
	}

	return true
}

func (p RollbackPlan) Counts() (wouldRun, skipped, blocked, unknown int) {
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

func buildTargetSelectionStep(plan RollbackPlan, issues []planIssue) PlanStep {
	if len(issues) != 0 {
		return PlanStep{
			Code:    "target_selection",
			Status:  PlanStatusBlocked,
			Summary: "Rollback target selection is blocked",
			Details: formatIssues(issues),
			Action:  firstIssueAction(issues, "Resolve the backup target selection error before running rollback."),
		}
	}

	summary := "Rollback would use the latest valid backup set"
	details := fmt.Sprintf("Would use prefix %s at %s with manifest %s.", plan.SelectedPrefix, plan.SelectedStamp, plan.ManifestJSON)
	if plan.SelectionMode == "explicit" {
		summary = "Rollback would use the explicitly selected backup set"
		details = fmt.Sprintf("Would use DB backup %s and files backup %s.", plan.DBBackup, plan.FilesBackup)
		if strings.TrimSpace(plan.ManifestJSON) != "" {
			details += fmt.Sprintf(" Matching manifest %s is available under BACKUP_ROOT.", plan.ManifestJSON)
		}
	}

	return PlanStep{
		Code:    "target_selection",
		Status:  PlanStatusWouldRun,
		Summary: summary,
		Details: details,
	}
}

func buildRuntimePrepareStep(plan RollbackPlan, checks []doctorusecase.Check) (PlanStep, []string) {
	blockers := blockingDoctorChecks(checks)
	if len(blockers) != 0 {
		return PlanStep{
			Code:    "runtime_prepare",
			Status:  PlanStatusBlocked,
			Summary: "Rollback runtime preparation is blocked",
			Details: formatDoctorChecks(blockers),
			Action:  firstDoctorAction(blockers, "Resolve the blocking rollback prerequisites before running rollback."),
		}, nil
	}

	cfg := platformdocker.ComposeConfig{
		ProjectDir:  plan.ProjectDir,
		ComposeFile: plan.ComposeFile,
		EnvFile:     plan.EnvFile,
	}

	dbState, err := platformdocker.ComposeServiceStateFor(cfg, "db")
	if err != nil {
		return PlanStep{
			Code:    "runtime_prepare",
			Status:  PlanStatusUnknown,
			Summary: "Rollback runtime preparation could not be planned completely",
			Details: fmt.Sprintf("Could not inspect the db service state: %v", err),
			Action:  "Check Docker access for this contour and rerun the rollback dry-run plan.",
		}, nil
	}

	services, err := platformdocker.ComposeRunningServices(cfg)
	if err != nil {
		return PlanStep{
			Code:    "runtime_prepare",
			Status:  PlanStatusUnknown,
			Summary: "Rollback runtime preparation could not be planned completely",
			Details: fmt.Sprintf("Could not inspect the running services: %v", err),
			Action:  "Check Docker access for this contour and rerun the rollback dry-run plan.",
		}, nil
	}

	warnings := []string{}
	for _, check := range checks {
		if check.Code != "running_services" || check.Status != "fail" {
			continue
		}
		warnings = append(warnings, formatCheckWarning("Current runtime health is degraded: ", check))
	}

	dbMode := "The db service is already running and would be reused for rollback."
	switch dbState.Status {
	case "", "exited", "dead", "created", "removing":
		dbMode = "The db service is not running now, so rollback would start it temporarily before restore."
	case "unhealthy":
		warnings = append(warnings, "The db service currently reports unhealthy; rollback would still wait for it to become ready before restore.")
		dbMode = "The db service currently reports unhealthy and rollback would wait for it to become ready before restore."
	default:
		if strings.TrimSpace(dbState.Status) != "" && dbState.Status != "running" && dbState.Status != "healthy" {
			warnings = append(warnings, fmt.Sprintf("The db service currently reports %s; rollback would still wait for it to become ready before restore.", dbState.Status))
			dbMode = fmt.Sprintf("The db service currently reports %s and rollback would wait for it to become ready before restore.", dbState.Status)
		}
	}

	appServices := applicationServices(services)
	appMode := "Application services are already stopped."
	if len(appServices) != 0 {
		appMode = fmt.Sprintf("Would stop the running application services: %s.", strings.Join(appServices, ", "))
	}

	return PlanStep{
		Code:    "runtime_prepare",
		Status:  PlanStatusWouldRun,
		Summary: "Runtime and container preparation would run",
		Details: fmt.Sprintf("%s %s Readiness waits would share a %d second timeout budget.", dbMode, appMode, plan.TimeoutSeconds),
	}, warnings
}

func buildSnapshotStep(plan RollbackPlan, env platformconfig.OperationEnv, envErr error, runtimeStep PlanStep) PlanStep {
	if !plan.SnapshotEnabled {
		return PlanStep{
			Code:    "snapshot_recovery_point",
			Status:  PlanStatusSkipped,
			Summary: "Emergency recovery-point creation would be skipped",
			Details: "Rollback would skip the emergency snapshot because of --no-snapshot.",
		}
	}

	if runtimeStep.Status != PlanStatusWouldRun {
		return blockedDownstreamStep(
			"snapshot_recovery_point",
			"The emergency recovery-point step would not run",
			"Runtime preparation must be ready before rollback can capture an emergency snapshot.",
			runtimeStep,
		)
	}

	if envErr != nil {
		return PlanStep{
			Code:    "snapshot_recovery_point",
			Status:  PlanStatusBlocked,
			Summary: "Emergency recovery-point creation is blocked",
			Details: fmt.Sprintf("The rollback plan could not load the contour env file: %v", envErr),
			Action:  "Resolve the env-file error before running rollback.",
		}
	}

	namePrefix := strings.TrimSpace(env.Value("BACKUP_NAME_PREFIX"))
	if namePrefix == "" {
		namePrefix = strings.TrimSpace(env.ComposeProject())
	}

	return PlanStep{
		Code:    "snapshot_recovery_point",
		Status:  PlanStatusWouldRun,
		Summary: "An emergency recovery point would be created",
		Details: fmt.Sprintf("Would write a pre-rollback recovery point under %s using prefix %s before any destructive restore step.", plan.BackupRoot, namePrefix),
	}
}

func buildDBRestoreStep(plan RollbackPlan, env platformconfig.OperationEnv, envErr error, runtimeStep, targetStep PlanStep) (PlanStep, []string) {
	if targetStep.Status != PlanStatusWouldRun {
		return blockedDownstreamStep(
			"db_restore",
			"Database restore would not run",
			"Rollback target selection must succeed before the database restore can be planned.",
			targetStep,
		), nil
	}
	if runtimeStep.Status != PlanStatusWouldRun {
		return blockedDownstreamStep(
			"db_restore",
			"Database restore would not run",
			"Runtime preparation must be ready before the database restore can run.",
			runtimeStep,
		), nil
	}
	if envErr != nil {
		return PlanStep{
			Code:    "db_restore",
			Status:  PlanStatusBlocked,
			Summary: "Database restore is blocked",
			Details: fmt.Sprintf("The rollback plan could not load the contour env file: %v", envErr),
			Action:  "Resolve the env-file error before running rollback.",
		}, nil
	}

	readiness, err := platformlocks.CheckRestoreDBReadiness()
	if err != nil {
		return PlanStep{
			Code:    "db_restore",
			Status:  PlanStatusUnknown,
			Summary: "Database restore could not be planned completely",
			Details: fmt.Sprintf("Could not inspect the restore-db lock: %v", err),
			Action:  "Check the temporary lock directory permissions and rerun the rollback dry-run plan.",
		}, nil
	}
	if readiness.State == platformlocks.LockActive {
		return PlanStep{
			Code:    "db_restore",
			Status:  PlanStatusBlocked,
			Summary: "Database restore is blocked by another restore-db operation",
			Details: readiness.MetadataPath,
			Action:  "Wait for the active restore-db operation to finish before running rollback.",
		}, nil
	}

	warnings := []string{}
	if readiness.State == platformlocks.LockStale {
		warnings = append(warnings, fmt.Sprintf("A stale restore-db lock file is present at %s; rollback would rewrite it when execution starts.", readiness.MetadataPath))
	}

	if strings.TrimSpace(env.Value("DB_NAME")) == "" || strings.TrimSpace(env.Value("DB_USER")) == "" || strings.TrimSpace(env.Value("DB_PASSWORD")) == "" || strings.TrimSpace(env.Value("DB_ROOT_PASSWORD")) == "" {
		return PlanStep{
			Code:    "db_restore",
			Status:  PlanStatusBlocked,
			Summary: "Database restore is blocked by missing database credentials",
			Details: "Rollback requires DB_NAME, DB_USER, DB_PASSWORD, and DB_ROOT_PASSWORD to be present in the contour env file.",
			Action:  fmt.Sprintf("Populate the required database credentials in %s before running rollback.", plan.EnvFile),
		}, warnings
	}

	return PlanStep{
		Code:    "db_restore",
		Status:  PlanStatusWouldRun,
		Summary: "Database restore would run",
		Details: fmt.Sprintf("Would destructively reset database %s from %s using the contour db service.", env.Value("DB_NAME"), plan.DBBackup),
	}, warnings
}

func buildFilesRestoreStep(plan RollbackPlan, env platformconfig.OperationEnv, envErr error, runtimeStep, targetStep PlanStep) (PlanStep, []string) {
	if targetStep.Status != PlanStatusWouldRun {
		return blockedDownstreamStep(
			"files_restore",
			"Files restore would not run",
			"Rollback target selection must succeed before the files restore can be planned.",
			targetStep,
		), nil
	}
	if runtimeStep.Status != PlanStatusWouldRun {
		return blockedDownstreamStep(
			"files_restore",
			"Files restore would not run",
			"Runtime preparation must be ready before the files restore can run.",
			runtimeStep,
		), nil
	}
	if envErr != nil {
		return PlanStep{
			Code:    "files_restore",
			Status:  PlanStatusBlocked,
			Summary: "Files restore is blocked",
			Details: fmt.Sprintf("The rollback plan could not load the contour env file: %v", envErr),
			Action:  "Resolve the env-file error before running rollback.",
		}, nil
	}

	readiness, err := platformlocks.CheckRestoreFilesReadiness()
	if err != nil {
		return PlanStep{
			Code:    "files_restore",
			Status:  PlanStatusUnknown,
			Summary: "Files restore could not be planned completely",
			Details: fmt.Sprintf("Could not inspect the restore-files lock: %v", err),
			Action:  "Check the temporary lock directory permissions and rerun the rollback dry-run plan.",
		}, nil
	}
	if readiness.State == platformlocks.LockActive {
		return PlanStep{
			Code:    "files_restore",
			Status:  PlanStatusBlocked,
			Summary: "Files restore is blocked by another restore-files operation",
			Details: readiness.MetadataPath,
			Action:  "Wait for the active restore-files operation to finish before running rollback.",
		}, nil
	}

	warnings := []string{}
	if readiness.State == platformlocks.LockStale {
		warnings = append(warnings, fmt.Sprintf("A stale restore-files lock file is present at %s; rollback would rewrite it when execution starts.", readiness.MetadataPath))
	}

	targetDir := platformconfig.ResolveProjectPath(plan.ProjectDir, env.ESPOStorageDir())
	parentDetails, targetIssue := inspectFilesTarget(targetDir, plan.FilesBackup)
	if targetIssue != nil {
		return PlanStep{
			Code:    "files_restore",
			Status:  PlanStatusBlocked,
			Summary: targetIssue.Summary,
			Details: targetIssue.Details,
			Action:  targetIssue.Action,
		}, warnings
	}

	return PlanStep{
		Code:    "files_restore",
		Status:  PlanStatusWouldRun,
		Summary: "Files restore would run",
		Details: fmt.Sprintf("Would replace %s from %s. %s", targetDir, plan.FilesBackup, parentDetails),
	}, warnings
}

func buildRuntimeReturnStep(plan RollbackPlan, env platformconfig.OperationEnv, envErr error, runtimeStep PlanStep) PlanStep {
	if plan.NoStart {
		return PlanStep{
			Code:    "runtime_return",
			Status:  PlanStatusSkipped,
			Summary: "Contour restart and readiness checks would be skipped",
			Details: "Rollback would leave the contour stopped because of --no-start.",
		}
	}
	if runtimeStep.Status != PlanStatusWouldRun {
		return blockedDownstreamStep(
			"runtime_return",
			"Contour restart would not run",
			"Runtime preparation must be ready before the contour can be returned to service.",
			runtimeStep,
		)
	}
	if envErr != nil {
		return PlanStep{
			Code:    "runtime_return",
			Status:  PlanStatusBlocked,
			Summary: "Contour restart is blocked",
			Details: fmt.Sprintf("The rollback plan could not load the contour env file: %v", envErr),
			Action:  "Resolve the env-file error before running rollback.",
		}
	}

	details := fmt.Sprintf("Would run docker compose up -d and wait up to %d seconds for the contour to become ready.", plan.TimeoutSeconds)
	if plan.SkipHTTPProbe {
		details += " The final HTTP probe would be skipped because of --skip-http-probe."
	} else {
		siteURL := strings.TrimSpace(env.Value("SITE_URL"))
		if siteURL == "" {
			return PlanStep{
				Code:    "runtime_return",
				Status:  PlanStatusBlocked,
				Summary: "Contour restart is blocked by missing SITE_URL",
				Details: "The final rollback readiness probe needs SITE_URL unless --skip-http-probe is used.",
				Action:  fmt.Sprintf("Set SITE_URL in %s or rerun intentionally with --skip-http-probe.", plan.EnvFile),
			}
		}
		if _, err := url.ParseRequestURI(siteURL); err != nil {
			return PlanStep{
				Code:    "runtime_return",
				Status:  PlanStatusBlocked,
				Summary: "Contour restart is blocked by an invalid SITE_URL",
				Details: err.Error(),
				Action:  fmt.Sprintf("Fix SITE_URL in %s or rerun intentionally with --skip-http-probe.", plan.EnvFile),
			}
		}
		details += fmt.Sprintf(" The final HTTP probe would check %s.", siteURL)
	}

	return PlanStep{
		Code:    "runtime_return",
		Status:  PlanStatusWouldRun,
		Summary: "Contour restart and readiness checks would run",
		Details: details,
	}
}

func resolveTarget(backupRoot string, envErr error, req PlanRequest) (resolvedTarget, []planIssue, []string) {
	if strings.TrimSpace(req.DBBackup) != "" || strings.TrimSpace(req.FilesBackup) != "" {
		return resolveExplicitTarget(backupRoot, req.DBBackup, req.FilesBackup)
	}

	if envErr != nil {
		return resolvedTarget{}, []planIssue{{
			Summary: "Automatic rollback target selection could not resolve BACKUP_ROOT",
			Details: envErr.Error(),
			Action:  "Fix the contour env file or pass an explicit --db-backup and --files-backup pair.",
		}}, nil
	}
	if strings.TrimSpace(backupRoot) == "" {
		return resolvedTarget{}, []planIssue{{
			Summary: "Automatic rollback target selection needs BACKUP_ROOT",
			Details: "The contour env file resolved BACKUP_ROOT to a blank path.",
			Action:  "Set BACKUP_ROOT in the contour env file or pass an explicit --db-backup and --files-backup pair.",
		}}, nil
	}

	manifestPath, err := backupusecase.LatestCompleteManifest(backupRoot)
	if err != nil {
		return resolvedTarget{}, []planIssue{{
			Summary: "Automatic rollback target selection could not find a valid backup set",
			Details: err.Error(),
			Action:  "Create or repair a coherent backup set under BACKUP_ROOT, or pass an explicit --db-backup and --files-backup pair.",
		}}, nil
	}

	info, err := backupusecase.VerifyDetailed(backupusecase.VerifyRequest{
		ManifestPath: manifestPath,
	})
	if err != nil {
		return resolvedTarget{}, []planIssue{{
			Summary: "The selected rollback manifest is not valid",
			Details: err.Error(),
			Action:  "Repair the selected backup set or choose an explicit rollback pair.",
		}}, nil
	}

	group, _, err := domainbackup.ParseManifestName(manifestPath)
	if err != nil {
		return resolvedTarget{}, []planIssue{{
			Summary: "The selected rollback manifest name is unsupported",
			Details: err.Error(),
			Action:  "Rename the manifest to the canonical backup-set pattern or choose an explicit rollback pair.",
		}}, nil
	}

	set := domainbackup.BuildBackupSet(backupRoot, group.Prefix, group.Stamp)
	manifestTXT := ""
	if _, err := os.Stat(set.ManifestTXT.Path); err == nil {
		manifestTXT = set.ManifestTXT.Path
	}

	return resolvedTarget{
		SelectionMode: "auto_latest_valid",
		Prefix:        group.Prefix,
		Stamp:         group.Stamp,
		ManifestTXT:   manifestTXT,
		ManifestJSON:  info.ManifestPath,
		DBBackup:      info.DBBackupPath,
		FilesBackup:   info.FilesPath,
	}, nil, nil
}

func resolveExplicitTarget(backupRoot, dbPath, filesPath string) (resolvedTarget, []planIssue, []string) {
	dbPath = filepath.Clean(dbPath)
	filesPath = filepath.Clean(filesPath)

	if err := backupstore.VerifyDirectDBBackup(dbPath); err != nil {
		return resolvedTarget{}, []planIssue{{
			Summary: "The explicit database backup is not valid",
			Details: err.Error(),
			Action:  "Choose a readable .sql.gz backup with a valid .sha256 sidecar.",
		}}, nil
	}
	if err := backupstore.VerifyDirectFilesBackup(filesPath); err != nil {
		return resolvedTarget{}, []planIssue{{
			Summary: "The explicit files backup is not valid",
			Details: err.Error(),
			Action:  "Choose a readable .tar.gz backup with a valid .sha256 sidecar.",
		}}, nil
	}

	dbGroup, err := domainbackup.ParseDBBackupName(dbPath)
	if err != nil {
		return resolvedTarget{}, []planIssue{{
			Summary: "The explicit database backup name is unsupported",
			Details: err.Error(),
			Action:  "Rename the database backup to the canonical pattern or use a supported backup set.",
		}}, nil
	}
	filesGroup, err := domainbackup.ParseFilesBackupName(filesPath)
	if err != nil {
		return resolvedTarget{}, []planIssue{{
			Summary: "The explicit files backup name is unsupported",
			Details: err.Error(),
			Action:  "Rename the files backup to the canonical pattern or use a supported backup set.",
		}}, nil
	}
	if dbGroup != filesGroup {
		return resolvedTarget{}, []planIssue{{
			Summary: "The explicit database and files backups do not belong to the same backup set",
			Details: fmt.Sprintf("Database backup resolves to %s %s, but files backup resolves to %s %s.", dbGroup.Prefix, dbGroup.Stamp, filesGroup.Prefix, filesGroup.Stamp),
			Action:  "Pass a DB backup and files backup from the same backup set.",
		}}, nil
	}

	target := resolvedTarget{
		SelectionMode: "explicit",
		Prefix:        dbGroup.Prefix,
		Stamp:         dbGroup.Stamp,
		DBBackup:      dbPath,
		FilesBackup:   filesPath,
	}

	warnings := []string{}
	if strings.TrimSpace(backupRoot) != "" {
		set := domainbackup.BuildBackupSet(backupRoot, dbGroup.Prefix, dbGroup.Stamp)
		if _, err := os.Stat(set.ManifestJSON.Path); err == nil {
			if info, verifyErr := backupusecase.VerifyDetailed(backupusecase.VerifyRequest{ManifestPath: set.ManifestJSON.Path}); verifyErr == nil {
				target.ManifestJSON = info.ManifestPath
				if _, txtErr := os.Stat(set.ManifestTXT.Path); txtErr == nil {
					target.ManifestTXT = set.ManifestTXT.Path
				}
			} else {
				warnings = append(warnings, fmt.Sprintf("The matching manifest under BACKUP_ROOT did not verify cleanly: %v. Rollback would still use the explicit backup archives directly.", verifyErr))
			}
		}
	}

	return target, nil, warnings
}

func inspectFilesTarget(targetDir, filesBackup string) (string, *planIssue) {
	size, err := platformfs.EnsureNonEmptyFile("files backup", filesBackup)
	if err != nil {
		return "", &planIssue{
			Summary: "Files restore could not inspect the selected files backup",
			Details: err.Error(),
			Action:  "Choose a readable files backup archive before running rollback.",
		}
	}

	parent := filepath.Dir(targetDir)
	readiness, err := platformfs.InspectDirReadiness(parent, 0, false)
	if err != nil {
		return "", &planIssue{
			Summary: "Files restore target parent is not ready",
			Details: err.Error(),
			Action:  fmt.Sprintf("Adjust permissions for %s or choose a different target path.", parent),
		}
	}
	if !readiness.Writable {
		target := readiness.ProbePath
		if target == "" {
			target = readiness.Path
		}
		return "", &planIssue{
			Summary: "Files restore target parent is not writable",
			Details: fmt.Sprintf("%s is not writable", target),
			Action:  fmt.Sprintf("Adjust permissions for %s before running rollback.", target),
		}
	}

	probePath := readiness.ProbePath
	if probePath == "" {
		probePath = parent
	}
	if err := platformfs.EnsureFreeSpace(probePath, uint64(size)); err != nil {
		return "", &planIssue{
			Summary: "Files restore target parent is below the required free-space threshold",
			Details: err.Error(),
			Action:  fmt.Sprintf("Free space under %s before running rollback.", probePath),
		}
	}

	if readiness.Exists {
		return fmt.Sprintf("Parent directory %s is writable and has enough free space for the selected files backup.", readiness.Path), nil
	}

	return fmt.Sprintf("Parent directory %s does not exist yet, but %s is writable and has enough free space to create it during rollback.", readiness.Path, probePath), nil
}

func collectPlanWarnings(checks []doctorusecase.Check, plan RollbackPlan, extra []string) []string {
	warnings := []string{}
	for _, check := range checks {
		switch check.Status {
		case "warn":
			warnings = append(warnings, formatCheckWarning("", check))
		case "fail":
			if !doctorCheckBlocksRollback(check) {
				warnings = append(warnings, formatCheckWarning("Current state is degraded: ", check))
			}
		}
	}
	if !plan.SnapshotEnabled {
		warnings = append(warnings, "Rollback would skip the emergency recovery point because of --no-snapshot.")
	}
	if plan.NoStart {
		warnings = append(warnings, "Rollback would leave the contour stopped because of --no-start.")
	}
	if plan.SkipHTTPProbe {
		warnings = append(warnings, "Rollback would skip the final HTTP probe because of --skip-http-probe.")
	}
	warnings = append(warnings, extra...)
	return dedupeStrings(warnings)
}

func blockingDoctorChecks(checks []doctorusecase.Check) []doctorusecase.Check {
	items := make([]doctorusecase.Check, 0, len(checks))
	for _, check := range checks {
		if check.Status != "fail" {
			continue
		}
		if doctorCheckBlocksRollback(check) {
			items = append(items, check)
		}
	}

	return items
}

func doctorCheckBlocksRollback(check doctorusecase.Check) bool {
	switch check.Code {
	case "compose_file", "env_resolution", "env_contract", "db_storage_dir", "espo_storage_dir", "backup_root", "shared_operation_lock", "maintenance_lock", "docker_cli", "docker_daemon", "docker_compose", "compose_config":
		return true
	default:
		return false
	}
}

func blockedDownstreamStep(code, summary, details string, blocker PlanStep) PlanStep {
	blocked := details
	if strings.TrimSpace(blocker.Details) != "" {
		blocked += " " + blocker.Details
	}
	return PlanStep{
		Code:    code,
		Status:  PlanStatusBlocked,
		Summary: summary,
		Details: blocked,
		Action:  blocker.Action,
	}
}

func applicationServices(services []string) []string {
	items := make([]string, 0, len(services))
	for _, service := range services {
		if service == "" || service == "db" {
			continue
		}
		items = append(items, service)
	}

	return items
}

func plannedEnvFilePath(projectDir, scope, override string) string {
	if strings.TrimSpace(override) != "" {
		return filepath.Clean(override)
	}

	return filepath.Join(projectDir, ".env."+scope)
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

func formatIssues(issues []planIssue) string {
	parts := make([]string, 0, len(issues))
	for _, issue := range issues {
		part := issue.Summary
		if strings.TrimSpace(issue.Details) != "" {
			part += ": " + strings.TrimSpace(issue.Details)
		}
		parts = append(parts, part)
	}

	return strings.Join(parts, "; ")
}

func firstIssueAction(issues []planIssue, fallback string) string {
	for _, issue := range issues {
		if strings.TrimSpace(issue.Action) != "" {
			return issue.Action
		}
	}

	return fallback
}

func dedupeStrings(values []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}

	return out
}
