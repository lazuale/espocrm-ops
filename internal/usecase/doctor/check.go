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
	platformlocks "github.com/lazuale/espocrm-ops/internal/platform/locks"
)

const (
	statusOK   = "ok"
	statusWarn = "warn"
	statusFail = "fail"
)

type Request struct {
	Scope                  string
	ProjectDir             string
	ComposeFile            string
	EnvFileOverride        string
	EnvContourHint         string
	PathCheckMode          PathCheckMode
	InheritedOperationLock bool
	InheritedMaintenance   bool
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
	checkSharedOperationLock(&report, req.InheritedOperationLock)
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
	env, err := platformconfig.LoadOperationEnv(report.ProjectDir, scope, req.EnvFileOverride, req.EnvContourHint)
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
	checkMaintenanceLock(report, scope, backupRoot, req.InheritedMaintenance)

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

func checkSharedOperationLock(report *Report, inherited bool) {
	if inherited {
		report.ok("", "shared_operation_lock", "The shared operation lock is already held by the parent operation", "Inherited from the active parent operation.")
		return
	}

	readiness, err := platformlocks.CheckSharedOperationReadiness(report.ProjectDir)
	if err != nil {
		report.fail("", "shared_operation_lock", "Could not inspect the shared operation lock", err.Error(), "Check the filesystem permissions for the temporary lock directory and rerun doctor.")
		return
	}

	switch readiness.State {
	case platformlocks.LockReady:
		report.ok("", "shared_operation_lock", "The shared operation lock is available", readiness.MetadataPath)
	case platformlocks.LockStale:
		report.warn("", "shared_operation_lock", "The shared operation lock metadata is stale", readiness.MetadataPath, "Remove the stale lock metadata after verifying no toolkit operation is still running.")
	case platformlocks.LockActive:
		report.fail("", "shared_operation_lock", "Another toolkit operation is already running", lockOwnerDetails(readiness.MetadataPath, readiness.PID), "Wait for the active toolkit operation to finish before running a stateful command.")
	case platformlocks.LockLegacy:
		report.fail("", "shared_operation_lock", "A legacy shared lock blocks safe readiness checks", lockOwnerDetails(readiness.MetadataPath, readiness.PID), "Remove the legacy lock only after verifying that no toolkit process still owns it.")
	default:
		report.fail("", "shared_operation_lock", "The shared operation lock reported an unknown state", readiness.State, "Inspect the lock files under the system temp directory and rerun doctor.")
	}
}

func checkMaintenanceLock(report *Report, scope, backupRoot string, inherited bool) {
	if inherited {
		report.ok(scope, "maintenance_lock", "The maintenance lock is already held by the parent operation", backupRoot)
		return
	}

	readiness, err := platformlocks.CheckMaintenanceReadiness(backupRoot)
	if err != nil {
		report.fail(scope, "maintenance_lock", "Could not inspect the maintenance lock", err.Error(), "Check the backup lock directory permissions and rerun doctor.")
		return
	}

	switch readiness.State {
	case platformlocks.LockReady:
		report.ok(scope, "maintenance_lock", "The maintenance lock is available", readiness.MetadataPath)
	case platformlocks.LockStale:
		report.warn(scope, "maintenance_lock", "Found stale maintenance lock metadata", strings.Join(readiness.StalePaths, "; "), "Remove the stale maintenance lock files after verifying that no maintenance operation is still running.")
	case platformlocks.LockActive:
		report.fail(scope, "maintenance_lock", "Another maintenance operation is already running for this contour", lockOwnerDetails(readiness.MetadataPath, readiness.PID), "Wait for the running maintenance operation to finish before starting a new one.")
	case platformlocks.LockLegacy:
		report.fail(scope, "maintenance_lock", "A legacy maintenance lock blocks safe readiness checks", lockOwnerDetails(readiness.MetadataPath, readiness.PID), "Remove the legacy maintenance lock only after verifying that no toolkit process still owns it.")
	default:
		report.fail(scope, "maintenance_lock", "The maintenance lock reported an unknown state", readiness.State, "Inspect the contour lock files and rerun doctor.")
	}
}

func checkDocker(report *Report) dockerState {
	state := dockerState{}

	clientVersion, err := platformdocker.DockerClientVersion()
	if err != nil {
		report.fail("", "docker_cli", "Docker CLI is not available", err.Error(), "Install Docker and ensure the `docker` binary is on PATH.")
		return state
	}
	state.cliReady = true
	state.cliVersion = clientVersion
	report.ok("", "docker_cli", "Docker CLI is available", fmt.Sprintf("docker %s", clientVersion))

	serverVersion, err := platformdocker.DockerServerVersion()
	if err != nil {
		report.fail("", "docker_daemon", "Docker daemon is not reachable", err.Error(), "Start the Docker daemon and verify that `docker version` can reach the server.")
	} else {
		state.daemonReady = true
		state.serverVersion = serverVersion
		if versionAtLeast(serverVersion, "24.0.0") {
			report.ok("", "docker_daemon", "Docker daemon is reachable", fmt.Sprintf("server %s", serverVersion))
		} else {
			report.warn("", "docker_daemon", "Docker daemon is reachable but below the recommended version", fmt.Sprintf("server %s; recommended minimum 24.0.0", serverVersion), "Upgrade Docker Engine to reduce compatibility risk before running stateful operations.")
		}
	}

	composeVersion, err := platformdocker.ComposeVersion()
	if err != nil {
		report.fail("", "docker_compose", "Docker Compose is not available", err.Error(), "Install Docker Compose v2 and verify that `docker compose version` succeeds.")
		return state
	}
	state.composeReady = true
	state.composeVersion = composeVersion
	if versionAtLeast(composeVersion, "2.20.0") {
		report.ok("", "docker_compose", "Docker Compose is available", fmt.Sprintf("compose %s", composeVersion))
	} else {
		report.warn("", "docker_compose", "Docker Compose is available but below the recommended version", fmt.Sprintf("compose %s; recommended minimum 2.20.0", composeVersion), "Upgrade Docker Compose to reduce compatibility risk before running stateful operations.")
	}

	return state
}

func checkComposeConfig(report *Report, scope string, cfg platformdocker.ComposeConfig) {
	if err := platformdocker.ValidateComposeConfig(cfg); err != nil {
		report.fail(scope, "compose_config", "Docker Compose config validation failed", err.Error(), "Run `docker compose config -q` for the same env file, fix the reported configuration error, and rerun doctor.")
		return
	}

	report.ok(scope, "compose_config", "Docker Compose config validation passed", fmt.Sprintf("compose file %s with env %s", cfg.ComposeFile, cfg.EnvFile))
}

func checkRunningServices(report *Report, scope string, cfg platformdocker.ComposeConfig) {
	services, err := platformdocker.ComposeRunningServices(cfg)
	if err != nil {
		report.fail(scope, "running_services", "Could not inspect running services", err.Error(), "Check Docker access for this contour and rerun doctor.")
		return
	}

	if len(services) == 0 {
		report.ok(scope, "running_services", "No services are currently running for this contour", "The runtime health probe is not required while the contour is stopped.")
		return
	}

	unhealthy := []string{}
	for _, service := range services {
		state, err := platformdocker.ComposeServiceStateFor(cfg, service)
		if err != nil {
			report.fail(scope, "running_services", "Could not inspect the running service health", err.Error(), "Check the service containers with `docker compose ps` and rerun doctor.")
			return
		}

		switch state.Status {
		case "", "running", "healthy":
		case "unhealthy":
			if strings.TrimSpace(state.HealthMessage) != "" {
				unhealthy = append(unhealthy, fmt.Sprintf("%s: %s", service, state.HealthMessage))
			} else {
				unhealthy = append(unhealthy, fmt.Sprintf("%s: reported unhealthy", service))
			}
		default:
			unhealthy = append(unhealthy, fmt.Sprintf("%s: reported %s", service, state.Status))
		}
	}

	if len(unhealthy) != 0 {
		report.fail(scope, "running_services", "A running service is unhealthy", strings.Join(unhealthy, "; "), "Repair the unhealthy service state before running a stateful operation.")
		return
	}

	report.ok(scope, "running_services", "Running services are healthy", strings.Join(services, ", "))
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

	if prodEnv.Value("ESPOCRM_IMAGE") != devEnv.Value("ESPOCRM_IMAGE") {
		problems = append(problems, fmt.Sprintf("ESPOCRM_IMAGE differs: prod=%s dev=%s", prodEnv.Value("ESPOCRM_IMAGE"), devEnv.Value("ESPOCRM_IMAGE")))
	}
	if prodEnv.Value("MARIADB_TAG") != devEnv.Value("MARIADB_TAG") {
		problems = append(problems, fmt.Sprintf("MARIADB_TAG differs: prod=%s dev=%s", prodEnv.Value("MARIADB_TAG"), devEnv.Value("MARIADB_TAG")))
	}
	if prodEnv.Value("ESPO_DEFAULT_LANGUAGE") != devEnv.Value("ESPO_DEFAULT_LANGUAGE") {
		problems = append(problems, fmt.Sprintf("ESPO_DEFAULT_LANGUAGE differs: prod=%s dev=%s", prodEnv.Value("ESPO_DEFAULT_LANGUAGE"), devEnv.Value("ESPO_DEFAULT_LANGUAGE")))
	}
	if prodEnv.Value("ESPO_TIME_ZONE") != devEnv.Value("ESPO_TIME_ZONE") {
		problems = append(problems, fmt.Sprintf("ESPO_TIME_ZONE differs: prod=%s dev=%s", prodEnv.Value("ESPO_TIME_ZONE"), devEnv.Value("ESPO_TIME_ZONE")))
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
		return fmt.Sprintf("Create %s/.env.%s from ops/env/.env.%s.example or pass --env-file to point doctor at the correct file.", projectDir, scope, scope)
	case platformconfig.InvalidEnvFileError, platformconfig.EnvParseError, platformconfig.MissingEnvValueError:
		return "Fix the env file contents and rerun doctor."
	default:
		return "Check the env file path and permissions, then rerun doctor."
	}
}

func lockOwnerDetails(path, pid string) string {
	if strings.TrimSpace(pid) == "" {
		return path
	}

	return fmt.Sprintf("%s (PID %s)", path, pid)
}

func versionAtLeast(current, minimum string) bool {
	currentParts := parseVersion(current)
	minimumParts := parseVersion(minimum)
	maxLen := max(len(minimumParts), len(currentParts))

	for i := 0; i < maxLen; i++ {
		currentPart := 0
		if i < len(currentParts) {
			currentPart = currentParts[i]
		}
		minimumPart := 0
		if i < len(minimumParts) {
			minimumPart = minimumParts[i]
		}
		if currentPart > minimumPart {
			return true
		}
		if currentPart < minimumPart {
			return false
		}
	}

	return true
}

func parseVersion(raw string) []int {
	raw = strings.TrimPrefix(strings.TrimSpace(raw), "v")
	parts := strings.Split(raw, ".")
	out := make([]int, 0, len(parts))
	for _, part := range parts {
		digits := strings.Builder{}
		for _, ch := range part {
			if ch < '0' || ch > '9' {
				break
			}
			digits.WriteRune(ch)
		}
		if digits.Len() == 0 {
			out = append(out, 0)
			continue
		}
		parsed, err := strconv.Atoi(digits.String())
		if err != nil {
			out = append(out, 0)
			continue
		}
		out = append(out, parsed)
	}

	return out
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
