package supportbundle

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/lazuale/espocrm-ops/internal/contract/apperr"
	platformconfig "github.com/lazuale/espocrm-ops/internal/platform/config"
	platformdocker "github.com/lazuale/espocrm-ops/internal/platform/docker"
	platformfs "github.com/lazuale/espocrm-ops/internal/platform/fs"
	platformlocks "github.com/lazuale/espocrm-ops/internal/platform/locks"
	backupusecase "github.com/lazuale/espocrm-ops/internal/usecase/backup"
	doctorusecase "github.com/lazuale/espocrm-ops/internal/usecase/doctor"
	journalusecase "github.com/lazuale/espocrm-ops/internal/usecase/journal"
)

const (
	SupportBundleKind    = "support_bundle"
	SupportBundleVersion = 1

	defaultSupportBundleTailLines     = 300
	defaultSupportBundleRetentionDays = 14
	supportBundleHistoryLimit         = 10

	supportBundleSectionStatusCompleted = "completed"
	supportBundleSectionStatusSkipped   = "skipped"
	supportBundleSectionStatusFailed    = "failed"
	supportBundleSectionStatusNotRun    = "not_run"
)

type Request struct {
	Scope           string
	ProjectDir      string
	ComposeFile     string
	EnvFileOverride string
	EnvContourHint  string
	JournalDir      string
	OutputPath      string
	TailLines       int
	Now             time.Time
	LogWriter       io.Writer
}

type Section struct {
	Code    string   `json:"code"`
	Status  string   `json:"status"`
	Summary string   `json:"summary"`
	Details string   `json:"details,omitempty"`
	Action  string   `json:"action,omitempty"`
	Files   []string `json:"files,omitempty"`
}

type Info struct {
	Scope            string
	ProjectDir       string
	ComposeFile      string
	EnvFile          string
	BackupRoot       string
	OutputPath       string
	GeneratedAt      string
	TailLines        int
	RetentionDays    int
	BundleKind       string
	BundleVersion    int
	IncludedSections []string
	OmittedSections  []string
	Warnings         []string
	Sections         []Section
}

type bundleManifest struct {
	BundleKind       string                 `json:"bundle_kind"`
	BundleVersion    int                    `json:"bundle_version"`
	GeneratedAt      string                 `json:"generated_at"`
	Scope            string                 `json:"scope"`
	TailLines        int                    `json:"tail_lines"`
	IncludedSections []string               `json:"included_sections"`
	OmittedSections  []string               `json:"omitted_sections,omitempty"`
	Warnings         []string               `json:"warnings,omitempty"`
	Artifacts        bundleManifestArtifact `json:"artifacts"`
	Sections         []Section              `json:"sections"`
}

type bundleManifestArtifact struct {
	ProjectDir  string `json:"project_dir"`
	ComposeFile string `json:"compose_file"`
	EnvFile     string `json:"env_file"`
	BackupRoot  string `json:"backup_root"`
	BundlePath  string `json:"bundle_path"`
}

