package doctor

import (
	"fmt"
	"net/url"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	platformconfig "github.com/lazuale/espocrm-ops/internal/platform/config"
	platformdocker "github.com/lazuale/espocrm-ops/internal/platform/docker"
	platformfs "github.com/lazuale/espocrm-ops/internal/platform/fs"
)

const (
	statusOK   = "ok"
	statusWarn = "warn"
	statusFail = "fail"
)

type Request struct {
	Scope           string
	ProjectDir      string
	ComposeFile     string
	EnvFileOverride string
	PathCheckMode   PathCheckMode
}

type Check struct {
	Scope   string
	Code    string
	Status  string
	Summary string
	Details string
	Action  string
}

type ScopeArtifact struct {
	Scope      string
	EnvFile    string
	BackupRoot string
}

type Report struct {
	TargetScope string
	ProjectDir  string
	ComposeFile string
	Checks      []Check
	Scopes      []ScopeArtifact
}

type dockerState struct {
	cliVersion     string
	serverVersion  string
	composeVersion string
	cliReady       bool
	daemonReady    bool
	composeReady   bool
}

func Diagnose(req Request) (Report, error) {
	pathMode := normalizePathCheckMode(req.PathCheckMode)
	report := Report{
		TargetScope: strings.TrimSpace(req.Scope),
		ProjectDir:  filepath.Clean(req.ProjectDir),
		ComposeFile: filepath.Clean(req.ComposeFile),
	}

	checkComposeFile(&report)
	checkSharedOperationLock(&report)
	docker := checkDocker(&report)

	loaded := map[string]platformconfig.OperationEnv{}
	for _, scope := range requestedScopes(report.TargetScope) {
		env, ok := diagnoseScope(&report, req, scope, docker, pathMode)
		if ok {
			loaded[scope] = env
		}
	}

	if report.TargetScope == "all" {
		prodEnv, prodOK := loaded["prod"]
		devEnv, devOK := loaded["dev"]
		if prodOK && devOK {
			checkCrossScopeIsolation(&report, report.ProjectDir, prodEnv, devEnv)
			checkCrossScopeCompatibility(&report, prodEnv, devEnv)
		}
	}

	return report, nil
}

func (r Report) Ready() bool {
	for _, check := range r.Checks {
		if check.Status == statusFail {
			return false
		}
	}

	return true
}

func (r Report) Counts() (passed, warnings, failed int) {
	for _, check := range r.Checks {
		switch check.Status {
		case statusOK:
			passed++
		case statusWarn:
			warnings++
		case statusFail:
			failed++
		}
	}

	return passed, warnings, failed
}

func requestedScopes(target string) []string {
	if target == "all" {
		return []string{"prod", "dev"}
	}

	return []string{target}
}

func diagnoseScope(report *Report, req Request, scope string, docker dockerState, pathMode PathCheckMode) (platformconfig.OperationEnv, bool) {
	env, err := platformconfig.LoadOperationEnv(report.ProjectDir, scope, req.EnvFileOverride)
	if err != nil {
		report.fail(scope, "env_resolution", fmt.Sprintf("Could not resolve the %s env file", scope), err.Error(), envAction(err, report.ProjectDir, scope))
		return platformconfig.OperationEnv{}, false
	}

	backupRoot := platformconfig.ResolveProjectPath(report.ProjectDir, env.BackupRoot())
	report.Scopes = append(report.Scopes, ScopeArtifact{
		Scope:      scope,
		EnvFile:    env.FilePath,
		BackupRoot: backupRoot,
	})
	report.ok(scope, "env_resolution", fmt.Sprintf("Loaded %s env file", scope), fmt.Sprintf("Using %s", env.FilePath))

	checkEnvContract(report, scope, env)

	minFreeMB, hasMinFree := parseInteger(env.Value("MIN_FREE_DISK_MB"))
	checkRuntimePath(report, scope, "db_storage_dir", "DB_STORAGE_DIR", platformconfig.ResolveProjectPath(report.ProjectDir, env.DBStorageDir()), minFreeMB, hasMinFree, pathMode)
	checkRuntimePath(report, scope, "espo_storage_dir", "ESPO_STORAGE_DIR", platformconfig.ResolveProjectPath(report.ProjectDir, env.ESPOStorageDir()), minFreeMB, hasMinFree, pathMode)
	checkRuntimePath(report, scope, "backup_root", "BACKUP_ROOT", backupRoot, minFreeMB, hasMinFree, pathMode)
	checkMaintenanceLock(report, scope, backupRoot)

	if docker.composeReady && docker.daemonReady {
		cfg := platformdocker.ComposeConfig{
			ProjectDir:  report.ProjectDir,
			ComposeFile: report.ComposeFile,
			EnvFile:     env.FilePath,
		}
		checkComposeConfig(report, scope, cfg)
		checkRunningServices(report, scope, cfg)
	}

	return env, true
}

