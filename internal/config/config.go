package config

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type Request struct {
	Scope      string
	ProjectDir string
}

type Config struct {
	Scope          string
	ProjectDir     string
	ComposeFile    string
	EnvFile        string
	BackupRoot     string
	StorageDir     string
	AppServices    []string
	DBService      string
	DBUser         string
	DBPassword     string
	DBRootPassword string
	DBName         string
}

var requiredKeys = []string{
	"BACKUP_ROOT",
	"ESPO_STORAGE_DIR",
	"APP_SERVICES",
	"DB_SERVICE",
	"DB_USER",
	"DB_PASSWORD",
	"DB_ROOT_PASSWORD",
	"DB_NAME",
}

func Load(req Request) (Config, error) {
	scope := strings.TrimSpace(req.Scope)
	if err := validateScope(scope); err != nil {
		return Config{}, err
	}

	projectDir, err := filepath.Abs(filepath.Clean(strings.TrimSpace(req.ProjectDir)))
	if err != nil {
		return Config{}, fmt.Errorf("resolve project dir: %w", err)
	}
	if err := requireDirectory(projectDir, "project dir"); err != nil {
		return Config{}, err
	}

	envFile := filepath.Join(projectDir, ".env."+scope)
	values, err := loadEnvAssignments(envFile)
	if err != nil {
		return Config{}, err
	}
	for _, key := range requiredKeys {
		if strings.TrimSpace(values[key]) == "" {
			return Config{}, fmt.Errorf("%s is required in %s", key, envFile)
		}
	}

	appServices, err := resolveAppServices(values["APP_SERVICES"], envFile)
	if err != nil {
		return Config{}, err
	}

	composeFile := filepath.Join(projectDir, "compose.yaml")
	if err := requireFile(composeFile, "compose file"); err != nil {
		return Config{}, err
	}

	return Config{
		Scope:          scope,
		ProjectDir:     projectDir,
		ComposeFile:    composeFile,
		EnvFile:        envFile,
		BackupRoot:     resolveProjectPath(projectDir, values["BACKUP_ROOT"]),
		StorageDir:     resolveProjectPath(projectDir, values["ESPO_STORAGE_DIR"]),
		AppServices:    appServices,
		DBService:      strings.TrimSpace(values["DB_SERVICE"]),
		DBUser:         strings.TrimSpace(values["DB_USER"]),
		DBPassword:     strings.TrimSpace(values["DB_PASSWORD"]),
		DBRootPassword: strings.TrimSpace(values["DB_ROOT_PASSWORD"]),
		DBName:         strings.TrimSpace(values["DB_NAME"]),
	}, nil
}

func validateScope(scope string) error {
	if scope == "" {
		return fmt.Errorf("--scope is required")
	}
	if scope == "." || scope == ".." {
		return fmt.Errorf("--scope is unsafe")
	}
	for _, ch := range scope {
		if ('a' <= ch && ch <= 'z') || ('A' <= ch && ch <= 'Z') || ('0' <= ch && ch <= '9') || ch == '_' || ch == '-' || ch == '.' {
			continue
		}
		return fmt.Errorf("--scope must match [A-Za-z0-9._-]+")
	}
	return nil
}

func resolveProjectPath(projectDir, value string) string {
	value = strings.TrimSpace(value)
	if filepath.IsAbs(value) {
		return filepath.Clean(value)
	}
	return filepath.Join(projectDir, filepath.Clean(value))
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

func resolveAppServices(raw, envFile string) ([]string, error) {
	parts := strings.Split(strings.TrimSpace(raw), ",")
	services := make([]string, 0, len(parts))
	for _, part := range parts {
		service := strings.TrimSpace(part)
		if service == "" {
			return nil, fmt.Errorf("APP_SERVICES in %s contains empty service name", envFile)
		}
		services = append(services, service)
	}
	if len(services) == 0 {
		return nil, fmt.Errorf("APP_SERVICES is required in %s", envFile)
	}
	return services, nil
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
		if err := validateEnvValue(rawValue, path, lineNo); err != nil {
			return nil, err
		}
		values[key] = rawValue
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

func validateEnvValue(rawValue, path string, lineNo int) error {
	if strings.ContainsAny(rawValue, " \t") {
		return fmt.Errorf("%s:%d: values with spaces are not allowed", path, lineNo)
	}
	if strings.ContainsAny(rawValue, `"'`) {
		return fmt.Errorf("%s:%d: quoted values are not allowed", path, lineNo)
	}
	if strings.Contains(rawValue, "$(") || strings.Contains(rawValue, "${") || strings.Contains(rawValue, "`") {
		return fmt.Errorf("%s:%d: shell expansion syntax is not allowed", path, lineNo)
	}
	return nil
}

func isEnvKeyStart(ch rune) bool {
	return ch == '_' || ('A' <= ch && ch <= 'Z') || ('a' <= ch && ch <= 'z')
}

func isEnvKeyPart(ch rune) bool {
	return isEnvKeyStart(ch) || ('0' <= ch && ch <= '9')
}