func Generate(req Request) (info Info, err error) {
	info = Info{
		Scope:         strings.TrimSpace(req.Scope),
		ProjectDir:    filepath.Clean(req.ProjectDir),
		ComposeFile:   filepath.Clean(req.ComposeFile),
		OutputPath:    strings.TrimSpace(req.OutputPath),
		TailLines:     req.TailLines,
		BundleKind:    SupportBundleKind,
		BundleVersion: SupportBundleVersion,
	}
	if info.TailLines <= 0 {
		info.TailLines = defaultSupportBundleTailLines
	}

	now := req.Now.UTC()
	if req.Now.IsZero() {
		now = time.Now().UTC()
	}
	info.GeneratedAt = now.Format(time.RFC3339)

	env, err := platformconfig.LoadOperationEnv(
		info.ProjectDir,
		info.Scope,
		strings.TrimSpace(req.EnvFileOverride),
		strings.TrimSpace(req.EnvContourHint),
	)
	if err != nil {
		failSection(&info, "operation_preflight", "Support bundle preflight failed", err.Error(), "Resolve env resolution or contour readiness before rerunning support-bundle.")
		notRunSection(&info, "archive_create", "Support bundle archive was not created because preflight failed")
		return info, wrapSupportBundleEnvError(err)
	}
	info.EnvFile = env.FilePath
	info.BackupRoot = platformconfig.ResolveProjectPath(info.ProjectDir, env.BackupRoot())

	if err := ensureSupportRuntimeDirs(info.ProjectDir, env); err != nil {
		failSection(&info, "operation_preflight", "Support bundle preflight failed", err.Error(), "Resolve runtime filesystem readiness before rerunning support-bundle.")
		notRunSection(&info, "archive_create", "Support bundle archive was not created because preflight failed")
		return info, apperr.Wrap(apperr.KindIO, "support_bundle_failed", err)
	}

	opLock, err := platformlocks.AcquireSharedOperationLock(info.ProjectDir, "support-bundle", req.LogWriter)
	if err != nil {
		failSection(&info, "operation_preflight", "Support bundle preflight failed", err.Error(), "Wait for the active toolkit operation to finish before rerunning support-bundle.")
		notRunSection(&info, "archive_create", "Support bundle archive was not created because preflight failed")
		return info, wrapSupportBundleLockError(err)
	}
	defer func() {
		if opLock == nil {
			return
		}
		if releaseErr := opLock.Release(); releaseErr != nil {
			info.Warnings = append(info.Warnings, fmt.Sprintf("failed to release the shared operations lock: %v", releaseErr))
			info.Warnings = dedupeStrings(info.Warnings)
		}
	}()

	info.OutputPath = resolveSupportOutputPath(info.OutputPath, info.BackupRoot, resolvedSupportNamePrefix(env), now)
	info.RetentionDays, err = resolvedSupportRetentionDays(env)
	if err != nil {
		info.Warnings = append(info.Warnings, err.Error())
	}

	includeSection(
		&info,
		"operation_preflight",
		"Support bundle preflight completed",
		"Loaded the resolved contour env file, prepared runtime directories, and acquired the shared operations lock.",
		nil,
	)

	tmpDir, err := os.MkdirTemp(info.ProjectDir, ".support."+info.Scope+".")
	if err != nil {
		failSection(&info, "archive_create", "Support bundle archive creation failed", err.Error(), "Resolve temporary workspace creation before rerunning support-bundle.")
		return info, apperr.Wrap(apperr.KindIO, "support_bundle_failed", err)
	}
	defer func() {
		_ = os.RemoveAll(tmpDir)
	}()

	redactedEnv, err := os.ReadFile(info.EnvFile)
	if err != nil {
		failSection(&info, "env_redacted", "Redacted env file collection failed", err.Error(), "Resolve env file readability before rerunning support-bundle.")
		failSection(&info, "archive_create", "Support bundle archive creation failed", "The required redacted env file could not be written.", "Resolve the failed section collection first and rerun support-bundle.")
		return info, apperr.Wrap(apperr.KindIO, "support_bundle_failed", err)
	}
	if err := writeTextFile(tmpDir, "env.redacted", redactEnvFile(string(redactedEnv))); err != nil {
		failSection(&info, "env_redacted", "Redacted env file collection failed", err.Error(), "Resolve bundle workspace write access before rerunning support-bundle.")
		failSection(&info, "archive_create", "Support bundle archive creation failed", "The required redacted env file could not be written.", "Resolve the failed section collection first and rerun support-bundle.")
		return info, apperr.Wrap(apperr.KindIO, "support_bundle_failed", err)
	}
	includeSection(
		&info,
		"env_redacted",
		"Redacted env file included",
		"Masked secret env keys and wrote env.redacted.",
		[]string{"env.redacted"},
	)

	composeCfg := platformdocker.ComposeConfig{
		ProjectDir:  info.ProjectDir,
		ComposeFile: info.ComposeFile,
		EnvFile:     info.EnvFile,
	}

	if raw, err := platformdocker.ComposeConfigText(composeCfg); err != nil {
		info.Warnings = append(info.Warnings, fmt.Sprintf("omitted compose_config: %v", err))
		skipSection(
			&info,
			"compose_config",
			"Docker Compose config omitted",
			err.Error(),
			"Restore Docker Compose access to include the rendered runtime config in the support bundle.",
		)
	} else if err := writeTextFile(tmpDir, "compose.config.yaml", redactYAMLSecretValues(raw)); err != nil {
		failSection(&info, "compose_config", "Docker Compose config collection failed", err.Error(), "Resolve bundle workspace write access before rerunning support-bundle.")
		failSection(&info, "archive_create", "Support bundle archive creation failed", "The compose config section could not be written.", "Resolve the failed section collection first and rerun support-bundle.")
		return info, apperr.Wrap(apperr.KindIO, "support_bundle_failed", err)
	} else {
		includeSection(
			&info,
			"compose_config",
			"Docker Compose config included",
			"Redacted secret YAML keys and wrote compose.config.yaml.",
			[]string{"compose.config.yaml"},
		)
	}

	if raw, err := platformdocker.ComposePSText(composeCfg); err != nil {
		info.Warnings = append(info.Warnings, fmt.Sprintf("omitted compose_ps: %v", err))
		skipSection(
			&info,
			"compose_ps",
			"Docker Compose service status omitted",
			err.Error(),
			"Restore Docker daemon access to include current compose service status in the support bundle.",
		)
	} else if err := writeTextFile(tmpDir, "compose.ps.txt", raw); err != nil {
		failSection(&info, "compose_ps", "Docker Compose service status collection failed", err.Error(), "Resolve bundle workspace write access before rerunning support-bundle.")
		failSection(&info, "archive_create", "Support bundle archive creation failed", "The compose service status section could not be written.", "Resolve the failed section collection first and rerun support-bundle.")
		return info, apperr.Wrap(apperr.KindIO, "support_bundle_failed", err)
	} else {
		includeSection(
			&info,
			"compose_ps",
			"Docker Compose service status included",
			"Wrote compose.ps.txt for the resolved contour.",
			[]string{"compose.ps.txt"},
		)
	}

	if raw, err := platformdocker.ComposeLogsText(composeCfg, info.TailLines); err != nil {
		info.Warnings = append(info.Warnings, fmt.Sprintf("omitted compose_logs: %v", err))
		skipSection(
			&info,
			"compose_logs",
			"Docker Compose logs omitted",
			err.Error(),
			"Restore Docker daemon access to include compose logs in the support bundle.",
		)
	} else if err := writeTextFile(tmpDir, "compose.logs.txt", raw); err != nil {
		failSection(&info, "compose_logs", "Docker Compose log collection failed", err.Error(), "Resolve bundle workspace write access before rerunning support-bundle.")
		failSection(&info, "archive_create", "Support bundle archive creation failed", "The compose log section could not be written.", "Resolve the failed section collection first and rerun support-bundle.")
		return info, apperr.Wrap(apperr.KindIO, "support_bundle_failed", err)
	} else {
		includeSection(
			&info,
			"compose_logs",
			"Docker Compose logs included",
			fmt.Sprintf("Wrote compose.logs.txt with the latest %d lines per service.", info.TailLines),
			[]string{"compose.logs.txt"},
		)
	}

	if raw, err := platformdocker.DockerVersionText(); err != nil {
		info.Warnings = append(info.Warnings, fmt.Sprintf("omitted docker_version: %v", err))
		skipSection(
			&info,
			"docker_version",
			"Docker version omitted",
			err.Error(),
			"Restore Docker CLI access to include Docker version details in the support bundle.",
		)
	} else if err := writeTextFile(tmpDir, "docker.version.txt", raw); err != nil {
		failSection(&info, "docker_version", "Docker version collection failed", err.Error(), "Resolve bundle workspace write access before rerunning support-bundle.")
		failSection(&info, "archive_create", "Support bundle archive creation failed", "The Docker version section could not be written.", "Resolve the failed section collection first and rerun support-bundle.")
		return info, apperr.Wrap(apperr.KindIO, "support_bundle_failed", err)
	} else {
		includeSection(
			&info,
			"docker_version",
			"Docker version included",
			"Wrote docker.version.txt.",
			[]string{"docker.version.txt"},
		)
	}

	if raw, err := platformdocker.ComposeVersionText(); err != nil {
		info.Warnings = append(info.Warnings, fmt.Sprintf("omitted docker_compose_version: %v", err))
		skipSection(
			&info,
			"docker_compose_version",
			"Docker Compose version omitted",
			err.Error(),
			"Restore Docker Compose access to include Compose version details in the support bundle.",
		)
	} else if err := writeTextFile(tmpDir, "docker.compose.version.txt", raw); err != nil {
		failSection(&info, "docker_compose_version", "Docker Compose version collection failed", err.Error(), "Resolve bundle workspace write access before rerunning support-bundle.")
		failSection(&info, "archive_create", "Support bundle archive creation failed", "The Docker Compose version section could not be written.", "Resolve the failed section collection first and rerun support-bundle.")
		return info, apperr.Wrap(apperr.KindIO, "support_bundle_failed", err)
	} else {
		includeSection(
			&info,
			"docker_compose_version",
			"Docker Compose version included",
			"Wrote docker.compose.version.txt.",
			[]string{"docker.compose.version.txt"},
		)
	}

	doctorReport, doctorErr := doctorusecase.Diagnose(doctorusecase.Request{
		Scope:                  info.Scope,
		ProjectDir:             info.ProjectDir,
		ComposeFile:            info.ComposeFile,
		EnvFileOverride:        info.EnvFile,
		EnvContourHint:         strings.TrimSpace(req.EnvContourHint),
		PathCheckMode:          doctorusecase.PathCheckModeReadOnly,
		InheritedOperationLock: true,
	})
	if doctorErr != nil {
		info.Warnings = append(info.Warnings, fmt.Sprintf("omitted doctor: %v", doctorErr))
		skipSection(
			&info,
			"doctor",
			"Doctor report omitted",
			doctorErr.Error(),
			"Resolve doctor collection errors to include readiness output in the support bundle.",
		)
	} else {
		if err := writeJSONFile(tmpDir, "doctor.json", doctorReport); err != nil {
			failSection(&info, "doctor", "Doctor report collection failed", err.Error(), "Resolve bundle workspace write access before rerunning support-bundle.")
			failSection(&info, "archive_create", "Support bundle archive creation failed", "The doctor section could not be written.", "Resolve the failed section collection first and rerun support-bundle.")
			return info, apperr.Wrap(apperr.KindIO, "support_bundle_failed", err)
		}
		if err := writeTextFile(tmpDir, "doctor.txt", renderDoctorReportText(doctorReport)); err != nil {
			failSection(&info, "doctor", "Doctor report collection failed", err.Error(), "Resolve bundle workspace write access before rerunning support-bundle.")
			failSection(&info, "archive_create", "Support bundle archive creation failed", "The doctor section could not be written.", "Resolve the failed section collection first and rerun support-bundle.")
			return info, apperr.Wrap(apperr.KindIO, "support_bundle_failed", err)
		}
		if !doctorReport.Ready() {
			info.Warnings = append(info.Warnings, "doctor included readiness failures in the support bundle")
		}
		includeSection(
			&info,
			"doctor",
			"Doctor report included",
			"Wrote doctor.txt and doctor.json for the resolved contour.",
			[]string{"doctor.txt", "doctor.json"},
		)
	}

	history, historyErr := journalusecase.History(journalusecase.HistoryInput{
		JournalDir: strings.TrimSpace(req.JournalDir),
		Filters: journalusecase.Filters{
			Scope: info.Scope,
			Limit: supportBundleHistoryLimit,
		},
	})
	if historyErr != nil {
		info.Warnings = append(info.Warnings, fmt.Sprintf("omitted history: %v", historyErr))
		skipSection(
			&info,
			"history",
			"Recent operation history omitted",
			historyErr.Error(),
			"Repair journal filesystem access to include recent operation history in the support bundle.",
		)
	} else {
		if err := writeJSONFile(tmpDir, "history.json", history); err != nil {
			failSection(&info, "history", "Recent operation history collection failed", err.Error(), "Resolve bundle workspace write access before rerunning support-bundle.")
			failSection(&info, "archive_create", "Support bundle archive creation failed", "The history section could not be written.", "Resolve the failed section collection first and rerun support-bundle.")
			return info, apperr.Wrap(apperr.KindIO, "support_bundle_failed", err)
		}
		if err := writeTextFile(tmpDir, "history.txt", renderOperationHistoryText(history.Operations)); err != nil {
			failSection(&info, "history", "Recent operation history collection failed", err.Error(), "Resolve bundle workspace write access before rerunning support-bundle.")
			failSection(&info, "archive_create", "Support bundle archive creation failed", "The history section could not be written.", "Resolve the failed section collection first and rerun support-bundle.")
			return info, apperr.Wrap(apperr.KindIO, "support_bundle_failed", err)
		}
		info.Warnings = append(info.Warnings, journalusecase.WarningsFromReadStats(history.Stats)...)
		includeSection(
			&info,
			"history",
			"Recent operation history included",
			fmt.Sprintf("Wrote history.txt and history.json with up to %d recent operations for contour %s.", supportBundleHistoryLimit, info.Scope),
			[]string{"history.txt", "history.json"},
		)
	}

	catalog, catalogErr := backupusecase.Catalog(backupusecase.CatalogRequest{
		BackupRoot: info.BackupRoot,
		JournalDir: strings.TrimSpace(req.JournalDir),
		Limit:      0,
		Now:        now,
	})
	if catalogErr != nil {
		info.Warnings = append(info.Warnings, fmt.Sprintf("omitted backup_catalog: %v", catalogErr))
		skipSection(
			&info,
			"backup_catalog",
			"Backup catalog omitted",
			catalogErr.Error(),
			"Repair backup-root or journal access to include backup inventory in the support bundle.",
		)
	} else {
		if err := writeJSONFile(tmpDir, "backup-catalog.json", catalog); err != nil {
			failSection(&info, "backup_catalog", "Backup catalog collection failed", err.Error(), "Resolve bundle workspace write access before rerunning support-bundle.")
			failSection(&info, "archive_create", "Support bundle archive creation failed", "The backup catalog section could not be written.", "Resolve the failed section collection first and rerun support-bundle.")
			return info, apperr.Wrap(apperr.KindIO, "support_bundle_failed", err)
		}
		if err := writeTextFile(tmpDir, "backup-catalog.txt", renderBackupCatalogText(catalog)); err != nil {
			failSection(&info, "backup_catalog", "Backup catalog collection failed", err.Error(), "Resolve bundle workspace write access before rerunning support-bundle.")
			failSection(&info, "archive_create", "Support bundle archive creation failed", "The backup catalog section could not be written.", "Resolve the failed section collection first and rerun support-bundle.")
			return info, apperr.Wrap(apperr.KindIO, "support_bundle_failed", err)
		}
		info.Warnings = append(info.Warnings, backupCatalogWarnings(catalog.JournalRead)...)
		includeSection(
			&info,
			"backup_catalog",
			"Backup catalog included",
			"Wrote backup-catalog.txt and backup-catalog.json for the resolved backup root.",
			[]string{"backup-catalog.txt", "backup-catalog.json"},
		)
	}

	if err := copyOptionalLatestFile(
		&info,
		tmpDir,
		filepath.Join(info.BackupRoot, "manifests"),
		"*.manifest.json",
		"latest_manifest_json",
		"Latest JSON manifest included",
		"Latest JSON manifest omitted",
		"latest.manifest.json",
	); err != nil {
		failSection(&info, "archive_create", "Support bundle archive creation failed", "The latest JSON manifest section could not be written.", "Resolve the failed section collection first and rerun support-bundle.")
		return info, apperr.Wrap(apperr.KindIO, "support_bundle_failed", err)
	}
	if err := copyOptionalLatestFile(
		&info,
		tmpDir,
		filepath.Join(info.BackupRoot, "manifests"),
		"*.manifest.txt",
		"latest_manifest_txt",
		"Latest text manifest included",
		"Latest text manifest omitted",
		"latest.manifest.txt",
	); err != nil {
		failSection(&info, "archive_create", "Support bundle archive creation failed", "The latest text manifest section could not be written.", "Resolve the failed section collection first and rerun support-bundle.")
		return info, apperr.Wrap(apperr.KindIO, "support_bundle_failed", err)
	}

	if info.RetentionDays >= 0 {
		if cleanupErr := cleanupSupportRetention(filepath.Join(info.BackupRoot, "support"), info.RetentionDays, now); cleanupErr != nil {
			info.Warnings = append(info.Warnings, fmt.Sprintf("support bundle retention cleanup failed: %v", cleanupErr))
		}
	}
	info.Warnings = dedupeStrings(info.Warnings)

	if err := finalizeBundleMetadata(&info, tmpDir); err != nil {
		failSection(&info, "archive_create", "Support bundle archive creation failed", "The bundle summary or manifest could not be written.", "Resolve the failed section collection first and rerun support-bundle.")
		return info, apperr.Wrap(apperr.KindIO, "support_bundle_failed", err)
	}

	if err := platformfs.CreateTarGz(tmpDir, info.OutputPath); err != nil {
		failSection(&info, "archive_create", "Support bundle archive creation failed", err.Error(), "Resolve archive tool or filesystem readiness before rerunning support-bundle.")
		return info, apperr.Wrap(apperr.KindIO, "support_bundle_failed", err)
	}
	includeSection(
		&info,
		"archive_create",
		"Support bundle archive created",
		"Created the final support bundle archive at the resolved output path.",
		nil,
	)
	info.Warnings = dedupeStrings(info.Warnings)

	return info, nil
}

