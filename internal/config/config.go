package config

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type BackupRequest struct {
	Scope      string
	ProjectDir string
}

type BackupConfig struct {
	Scope                      string
	ProjectDir                 string
	ComposeFile                string
	EnvFile                    string
	EspoCRMImage               string
	MariaDBImage               string
	BackupRoot                 string
	BackupNamePrefix           string
	BackupRetentionDays        int
	MinFreeDiskMB              int
	StorageDir                 string
	AppServices                []string
	DBService                  string
	DBUser                     string
	DBPassword                 string
	DBRootPassword             string
	DBName                     string
	RuntimeUID                 int
	RuntimeGID                 int
	RuntimeOwnershipConfigured bool
	Warnings                   []string
}

func LoadBackup(req BackupRequest) (BackupConfig, error) {
	return load(req, false)
}

func LoadRestore(req BackupRequest) (BackupConfig, error) {
	return load(req, true)
}

const maxBackupNamePrefixLen = 80
const sha256DigestHexLen = 64

func load(req BackupRequest, restoreMode bool) (BackupConfig, error) {
	scope := strings.TrimSpace(req.Scope)
	if scope != "dev" && scope != "prod" {
		return BackupConfig{}, fmt.Errorf("--scope must be dev or prod")
	}

	projectDir, err := filepath.Abs(filepath.Clean(strings.TrimSpace(req.ProjectDir)))
	if err != nil {
		return BackupConfig{}, fmt.Errorf("resolve project dir: %w", err)
	}
	if err := requireDirectory(projectDir, "project dir"); err != nil {
		return BackupConfig{}, err
	}

	warnings := []string{}
	envFile := filepath.Join(projectDir, ".env."+scope)
	if scope == "prod" {
		envWarnings, err := inspectProdEnvFile(envFile)
		if err != nil {
			return BackupConfig{}, err
		}
		warnings = append(warnings, envWarnings...)
	}

	values, err := loadEnvAssignments(envFile)
	if err != nil {
		return BackupConfig{}, err
	}
	composeFile := filepath.Join(projectDir, "compose.yaml")
	if configured := strings.TrimSpace(values["COMPOSE_FILE"]); configured != "" {
		composeFile = resolveProjectPath(projectDir, configured)
	}
	if err := requireFile(composeFile, "compose file"); err != nil {
		return BackupConfig{}, err
	}
	if declared := strings.TrimSpace(values["ESPO_CONTOUR"]); declared != "" && declared != scope {
		return BackupConfig{}, fmt.Errorf("env file contour %q does not match --scope %q", declared, scope)
	}

	required := []string{
		"BACKUP_ROOT",
		"ESPO_STORAGE_DIR",
		"APP_SERVICES",
		"DB_SERVICE",
		"DB_USER",
		"DB_NAME",
	}
	for _, key := range required {
		if strings.TrimSpace(values[key]) == "" {
			return BackupConfig{}, fmt.Errorf("%s is required in %s", key, envFile)
		}
	}

	password, err := resolveRequiredInlineSecret(values, "DB_PASSWORD", envFile, true)
	if err != nil {
		return BackupConfig{}, err
	}
	rootPassword, err := resolveRequiredInlineSecret(values, "DB_ROOT_PASSWORD", envFile, restoreMode)
	if err != nil {
		return BackupConfig{}, err
	}
	runtimeUID, runtimeGID, runtimeOwnershipConfigured, err := resolveRuntimeOwnership(values, envFile, restoreMode)
	if err != nil {
		return BackupConfig{}, err
	}
	appServices, err := resolveAppServices(values["APP_SERVICES"])
	if err != nil {
		return BackupConfig{}, fmt.Errorf("APP_SERVICES in %s: %w", envFile, err)
	}
	backupNamePrefix, err := resolveBackupNamePrefix(values, envFile)
	if err != nil {
		return BackupConfig{}, err
	}
	backupRetentionDays, err := resolveRequiredEnvInt(values, "BACKUP_RETENTION_DAYS", envFile, 0)
	if err != nil {
		return BackupConfig{}, err
	}
	minFreeDiskMB, err := resolveRequiredEnvInt(values, "MIN_FREE_DISK_MB", envFile, 1)
	if err != nil {
		return BackupConfig{}, err
	}
	espoCRMImage := strings.TrimSpace(values["ESPOCRM_IMAGE"])
	mariaDBImage := strings.TrimSpace(values["MARIADB_IMAGE"])
	if scope == "prod" {
		warnings = append(warnings, digestPinnedImageWarnings(envFile, "ESPOCRM_IMAGE", espoCRMImage)...)
		warnings = append(warnings, digestPinnedImageWarnings(envFile, "MARIADB_IMAGE", mariaDBImage)...)
	}

	return BackupConfig{
		Scope:                      scope,
		ProjectDir:                 projectDir,
		ComposeFile:                composeFile,
		EnvFile:                    envFile,
		EspoCRMImage:               espoCRMImage,
		MariaDBImage:               mariaDBImage,
		BackupRoot:                 resolveProjectPath(projectDir, values["BACKUP_ROOT"]),
		BackupNamePrefix:           backupNamePrefix,
		BackupRetentionDays:        backupRetentionDays,
		MinFreeDiskMB:              minFreeDiskMB,
		StorageDir:                 resolveProjectPath(projectDir, values["ESPO_STORAGE_DIR"]),
		AppServices:                appServices,
		DBService:                  strings.TrimSpace(values["DB_SERVICE"]),
		DBUser:                     strings.TrimSpace(values["DB_USER"]),
		DBPassword:                 password,
		DBRootPassword:             rootPassword,
		DBName:                     strings.TrimSpace(values["DB_NAME"]),
		RuntimeUID:                 runtimeUID,
		RuntimeGID:                 runtimeGID,
		RuntimeOwnershipConfigured: runtimeOwnershipConfigured,
		Warnings:                   warnings,
	}, nil
}

