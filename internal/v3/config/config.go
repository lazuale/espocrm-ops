package config

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type BackupRequest struct {
	Scope      string
	ProjectDir string
}

type BackupConfig struct {
	Scope       string
	ProjectDir  string
	ComposeFile string
	EnvFile     string
	BackupRoot  string
	StorageDir  string
	DBService   string
	DBUser      string
	DBPassword  string
	DBName      string
}

func LoadBackup(req BackupRequest) (BackupConfig, error) {
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

	composeFile := filepath.Join(projectDir, "compose.yaml")
	if err := requireFile(composeFile, "compose file"); err != nil {
		return BackupConfig{}, err
	}

	envFile := filepath.Join(projectDir, ".env."+scope)

	values, err := loadEnvAssignments(envFile)
	if err != nil {
		return BackupConfig{}, err
	}
	if declared := strings.TrimSpace(values["ESPO_CONTOUR"]); declared != "" && declared != scope {
		return BackupConfig{}, fmt.Errorf("env file contour %q does not match --scope %q", declared, scope)
	}

	required := []string{
		"BACKUP_ROOT",
		"ESPO_STORAGE_DIR",
		"DB_USER",
		"DB_NAME",
	}
	for _, key := range required {
		if strings.TrimSpace(values[key]) == "" {
			return BackupConfig{}, fmt.Errorf("%s is required in %s", key, envFile)
		}
	}

	password, err := resolveDBPassword(values, projectDir, envFile)
	if err != nil {
		return BackupConfig{}, err
	}

	return BackupConfig{
		Scope:       scope,
		ProjectDir:  projectDir,
		ComposeFile: composeFile,
		EnvFile:     envFile,
		BackupRoot:  resolveProjectPath(projectDir, values["BACKUP_ROOT"]),
		StorageDir:  resolveProjectPath(projectDir, values["ESPO_STORAGE_DIR"]),
		DBService:   resolveDBService(values["DB_SERVICE"]),
		DBUser:      strings.TrimSpace(values["DB_USER"]),
		DBPassword:  password,
		DBName:      strings.TrimSpace(values["DB_NAME"]),
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

func resolveDBService(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "db"
	}
	return value
}

func resolveProjectPath(projectDir, value string) string {
	value = strings.TrimSpace(value)
	if filepath.IsAbs(value) {
		return filepath.Clean(value)
	}
	return filepath.Join(projectDir, filepath.Clean(value))
}

func resolveDBPassword(values map[string]string, projectDir, envFile string) (string, error) {
	inline := strings.TrimSpace(values["DB_PASSWORD"])
	fileRef := strings.TrimSpace(values["DB_PASSWORD_FILE"])

	switch {
	case inline != "" && fileRef != "":
		return "", fmt.Errorf("only one of DB_PASSWORD or DB_PASSWORD_FILE may be set in %s", envFile)
	case inline != "":
		return inline, nil
	case fileRef == "":
		return "", fmt.Errorf("DB_PASSWORD or DB_PASSWORD_FILE is required in %s", envFile)
	}

	passwordPath := resolveProjectPath(projectDir, fileRef)
	raw, err := os.ReadFile(passwordPath)
	if err != nil {
		return "", fmt.Errorf("read DB_PASSWORD_FILE %s: %w", passwordPath, err)
	}
	password := strings.TrimSpace(string(raw))
	if password == "" {
		return "", fmt.Errorf("DB_PASSWORD_FILE %s is empty", passwordPath)
	}
	return password, nil
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
	trimmed := strings.TrimLeft(line, " \t")
	if trimmed == "" {
		return "", "", false
	}

	sep := strings.IndexByte(trimmed, '=')
	if sep <= 0 {
		return "", "", false
	}

	key := trimmed[:sep]
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

	return key, trimmed[sep+1:], true
}

func isEnvKeyStart(ch rune) bool {
	return ch == '_' || ('A' <= ch && ch <= 'Z') || ('a' <= ch && ch <= 'z')
}

func isEnvKeyPart(ch rune) bool {
	return isEnvKeyStart(ch) || ('0' <= ch && ch <= '9')
}

func parseEnvValue(rawValue, path string, lineNo int) (string, error) {
	switch {
	case strings.HasPrefix(rawValue, "\""):
		return decodeDoubleQuotedValue(rawValue, path, lineNo)
	case strings.HasPrefix(rawValue, "'"):
		return decodeSingleQuotedValue(rawValue, path, lineNo)
	default:
		return decodeUnquotedValue(rawValue, path, lineNo)
	}
}

func decodeDoubleQuotedValue(rawValue, path string, lineNo int) (string, error) {
	if len(rawValue) < 2 || !strings.HasSuffix(rawValue, "\"") {
		return "", fmt.Errorf("%s:%d: unterminated double-quoted value", path, lineNo)
	}

	inner := rawValue[1 : len(rawValue)-1]
	var decoded strings.Builder
	escapeNext := false
	for _, ch := range inner {
		if escapeNext {
			switch ch {
			case '\\', '"':
				decoded.WriteRune(ch)
			default:
				return "", fmt.Errorf("%s:%d: unsupported escape sequence \\%c", path, lineNo, ch)
			}
			escapeNext = false
			continue
		}

		switch ch {
		case '\\':
			escapeNext = true
		case '"':
			return "", fmt.Errorf("%s:%d: inner double quotes must be escaped", path, lineNo)
		default:
			decoded.WriteRune(ch)
		}
	}
	if escapeNext {
		return "", fmt.Errorf("%s:%d: unfinished escape sequence", path, lineNo)
	}

	value := decoded.String()
	if err := rejectShellSyntax(value, path, lineNo); err != nil {
		return "", err
	}
	return value, nil
}

func decodeSingleQuotedValue(rawValue, path string, lineNo int) (string, error) {
	if len(rawValue) < 2 || !strings.HasSuffix(rawValue, "'") {
		return "", fmt.Errorf("%s:%d: unterminated single-quoted value", path, lineNo)
	}

	value := rawValue[1 : len(rawValue)-1]
	if strings.Contains(value, "'") {
		return "", fmt.Errorf("%s:%d: raw single quote is not allowed inside single-quoted value", path, lineNo)
	}
	if err := rejectShellSyntax(value, path, lineNo); err != nil {
		return "", err
	}
	return value, nil
}

func decodeUnquotedValue(rawValue, path string, lineNo int) (string, error) {
	if strings.ContainsAny(rawValue, " \t") {
		return "", fmt.Errorf("%s:%d: unquoted values with spaces are not allowed", path, lineNo)
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