func ensureSupportRuntimeDirs(projectDir string, env platformconfig.OperationEnv) error {
	paths := []string{
		platformconfig.ResolveProjectPath(projectDir, env.DBStorageDir()),
		platformconfig.ResolveProjectPath(projectDir, env.ESPOStorageDir()),
		filepath.Join(platformconfig.ResolveProjectPath(projectDir, env.BackupRoot()), "db"),
		filepath.Join(platformconfig.ResolveProjectPath(projectDir, env.BackupRoot()), "files"),
		filepath.Join(platformconfig.ResolveProjectPath(projectDir, env.BackupRoot()), "locks"),
		filepath.Join(platformconfig.ResolveProjectPath(projectDir, env.BackupRoot()), "manifests"),
		filepath.Join(platformconfig.ResolveProjectPath(projectDir, env.BackupRoot()), "reports"),
		filepath.Join(platformconfig.ResolveProjectPath(projectDir, env.BackupRoot()), "support"),
	}

	for _, path := range paths {
		if err := os.MkdirAll(path, 0o755); err != nil {
			return fmt.Errorf("create runtime directory %s: %w", path, err)
		}
	}

	return nil
}

func resolvedSupportNamePrefix(env platformconfig.OperationEnv) string {
	if value := strings.TrimSpace(env.Value("BACKUP_NAME_PREFIX")); value != "" {
		return value
	}
	if value := strings.TrimSpace(env.ComposeProject()); value != "" {
		return value
	}
	return "espocrm"
}