func requireDirectory(path, label string) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("stat %s %s: %w", label, path, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("%s %s must be a directory", label, path)
	}
	return nil
}

func requireFile(path, label string) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("stat %s %s: %w", label, path, err)
	}
	if info.IsDir() {
		return fmt.Errorf("%s %s must be a file", label, path)
	}
	return nil
}

func inspectProdEnvFile(path string) ([]string, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, fmt.Errorf("stat prod env file %s: %w", path, err)
	}
	warnings := []string{}
	if info.Mode()&os.ModeSymlink != 0 {
		warnings = append(warnings, fmt.Sprintf("prod env file %s is a symlink; regular file mode 0600 is recommended", path))
		return warnings, nil
	}
	if !info.Mode().IsRegular() {
		warnings = append(warnings, fmt.Sprintf("prod env file %s is not a regular file; regular file mode 0600 is recommended", path))
		return warnings, nil
	}

	mode := info.Mode()
	if mode != 0o600 {
		warnings = append(warnings, fmt.Sprintf("prod env file %s has permissions %04o; mode 0600 is recommended", path, mode.Perm()))
	}
	return warnings, nil
}

func digestPinnedImageWarnings(envFile, key, ref string) []string {
	if ref == "" || isDigestPinnedImageRef(ref) {
		return nil
	}
	return []string{fmt.Sprintf("%s in %s is not digest-pinned; mutable image refs are accepted here but reduce restore reproducibility", key, envFile)}
}

func isDigestPinnedImageRef(ref string) bool {
	prefix, digest, ok := strings.Cut(ref, "@sha256:")
	if !ok || prefix == "" {
		return false
	}
	if len(digest) != sha256DigestHexLen {
		return false
	}
	for _, ch := range digest {
		if ('0' <= ch && ch <= '9') || ('a' <= ch && ch <= 'f') {
			continue
		}
		return false
	}
	return true
}

func resolveProjectPath(projectDir, value string) string {
	value = strings.TrimSpace(value)
	if filepath.IsAbs(value) {
		return filepath.Clean(value)
	}
	return filepath.Join(projectDir, filepath.Clean(value))
}

func resolveRequiredInlineSecret(values map[string]string, key, envFile string, required bool) (string, error) {
	value := strings.TrimSpace(values[key])
	if value == "" {
		if !required {
			return "", nil
		}
		return "", fmt.Errorf("%s is required in %s", key, envFile)
	}
	return value, nil
}

func resolveAppServices(raw string) ([]string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, fmt.Errorf("value is required")
	}

	parts := strings.Split(raw, ",")
	services := make([]string, 0, len(parts))
	for _, part := range parts {
		service := strings.TrimSpace(part)
		if service == "" {
			return nil, fmt.Errorf("service names must be non-empty")
		}
		services = append(services, service)
	}
	return services, nil
}

func resolveBackupNamePrefix(values map[string]string, envFile string) (string, error) {
	raw, ok := values["BACKUP_NAME_PREFIX"]
	if !ok || raw == "" {
		return "", fmt.Errorf("BACKUP_NAME_PREFIX is required in %s", envFile)
	}
	if strings.TrimSpace(raw) == "" {
		return "", fmt.Errorf("BACKUP_NAME_PREFIX in %s must not be empty or whitespace", envFile)
	}
	if raw == "." || raw == ".." {
		return "", fmt.Errorf("BACKUP_NAME_PREFIX in %s must not be single-dot or double-dot", envFile)
	}
	if len(raw) > maxBackupNamePrefixLen {
		return "", fmt.Errorf("BACKUP_NAME_PREFIX in %s must be at most %d characters", envFile, maxBackupNamePrefixLen)
	}
	for _, ch := range raw {
		if ('a' <= ch && ch <= 'z') || ('A' <= ch && ch <= 'Z') || ('0' <= ch && ch <= '9') || ch == '_' || ch == '-' || ch == '.' {
			continue
		}
		return "", fmt.Errorf("BACKUP_NAME_PREFIX in %s must match [A-Za-z0-9._-]+", envFile)
	}
	return raw, nil
}

