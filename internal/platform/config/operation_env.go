package config

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
)

var knownOperationalEnvVars = []string{
	"ESPO_CONTOUR",
	"COMPOSE_PROJECT_NAME",
	"ESPOCRM_IMAGE",
	"MARIADB_TAG",
	"DB_STORAGE_DIR",
	"ESPO_STORAGE_DIR",
	"BACKUP_ROOT",
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
}

type OperationEnv struct {
	FilePath        string
	ResolvedContour string
	Values          map[string]string
}

func (e OperationEnv) Value(key string) string {
	if e.Values == nil {
		return ""
	}

	return e.Values[key]
}

func (e OperationEnv) ComposeProject() string {
	return e.Value("COMPOSE_PROJECT_NAME")
}

func (e OperationEnv) DBStorageDir() string {
	return e.Value("DB_STORAGE_DIR")
}

func (e OperationEnv) ESPOStorageDir() string {
	return e.Value("ESPO_STORAGE_DIR")
}

func (e OperationEnv) BackupRoot() string {
	return e.Value("BACKUP_ROOT")
}

func LoadOperationEnv(projectDir, scope, overridePath, contourHint string) (OperationEnv, error) {
	resolvedPath, err := resolveOperationEnvFile(projectDir, scope, overridePath)
	if err != nil {
		return OperationEnv{}, err
	}

	if err := validateEnvFileForLoading(resolvedPath); err != nil {
		return OperationEnv{}, err
	}

	values, err := loadEnvAssignments(resolvedPath)
	if err != nil {
		return OperationEnv{}, err
	}

	for _, key := range []string{"COMPOSE_PROJECT_NAME", "DB_STORAGE_DIR", "ESPO_STORAGE_DIR", "BACKUP_ROOT"} {
		if strings.TrimSpace(values[key]) == "" {
			return OperationEnv{}, MissingEnvValueError{Path: resolvedPath, Name: key}
		}
	}

	resolvedContour, err := validateLoadedEnvContour(resolvedPath, scope, contourHint, values["ESPO_CONTOUR"])
	if err != nil {
		return OperationEnv{}, err
	}

	return OperationEnv{
		FilePath:        resolvedPath,
		ResolvedContour: resolvedContour,
		Values:          values,
	}, nil
}

func ApplyOperationEnv(base []string, env OperationEnv, extra map[string]string) []string {
	filtered := []string{}
	blocked := map[string]bool{}

	for _, key := range knownOperationalEnvVars {
		blocked[key] = true
	}
	for _, key := range []string{
		"ENV_FILE",
		"ESPO_ENV",
		"RESOLVED_ENV_CONTOUR",
		"ESPO_OPERATION_LOCK",
		"ESPO_MAINTENANCE_LOCK",
		"ESPO_SHELL_EXEC_CONTEXT",
	} {
		blocked[key] = true
	}

	for _, entry := range base {
		key := envEntryKey(entry)
		if blocked[key] {
			continue
		}
		filtered = append(filtered, entry)
	}

	for key, value := range env.Values {
		filtered = append(filtered, key+"="+value)
	}

	filtered = append(filtered,
		"ENV_FILE="+env.FilePath,
		"ESPO_ENV="+env.ResolvedContour,
		"RESOLVED_ENV_CONTOUR="+env.ResolvedContour,
	)

	keys := make([]string, 0, len(extra))
	for key := range extra {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		filtered = append(filtered, key+"="+extra[key])
	}

	return filtered
}

func KnownOperationalEnvVars() []string {
	return append([]string(nil), knownOperationalEnvVars...)
}

func ResolveProjectPath(projectDir, value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if filepath.IsAbs(value) {
		return filepath.Clean(value)
	}

	return filepath.Clean(filepath.Join(projectDir, strings.TrimPrefix(value, "./")))
}

func resolveOperationEnvFile(projectDir, scope, overridePath string) (string, error) {
	overridePath = strings.TrimSpace(overridePath)
	if overridePath != "" {
		if _, err := os.Stat(overridePath); err != nil {
			if os.IsNotExist(err) {
				return "", MissingEnvFileError{Path: overridePath}
			}
			return "", fmt.Errorf("stat env file %s: %w", overridePath, err)
		}
		return overridePath, nil
	}

	switch strings.TrimSpace(scope) {
	case "dev":
		return filepath.Join(projectDir, ".env.dev"), nil
	case "prod":
		return filepath.Join(projectDir, ".env.prod"), nil
	default:
		return "", UnsupportedContourError{Contour: scope}
	}
}