func resolvedSupportRetentionDays(env platformconfig.OperationEnv) (int, error) {
	raw := strings.TrimSpace(env.Value("SUPPORT_RETENTION_DAYS"))
	if raw == "" {
		return defaultSupportBundleRetentionDays, nil
	}

	days, err := strconv.Atoi(raw)
	if err != nil || days < 0 {
		return -1, fmt.Errorf("support bundle retention cleanup skipped because SUPPORT_RETENTION_DAYS=%q is invalid", raw)
	}

	return days, nil
}

func resolveSupportOutputPath(raw, backupRoot, prefix string, now time.Time) string {
	if strings.TrimSpace(raw) != "" {
		return filepath.Clean(raw)
	}

	return nextSupportBundlePath(backupRoot, prefix, now)
}

func nextSupportBundlePath(backupRoot, prefix string, now time.Time) string {
	supportDir := filepath.Join(backupRoot, "support")
	stampTime := now.UTC()
	for {
		stamp := stampTime.Format("2006-01-02_15-04-05")
		path := filepath.Join(supportDir, fmt.Sprintf("%s_support_%s.tar.gz", prefix, stamp))
		if _, err := os.Stat(path); os.IsNotExist(err) {
			return path
		}
		stampTime = stampTime.Add(time.Second)
	}
}