func checkComposeFile(report *Report) {
	if _, err := platformfs.EnsureNonEmptyFile("compose file", report.ComposeFile); err != nil {
		report.fail("", "compose_file", "Compose file is not ready", err.Error(), "Set --compose-file to a readable compose.yaml path before running doctor.")
		return
	}

	report.ok("", "compose_file", "Compose file is ready", report.ComposeFile)
}

func checkEnvContract(report *Report, scope string, env platformconfig.OperationEnv) {
	problems := []string{}

	for _, key := range []string{
		"ESPOCRM_IMAGE",
		"MARIADB_TAG",
		"BACKUP_NAME_PREFIX",
		"BACKUP_RETENTION_DAYS",
		"BACKUP_MAX_DB_AGE_HOURS",
		"BACKUP_MAX_FILES_AGE_HOURS",
		"REPORT_RETENTION_DAYS",
		"SUPPORT_RETENTION_DAYS",
		"MIN_FREE_DISK_MB",
		"DOCKER_LOG_MAX_SIZE",
		"DOCKER_LOG_MAX_FILE",
		"DB_MEM_LIMIT",
		"DB_CPUS",
		"DB_PIDS_LIMIT",
		"ESPO_MEM_LIMIT",
		"ESPO_CPUS",
		"ESPO_PIDS_LIMIT",
		"DAEMON_MEM_LIMIT",
		"DAEMON_CPUS",
		"DAEMON_PIDS_LIMIT",
		"WS_MEM_LIMIT",
		"WS_CPUS",
		"WS_PIDS_LIMIT",
		"APP_PORT",
		"WS_PORT",
		"SITE_URL",
		"WS_PUBLIC_URL",
		"DB_ROOT_PASSWORD",
		"DB_NAME",
		"DB_USER",
		"DB_PASSWORD",
		"ADMIN_USERNAME",
		"ADMIN_PASSWORD",
		"ESPO_DEFAULT_LANGUAGE",
		"ESPO_TIME_ZONE",
		"ESPO_LOGGER_LEVEL",
	} {
		if strings.TrimSpace(env.Value(key)) == "" {
			problems = append(problems, fmt.Sprintf("%s is required", key))
		}
	}

	for _, key := range []string{"DB_ROOT_PASSWORD", "DB_PASSWORD", "ADMIN_PASSWORD"} {
		value := env.Value(key)
		if containsPlaceholder(value) {
			problems = append(problems, fmt.Sprintf("%s still contains a placeholder value", key))
		}
	}

	if err := validateURLSetting("SITE_URL", env.Value("SITE_URL"), map[string]bool{"http": true, "https": true}); err != nil {
		problems = append(problems, err.Error())
	}
	if err := validateURLSetting("WS_PUBLIC_URL", env.Value("WS_PUBLIC_URL"), map[string]bool{"ws": true, "wss": true}); err != nil {
		problems = append(problems, err.Error())
	}

	for _, key := range []string{
		"BACKUP_RETENTION_DAYS",
		"BACKUP_MAX_DB_AGE_HOURS",
		"BACKUP_MAX_FILES_AGE_HOURS",
		"REPORT_RETENTION_DAYS",
		"SUPPORT_RETENTION_DAYS",
		"MIN_FREE_DISK_MB",
		"DOCKER_LOG_MAX_FILE",
		"DB_PIDS_LIMIT",
		"ESPO_PIDS_LIMIT",
		"DAEMON_PIDS_LIMIT",
		"WS_PIDS_LIMIT",
	} {
		if err := validateIntegerSetting(key, env.Value(key)); err != nil {
			problems = append(problems, err.Error())
		}
	}

	for _, key := range []string{"DB_CPUS", "ESPO_CPUS", "DAEMON_CPUS", "WS_CPUS"} {
		if err := validateDecimalSetting(key, env.Value(key)); err != nil {
			problems = append(problems, err.Error())
		}
	}

	for _, key := range []string{"DB_MEM_LIMIT", "ESPO_MEM_LIMIT", "DAEMON_MEM_LIMIT", "WS_MEM_LIMIT"} {
		if err := validateMemorySetting(key, env.Value(key)); err != nil {
			problems = append(problems, err.Error())
		}
	}

	if err := validateMemorySetting("DOCKER_LOG_MAX_SIZE", env.Value("DOCKER_LOG_MAX_SIZE")); err != nil {
		problems = append(problems, strings.Replace(err.Error(), "memory limit", "log size", 1))
	}

	appPort, appOK := parsePortSetting("APP_PORT", env.Value("APP_PORT"), &problems)
	wsPort, wsOK := parsePortSetting("WS_PORT", env.Value("WS_PORT"), &problems)
	if appOK && wsOK && appPort == wsPort {
		problems = append(problems, "APP_PORT and WS_PORT must not match")
	}

	if len(problems) != 0 {
		report.fail(scope, "env_contract", "The env file contains invalid runtime settings", strings.Join(problems, "; "), fmt.Sprintf("Fix the reported settings in %s and rerun doctor.", env.FilePath))
		return
	}

	report.ok(scope, "env_contract", "The env file satisfies the runtime contract", env.FilePath)
}