func resolveRequiredEnvInt(values map[string]string, key, envFile string, minValue int) (int, error) {
	raw, ok := values[key]
	if !ok || raw == "" {
		return 0, fmt.Errorf("%s is required in %s", key, envFile)
	}
	return parseEnvIntAtLeast(key, raw, envFile, minValue)
}

func resolveRuntimeOwnership(values map[string]string, envFile string, required bool) (int, int, bool, error) {
	rawUID, uidPresent := values["ESPO_RUNTIME_UID"]
	rawGID, gidPresent := values["ESPO_RUNTIME_GID"]

	if !uidPresent && !gidPresent {
		if required {
			return 0, 0, false, fmt.Errorf("ESPO_RUNTIME_UID and ESPO_RUNTIME_GID are required in %s", envFile)
		}
		return 0, 0, false, nil
	}

	rawUID = strings.TrimSpace(rawUID)
	rawGID = strings.TrimSpace(rawGID)
	switch {
	case rawUID == "" && rawGID == "":
		return 0, 0, false, fmt.Errorf("ESPO_RUNTIME_UID and ESPO_RUNTIME_GID are required in %s", envFile)
	case rawUID == "" || rawGID == "":
		return 0, 0, false, fmt.Errorf("ESPO_RUNTIME_UID and ESPO_RUNTIME_GID must both be set in %s", envFile)
	}

	uid, err := parseEnvIntAtLeast("ESPO_RUNTIME_UID", rawUID, envFile, 0)
	if err != nil {
		return 0, 0, false, err
	}
	gid, err := parseEnvIntAtLeast("ESPO_RUNTIME_GID", rawGID, envFile, 0)
	if err != nil {
		return 0, 0, false, err
	}

	return uid, gid, true, nil
}

func parseEnvIntAtLeast(key, raw, envFile string, minValue int) (int, error) {
	value, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || value < minValue {
		return 0, fmt.Errorf("%s in %s must be an integer %s", key, envFile, envIntMinDescription(minValue))
	}
	return value, nil
}

func envIntMinDescription(minValue int) string {
	if minValue == 1 {
		return "> 0"
	}
	return fmt.Sprintf(">= %d", minValue)
}

func loadEnvAssignments(path string) (values map[string]string, err error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open env file %s: %w", path, err)
	}
	defer func() {
		if closeErr := file.Close(); closeErr != nil {
			wrapped := fmt.Errorf("close env file %s: %w", path, closeErr)
			if err == nil {
				err = wrapped
			} else {
				err = errors.Join(err, wrapped)
			}
		}
	}()

	values = map[string]string{}
	scanner := bufio.NewScanner(file)
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		line := strings.TrimSuffix(scanner.Text(), "\r")
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}

		key, rawValue, ok := parseEnvAssignment(line)
		if !ok {
			return nil, fmt.Errorf("%s:%d: expected KEY=VALUE", path, lineNo)
		}
		if _, exists := values[key]; exists {
			return nil, fmt.Errorf("%s:%d: duplicate assignment for %s", path, lineNo, key)
		}

		value, err := parseEnvValue(rawValue, path, lineNo)
		if err != nil {
			return nil, err
		}
		values[key] = value
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read env file %s: %w", path, err)
	}

	return values, nil
}

func parseEnvAssignment(line string) (string, string, bool) {
	if line == "" || strings.TrimSpace(line) != line || strings.ContainsAny(line, " \t") {
		return "", "", false
	}

	sep := strings.IndexByte(line, '=')
	if sep <= 0 {
		return "", "", false
	}

	key := line[:sep]
	for i, ch := range key {
		if i == 0 {
			if !isEnvKeyStart(ch) {
				return "", "", false
			}
			continue
		}
		if !isEnvKeyPart(ch) {
			return "", "", false
		}
	}

	return key, line[sep+1:], true
}

func isEnvKeyStart(ch rune) bool {
	return ch == '_' || ('A' <= ch && ch <= 'Z') || ('a' <= ch && ch <= 'z')
}

func isEnvKeyPart(ch rune) bool {
	return isEnvKeyStart(ch) || ('0' <= ch && ch <= '9')
}

func parseEnvValue(rawValue, path string, lineNo int) (string, error) {
	if strings.ContainsAny(rawValue, " \t") {
		return "", fmt.Errorf("%s:%d: values with spaces are not allowed", path, lineNo)
	}
	if strings.ContainsAny(rawValue, `"'`) {
		return "", fmt.Errorf("%s:%d: quoted values are not allowed", path, lineNo)
	}
	if err := rejectShellSyntax(rawValue, path, lineNo); err != nil {
		return "", err
	}
	return rawValue, nil
}

func rejectShellSyntax(value, path string, lineNo int) error {
	if strings.Contains(value, "$(") || strings.Contains(value, "${") || strings.Contains(value, "`") {
		return fmt.Errorf("%s:%d: shell expansion syntax is not allowed", path, lineNo)
	}
	return nil
}