func cleanupSupportRetention(dir string, retentionDays int, now time.Time) error {
	if retentionDays < 0 {
		return fmt.Errorf("retention days must be non-negative")
	}

	matches, err := filepath.Glob(filepath.Join(dir, "*.tar.gz"))
	if err != nil {
		return fmt.Errorf("glob support bundles: %w", err)
	}

	cutoff := now.UTC().Add(-time.Duration(retentionDays+1) * 24 * time.Hour)
	for _, match := range matches {
		info, err := os.Stat(match)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return fmt.Errorf("stat retention candidate %s: %w", match, err)
		}
		if info.IsDir() || !info.ModTime().Before(cutoff) {
			continue
		}
		if err := os.Remove(match); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove retention candidate %s: %w", match, err)
		}
	}

	return nil
}

func finalizeBundleMetadata(info *Info, tmpDir string) error {
	summaryFiles := []string{"bundle.summary.txt"}
	manifestFiles := []string{"bundle.manifest.json"}

	includeSection(
		info,
		"bundle_summary",
		"Bundle summary included",
		"Summarized included sections, omitted sections, and warnings in bundle.summary.txt.",
		summaryFiles,
	)
	includeSection(
		info,
		"bundle_manifest",
		"Bundle manifest included",
		"Recorded bundle metadata, included sections, omitted sections, and section outcomes in bundle.manifest.json.",
		manifestFiles,
	)

	info.IncludedSections, info.OmittedSections = supportBundleSectionLists(info.Sections)

	if err := writeTextFile(tmpDir, "bundle.summary.txt", renderBundleSummary(*info)); err != nil {
		info.Sections[len(info.Sections)-2].Status = supportBundleSectionStatusFailed
		info.Sections[len(info.Sections)-2].Summary = "Bundle summary collection failed"
		info.Sections[len(info.Sections)-2].Details = err.Error()
		info.Sections[len(info.Sections)-2].Action = "Resolve bundle workspace write access before rerunning support-bundle."
		info.IncludedSections, info.OmittedSections = supportBundleSectionLists(info.Sections)
		return err
	}

	manifest := bundleManifest{
		BundleKind:       info.BundleKind,
		BundleVersion:    info.BundleVersion,
		GeneratedAt:      info.GeneratedAt,
		Scope:            info.Scope,
		TailLines:        info.TailLines,
		IncludedSections: append([]string(nil), info.IncludedSections...),
		OmittedSections:  append([]string(nil), info.OmittedSections...),
		Warnings:         append([]string(nil), info.Warnings...),
		Artifacts: bundleManifestArtifact{
			ProjectDir:  info.ProjectDir,
			ComposeFile: info.ComposeFile,
			EnvFile:     info.EnvFile,
			BackupRoot:  info.BackupRoot,
			BundlePath:  info.OutputPath,
		},
		Sections: append([]Section(nil), info.Sections...),
	}
	if err := writeJSONFile(tmpDir, "bundle.manifest.json", manifest); err != nil {
		info.Sections[len(info.Sections)-1].Status = supportBundleSectionStatusFailed
		info.Sections[len(info.Sections)-1].Summary = "Bundle manifest collection failed"
		info.Sections[len(info.Sections)-1].Details = err.Error()
		info.Sections[len(info.Sections)-1].Action = "Resolve bundle workspace write access before rerunning support-bundle."
		info.IncludedSections, info.OmittedSections = supportBundleSectionLists(info.Sections)
		return err
	}

	return nil
}