func checkCrossScopeIsolation(report *Report, projectDir string, prodEnv, devEnv platformconfig.OperationEnv) {
	problems := []string{}

	if prodEnv.ComposeProject() == devEnv.ComposeProject() {
		problems = append(problems, fmt.Sprintf("COMPOSE_PROJECT_NAME matches in dev and prod: %s", prodEnv.ComposeProject()))
	}
	if prodEnv.Value("APP_PORT") == devEnv.Value("APP_PORT") {
		problems = append(problems, fmt.Sprintf("APP_PORT matches in dev and prod: %s", prodEnv.Value("APP_PORT")))
	}
	if prodEnv.Value("WS_PORT") == devEnv.Value("WS_PORT") {
		problems = append(problems, fmt.Sprintf("WS_PORT matches in dev and prod: %s", prodEnv.Value("WS_PORT")))
	}
	if sameResolvedPath(projectDir, prodEnv.DBStorageDir(), devEnv.DBStorageDir()) {
		problems = append(problems, fmt.Sprintf("DB_STORAGE_DIR resolves to the same path in dev and prod: %s", platformconfig.ResolveProjectPath(projectDir, prodEnv.DBStorageDir())))
	}
	if sameResolvedPath(projectDir, prodEnv.ESPOStorageDir(), devEnv.ESPOStorageDir()) {
		problems = append(problems, fmt.Sprintf("ESPO_STORAGE_DIR resolves to the same path in dev and prod: %s", platformconfig.ResolveProjectPath(projectDir, prodEnv.ESPOStorageDir())))
	}
	if sameResolvedPath(projectDir, prodEnv.BackupRoot(), devEnv.BackupRoot()) {
		problems = append(problems, fmt.Sprintf("BACKUP_ROOT resolves to the same path in dev and prod: %s", platformconfig.ResolveProjectPath(projectDir, prodEnv.BackupRoot())))
	}

	if len(problems) != 0 {
		report.fail("cross", "cross_scope_isolation", "Dev and prod are not isolated from each other", strings.Join(problems, "; "), "Separate the conflicting ports, project names, and storage paths before running operations against either contour.")
		return
	}

	report.ok("cross", "cross_scope_isolation", "Dev and prod keep isolated ports and storage", "")
}

func checkCrossScopeCompatibility(report *Report, prodEnv, devEnv platformconfig.OperationEnv) {
	problems := []string{}

	for _, mismatch := range platformconfig.MigrationCompatibilityMismatches(prodEnv, devEnv) {
		problems = append(problems, fmt.Sprintf("%s differs: prod=%s dev=%s", mismatch.Name, mismatch.LeftValue, mismatch.RightValue))
	}

	if len(problems) != 0 {
		report.fail("cross", "cross_scope_compatibility", "Dev and prod do not satisfy the migration compatibility contract", strings.Join(problems, "; "), "Align the shared runtime compatibility settings before relying on cross-contour migration or restore flows.")
		return
	}

	report.ok("cross", "cross_scope_compatibility", "Dev and prod satisfy the migration compatibility contract", "")
}