func validateEnvFileForLoading(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return MissingEnvFileError{Path: path}
		}
		return fmt.Errorf("stat env file %s: %w", path, err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return InvalidEnvFileError{Path: path, Message: "env file must not be a symlink"}
	}
	if !info.Mode().IsRegular() {
		return InvalidEnvFileError{Path: path, Message: "env file must be a regular file"}
	}

	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open env file %s: %w", path, err)
	}
	_ = f.Close()

	currentUID := os.Getuid()
	statT, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return fmt.Errorf("determine env file owner %s: unsupported stat payload", path)
	}
	if statT.Uid != uint32(currentUID) && statT.Uid != 0 {
		return InvalidEnvFileError{Path: path, Message: "env file must belong to the current user or root"}
	}

	perm := info.Mode().Perm()
	if perm&0o137 != 0 {
		return InvalidEnvFileError{Path: path, Message: fmt.Sprintf("env file must not be broader than 640 and must not have execute bits: current %03o", perm)}
	}

	return nil
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
		if strings.TrimSpace(line) == "" || strings.HasPrefix(strings.TrimSpace(line), "#") {
			continue
		}

		key, rawValue, ok := parseEnvAssignment(line)
		if !ok {
			return nil, EnvParseError{Path: path, Line: lineNo, Message: "expected a KEY=VALUE line without shell code"}
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
			if !isEnvAssignmentKeyStart(ch) {
				return "", "", false
			}
			continue
		}
		if !isEnvAssignmentKeyPart(ch) {
			return "", "", false
		}
	}

	return key, trimmed[sep+1:], true
}

func isEnvAssignmentKeyStart(ch rune) bool {
	return ch == '_' || ('A' <= ch && ch <= 'Z') || ('a' <= ch && ch <= 'z')
}

func isEnvAssignmentKeyPart(ch rune) bool {
	return isEnvAssignmentKeyStart(ch) || ('0' <= ch && ch <= '9')
}

func parseEnvValue(rawValue, path string, lineNo int) (string, error) {
	switch {
	case strings.HasPrefix(rawValue, "\""):
		return decodeDoubleQuotedEnvValue(rawValue, path, lineNo)
	case strings.HasPrefix(rawValue, "'"):
		return decodeSingleQuotedEnvValue(rawValue, path, lineNo)
	default:
		return decodeUnquotedEnvValue(rawValue, path, lineNo)
	}
}

func decodeDoubleQuotedEnvValue(rawValue, path string, lineNo int) (string, error) {
	if len(rawValue) < 2 || !strings.HasSuffix(rawValue, "\"") {
		return "", EnvParseError{Path: path, Line: lineNo, Message: "double-quoted value must end with a closing quote"}
	}

	inner := rawValue[1 : len(rawValue)-1]
	var decoded strings.Builder
	escapeNext := false

	for _, ch := range inner {
		if escapeNext {
			switch ch {
			case '\\', '"', '$', '`':
				decoded.WriteRune(ch)
			default:
				return "", EnvParseError{Path: path, Line: lineNo, Message: fmt.Sprintf("unsupported escape sequence \\%c", ch)}
			}
			escapeNext = false
			continue
		}

		switch ch {
		case '\\':
			escapeNext = true
		case '"':
			return "", EnvParseError{Path: path, Line: lineNo, Message: "inner double quotes must be escaped"}
		case '$', '`':
			return "", EnvParseError{Path: path, Line: lineNo, Message: "double-quoted value must not contain unescaped shell expansions"}
		default:
			decoded.WriteRune(ch)
		}
	}

	if escapeNext {
		return "", EnvParseError{Path: path, Line: lineNo, Message: "double-quoted value ends with an unfinished escape sequence"}
	}

	return decoded.String(), nil
}

func decodeSingleQuotedEnvValue(rawValue, path string, lineNo int) (string, error) {
	if len(rawValue) < 2 || !strings.HasSuffix(rawValue, "'") {
		return "", EnvParseError{Path: path, Line: lineNo, Message: "single-quoted value must end with a closing quote"}
	}

	inner := rawValue[1 : len(rawValue)-1]
	if strings.Contains(inner, "'") {
		return "", EnvParseError{Path: path, Line: lineNo, Message: "single-quoted value must not contain a raw single quote"}
	}

	return inner, nil
}

func decodeUnquotedEnvValue(rawValue, path string, lineNo int) (string, error) {
	if strings.Contains(rawValue, "$(") || strings.Contains(rawValue, "${") || strings.Contains(rawValue, "`") {
		return "", EnvParseError{Path: path, Line: lineNo, Message: "value must not contain shell expansions"}
	}
	if strings.ContainsAny(rawValue, " \t") {
		return "", EnvParseError{Path: path, Line: lineNo, Message: "a value containing spaces must be quoted"}
	}

	return rawValue, nil
}