func copyOptionalLatestFile(info *Info, tmpDir, dir, pattern, code, okSummary, skipSummary, destName string) error {
	path, err := latestFileInDir(dir, pattern)
	if err != nil {
		info.Warnings = append(info.Warnings, fmt.Sprintf("omitted %s: %v", code, err))
		skipSection(info, code, skipSummary, err.Error(), "")
		return nil
	}
	if path == "" {
		skipSection(info, code, skipSummary, fmt.Sprintf("No files matching %s were found.", pattern), "")
		return nil
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		info.Warnings = append(info.Warnings, fmt.Sprintf("omitted %s: %v", code, err))
		skipSection(info, code, skipSummary, err.Error(), "")
		return nil
	}
	if err := writeBytesFile(tmpDir, destName, raw); err != nil {
		failSection(info, code, okSummary, err.Error(), "Resolve bundle workspace write access before rerunning support-bundle.")
		return err
	}

	includeSection(info, code, okSummary, fmt.Sprintf("Copied the newest file matching %s into %s.", pattern, destName), []string{destName})
	return nil
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

func writeJSONFile(dir, name string, value any) error {
	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal %s: %w", name, err)
	}
	raw = append(raw, '\n')
	return writeBytesFile(dir, name, raw)
}

func writeTextFile(dir, name, text string) error {
	if !strings.HasSuffix(text, "\n") {
		text += "\n"
	}
	return writeBytesFile(dir, name, []byte(text))
}