func validateURLSetting(name, raw string, allowedSchemes map[string]bool) error {
	value := strings.TrimSpace(raw)
	if value == "" {
		return nil
	}
	if containsPlaceholder(value) {
		return fmt.Errorf("%s still contains a placeholder value", name)
	}

	parsed, err := url.ParseRequestURI(value)
	if err != nil {
		return fmt.Errorf("%s must be a valid URL: %v", name, err)
	}
	if !allowedSchemes[strings.ToLower(parsed.Scheme)] {
		return fmt.Errorf("%s must use one of the supported schemes: %s", name, strings.Join(sortedSchemeNames(allowedSchemes), ", "))
	}

	return nil
}

func validateIntegerSetting(name, raw string) error {
	value := strings.TrimSpace(raw)
	if value == "" {
		return nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fmt.Errorf("%s must be an integer", name)
	}
	if parsed < 0 {
		return fmt.Errorf("%s must be non-negative", name)
	}

	return nil
}

func validateDecimalSetting(name, raw string) error {
	value := strings.TrimSpace(raw)
	if value == "" {
		return nil
	}
	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return fmt.Errorf("%s must be a number", name)
	}
	if parsed <= 0 {
		return fmt.Errorf("%s must be greater than zero", name)
	}

	return nil
}

func validateMemorySetting(name, raw string) error {
	value := strings.TrimSpace(raw)
	if value == "" {
		return nil
	}
	pattern := regexp.MustCompile(`^[0-9]+([.][0-9]+)?[bkmgBKMG]$`)
	if !pattern.MatchString(value) {
		return fmt.Errorf("%s must be a memory limit like 512m or 1g", name)
	}

	return nil
}

func parsePortSetting(name, raw string, problems *[]string) (int, bool) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return 0, false
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		*problems = append(*problems, fmt.Sprintf("%s must be an integer", name))
		return 0, false
	}
	if parsed < 1 || parsed > 65535 {
		*problems = append(*problems, fmt.Sprintf("%s must be between 1 and 65535", name))
		return 0, false
	}

	return parsed, true
}

func parseInteger(raw string) (int, bool) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return 0, false
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return 0, false
	}

	return parsed, true
}

func containsPlaceholder(value string) bool {
	value = strings.ToLower(strings.TrimSpace(value))
	return strings.Contains(value, "change_me") || strings.Contains(value, "your_server_ip")
}

func sameResolvedPath(projectDir, left, right string) bool {
	return platformconfig.ResolveProjectPath(projectDir, left) == platformconfig.ResolveProjectPath(projectDir, right)
}

func envAction(err error, projectDir, scope string) string {
	switch err.(type) {
	case platformconfig.MissingEnvFileError:
		return fmt.Sprintf("Create %s/.env.%s from env/.env.%s.example or pass --env-file to point doctor at the correct file.", projectDir, scope, scope)
	case platformconfig.InvalidEnvFileError, platformconfig.EnvParseError, platformconfig.MissingEnvValueError:
		return "Fix the env file contents and rerun doctor."
	default:
		return "Check the env file path and permissions, then rerun doctor."
	}
}

func sortedSchemeNames(values map[string]bool) []string {
	names := make([]string, 0, len(values))
	for name := range values {
		names = append(names, name)
	}
	if len(names) < 2 {
		return names
	}
	if names[0] > names[1] {
		names[0], names[1] = names[1], names[0]
	}

	return names
}

func (r *Report) ok(scope, code, summary, details string) {
	r.Checks = append(r.Checks, Check{
		Scope:   scope,
		Code:    code,
		Status:  statusOK,
		Summary: summary,
		Details: details,
	})
}

func (r *Report) warn(scope, code, summary, details, action string) {
	r.Checks = append(r.Checks, Check{
		Scope:   scope,
		Code:    code,
		Status:  statusWarn,
		Summary: summary,
		Details: details,
		Action:  action,
	})
}

func (r *Report) fail(scope, code, summary, details, action string) {
	r.Checks = append(r.Checks, Check{
		Scope:   scope,
		Code:    code,
		Status:  statusFail,
		Summary: summary,
		Details: details,
		Action:  action,
	})
}