func validateLoadedEnvContour(path, requestedScope, contourHint, declaredContour string) (string, error) {
	requestedScope = strings.TrimSpace(requestedScope)
	contourHint = strings.TrimSpace(contourHint)
	declaredContour = strings.TrimSpace(declaredContour)

	pathContour, err := inferEnvFileContourFromPath(path)
	if err != nil {
		pathContour = ""
	}

	if declaredContour != "" && !supportedContour(declaredContour) {
		return "", InvalidEnvFileError{Path: path, Message: fmt.Sprintf("ESPO_CONTOUR in the env file must be dev or prod: %s", declaredContour)}
	}
	if contourHint != "" && !supportedContour(contourHint) {
		return "", InvalidEnvFileError{Path: path, Message: fmt.Sprintf("ESPO_ENV_FILE_CONTOUR must be dev or prod: %s", contourHint)}
	}
	if pathContour != "" && declaredContour != "" && pathContour != declaredContour {
		return "", InvalidEnvFileError{Path: path, Message: fmt.Sprintf("env filename points to contour %q, but ESPO_CONTOUR=%s", pathContour, declaredContour)}
	}
	if contourHint != "" && declaredContour != "" && contourHint != declaredContour {
		return "", InvalidEnvFileError{Path: path, Message: fmt.Sprintf("ESPO_ENV_FILE_CONTOUR=%s conflicts with ESPO_CONTOUR=%s", contourHint, declaredContour)}
	}
	if contourHint != "" && pathContour != "" && contourHint != pathContour {
		return "", InvalidEnvFileError{Path: path, Message: fmt.Sprintf("ESPO_ENV_FILE_CONTOUR=%s conflicts with the env filename for contour %q", contourHint, pathContour)}
	}

	effective := declaredContour
	if effective == "" {
		effective = contourHint
	}
	if effective == "" {
		effective = pathContour
	}
	if effective == "" {
		return "", InvalidEnvFileError{Path: path, Message: "could not determine env file contour; add ESPO_CONTOUR=dev|prod or set ESPO_ENV_FILE_CONTOUR=dev|prod"}
	}
	if effective != requestedScope {
		return "", InvalidEnvFileError{Path: path, Message: fmt.Sprintf("env file %q belongs to contour %q, but the command was run for %q", path, effective, requestedScope)}
	}

	return effective, nil
}

func inferEnvFileContourFromPath(path string) (string, error) {
	return inferContourTokenFromText(filepath.Base(path))
}

func inferContourTokenFromText(value string) (string, error) {
	found := ""
	for _, contour := range []string{"dev", "prod"} {
		if containsContourToken(value, contour) {
			if found != "" && found != contour {
				return "", fmt.Errorf("ambiguous contour")
			}
			found = contour
		}
	}
	if found == "" {
		return "", fmt.Errorf("no contour token")
	}
	return found, nil
}

func containsContourToken(text, token string) bool {
	offset := 0
	for {
		idx := strings.Index(text[offset:], token)
		if idx < 0 {
			return false
		}
		start := offset + idx
		beforeOK := start == 0 || !isAlphaNum(rune(text[start-1]))
		afterIdx := start + len(token)
		afterOK := afterIdx >= len(text) || !isAlphaNum(rune(text[afterIdx]))
		if beforeOK && afterOK {
			return true
		}
		offset = start + 1
	}
}

func isAlphaNum(ch rune) bool {
	return ('a' <= ch && ch <= 'z') || ('A' <= ch && ch <= 'Z') || ('0' <= ch && ch <= '9')
}

func supportedContour(value string) bool {
	switch value {
	case "dev", "prod":
		return true
	default:
		return false
	}
}

func envEntryKey(entry string) string {
	if before, _, ok := strings.Cut(entry, "="); ok {
		return before
	}
	return entry
}

type MissingEnvFileError struct {
	Path string
}

func (e MissingEnvFileError) Error() string {
	return fmt.Sprintf("missing %s", e.Path)
}

type UnsupportedContourError struct {
	Contour string
}

func (e UnsupportedContourError) Error() string {
	return fmt.Sprintf("unsupported contour %q. use dev or prod", e.Contour)
}

type InvalidEnvFileError struct {
	Path    string
	Message string
}

func (e InvalidEnvFileError) Error() string {
	if strings.TrimSpace(e.Path) == "" {
		return e.Message
	}
	return fmt.Sprintf("%s: %s", e.Message, e.Path)
}

type EnvParseError struct {
	Path    string
	Line    int
	Message string
}

func (e EnvParseError) Error() string {
	return fmt.Sprintf("env file %q contains unsupported syntax on line %d: %s", e.Path, e.Line, e.Message)
}

type MissingEnvValueError struct {
	Path string
	Name string
}

func (e MissingEnvValueError) Error() string {
	return fmt.Sprintf("%s is not set in %s", e.Name, e.Path)
}
