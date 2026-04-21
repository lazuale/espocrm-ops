package doctor

import (
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	domainenv "github.com/lazuale/espocrm-ops/internal/domain/env"
)

func checkEnvContract(report *Report, scope string, env domainenv.OperationEnv) {
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