func writeBytesFile(dir, name string, raw []byte) error {
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", name, err)
	}
	return nil
}

func includeSection(info *Info, code, summary, details string, files []string) {
	info.Sections = append(info.Sections, Section{
		Code:    code,
		Status:  supportBundleSectionStatusCompleted,
		Summary: summary,
		Details: details,
		Files:   append([]string(nil), files...),
	})
}

func skipSection(info *Info, code, summary, details, action string) {
	info.Sections = append(info.Sections, Section{
		Code:    code,
		Status:  supportBundleSectionStatusSkipped,
		Summary: summary,
		Details: details,
		Action:  action,
	})
}

func failSection(info *Info, code, summary, details, action string) {
	info.Sections = append(info.Sections, Section{
		Code:    code,
		Status:  supportBundleSectionStatusFailed,
		Summary: summary,
		Details: details,
		Action:  action,
	})
}

func notRunSection(info *Info, code, summary string) {
	info.Sections = append(info.Sections, Section{
		Code:    code,
		Status:  supportBundleSectionStatusNotRun,
		Summary: summary,
	})
}

func supportBundleSectionLists(sections []Section) ([]string, []string) {
	included := []string{}
	omitted := []string{}

	for _, section := range sections {
		if !isReportableSupportSection(section.Code) {
			continue
		}

		switch section.Status {
		case supportBundleSectionStatusCompleted:
			included = append(included, section.Code)
		case supportBundleSectionStatusSkipped:
			omitted = append(omitted, section.Code)
		}
	}

	return included, omitted
}

func isReportableSupportSection(code string) bool {
	switch code {
	case "operation_preflight", "archive_create":
		return false
	default:
		return true
	}
}

func redactEnvFile(raw string) string {
	lines := strings.Split(strings.ReplaceAll(raw, "\r\n", "\n"), "\n")
	for idx, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}

		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		if !isSecretKey(key) {
			_ = value
			continue
		}

		lines[idx] = key + "=<redacted>"
	}

	return strings.Join(lines, "\n")
}

func redactYAMLSecretValues(raw string) string {
	lines := strings.Split(strings.ReplaceAll(raw, "\r\n", "\n"), "\n")
	for idx, line := range lines {
		trimmedLeft := strings.TrimLeft(line, " \t")
		indent := line[:len(line)-len(trimmedLeft)]

		key, _, ok := strings.Cut(trimmedLeft, ":")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		if !isIdentifierKey(key) || !isSecretKey(key) {
			continue
		}

		lines[idx] = indent + key + ": <redacted>"
	}

	return strings.Join(lines, "\n")
}

func isSecretKey(key string) bool {
	key = strings.ToUpper(strings.TrimSpace(key))
	return strings.HasSuffix(key, "PASSWORD") ||
		strings.HasSuffix(key, "TOKEN") ||
		strings.HasSuffix(key, "SECRET")
}

func isIdentifierKey(key string) bool {
	if key == "" {
		return false
	}
	for idx, r := range key {
		if idx == 0 {
			if (r < 'A' || r > 'Z') && (r < 'a' || r > 'z') && r != '_' {
				return false
			}
			continue
		}
		if (r < 'A' || r > 'Z') && (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '_' {
			return false
		}
	}
	return true
}

func backupCatalogWarnings(stats backupusecase.JournalReadStats) []string {
	return journalusecase.WarningsFromReadStats(journalusecase.ReadStats{
		TotalFilesSeen: stats.TotalFilesSeen,
		LoadedEntries:  stats.LoadedEntries,
		SkippedCorrupt: stats.SkippedCorrupt,
	})
}

func dedupeStrings(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || slices.Contains(out, value) {
			continue
		}
		out = append(out, value)
	}
	return out
}

func wrapSupportBundleEnvError(err error) error {
	switch err.(type) {
	case platformconfig.MissingEnvFileError,
		platformconfig.InvalidEnvFileError,
		platformconfig.EnvParseError,
		platformconfig.MissingEnvValueError,
		platformconfig.UnsupportedContourError:
		return apperr.Wrap(apperr.KindValidation, "support_bundle_failed", err)
	default:
		return apperr.Wrap(apperr.KindIO, "support_bundle_failed", err)
	}
}

func wrapSupportBundleLockError(err error) error {
	switch err.(type) {
	case platformlocks.LegacyMetadataOnlyLockError:
		return apperr.Wrap(apperr.KindConflict, "support_bundle_failed", err)
	default:
		return apperr.Wrap(apperr.KindIO, "support_bundle_failed", err)
	}
}
