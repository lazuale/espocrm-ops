package runtime

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
)

const redactedSecret = "<redacted>"

type Target struct {
	ProjectDir     string
	ComposeFile    string
	EnvFile        string
	DBService      string
	DBUser         string
	DBPassword     string
	DBRootPassword string
	DBName         string
}

type ServiceStatus struct {
	Name   string `json:"Service"`
	State  string `json:"State"`
	Health string `json:"Health"`
}

type ServiceHealthError struct {
	Service   string
	State     string
	Health    string
	Message   string
	Retryable bool
}

func (e *ServiceHealthError) Error() string {
	if e == nil {
		return ""
	}
	return e.Message
}

type DockerCompose struct{}

func TarExists() error {
	if _, err := exec.LookPath("tar"); err != nil {
		return fmt.Errorf("native tar is required: %w", err)
	}
	return nil
}

func TestTarGz(ctx context.Context, path string) error {
	return runTar(ctx, "-tzf", path)
}

func CreateStorageArchive(ctx context.Context, sourceDir, destPath string) error {
	if strings.TrimSpace(sourceDir) == "" {
		return fmt.Errorf("storage source dir is required")
	}
	if strings.TrimSpace(destPath) == "" {
		return fmt.Errorf("storage archive destination is required")
	}
	return runTar(ctx, "-C", sourceDir, "-czf", destPath, ".")
}

func ExtractStorageArchive(ctx context.Context, archivePath, destDir string) error {
	if strings.TrimSpace(archivePath) == "" {
		return fmt.Errorf("storage archive path is required")
	}
	if strings.TrimSpace(destDir) == "" {
		return fmt.Errorf("storage archive destination is required")
	}
	return runTar(ctx, "-xzf", archivePath, "-C", destDir)
}

func (DockerCompose) ComposeConfig(ctx context.Context, target Target) error {
	return runCompose(ctx, target, runOptions{}, "config")
}

func (DockerCompose) StopServices(ctx context.Context, target Target, services []string) error {
	args, err := serviceArgs("stop", services)
	if err != nil {
		return err
	}
	return runCompose(ctx, target, runOptions{}, args...)
}

func (DockerCompose) StartServices(ctx context.Context, target Target, services []string) error {
	args, err := upServiceArgs(services)
	if err != nil {
		return err
	}
	return runCompose(ctx, target, runOptions{}, args...)
}

func (DockerCompose) ServiceStatuses(ctx context.Context, target Target) ([]ServiceStatus, error) {
	var stdout bytes.Buffer
	if err := runCompose(ctx, target, runOptions{stdout: &stdout}, "ps", "--format", "json"); err != nil {
		return nil, err
	}
	statuses, err := decodeServiceStatuses(stdout.Bytes())
	if err != nil {
		return nil, fmt.Errorf("decode docker compose ps output: %w", err)
	}
	return statuses, nil
}

func (rt DockerCompose) RequireHealthyServices(ctx context.Context, target Target, services []string) error {
	statuses, err := rt.ServiceStatuses(ctx, target)
	if err != nil {
		return err
	}
	return requireHealthyServices(statuses, services)
}

func (DockerCompose) DumpDatabase(ctx context.Context, target Target, destPath string) (err error) {
	if err := ctx.Err(); err != nil {
		return err
	}
	password, err := requiredPassword("db password", target.DBPassword)
	if err != nil {
		return err
	}
	service, err := dbServiceName(target)
	if err != nil {
		return err
	}

	file, err := os.Create(destPath)
	if err != nil {
		return fmt.Errorf("create db backup file: %w", err)
	}
	defer closeResource(file, &err)

	writer := gzip.NewWriter(file)
	defer closeResource(writer, &err)

	return runCompose(ctx, target, runOptions{
		env:    []string{"MYSQL_PWD=" + password},
		stdout: writer,
	},
		"exec", "-T", "-e", "MYSQL_PWD", service,
		"mariadb-dump",
		"--single-transaction",
		"--quick",
		"--routines",
		"--triggers",
		"--events",
		"--hex-blob",
		"--default-character-set=utf8mb4",
		"-u", target.DBUser,
		target.DBName,
	)
}

func (DockerCompose) ResetDatabase(ctx context.Context, target Target) error {
	password, err := requiredPassword("db root password", target.DBRootPassword)
	if err != nil {
		return err
	}
	return runMariaDBExec(ctx, target, password,
		"mariadb",
		"-u", "root",
		"-e", resetDatabaseSQL(target.DBName),
	)
}

func (DockerCompose) RestoreDatabase(ctx context.Context, target Target, sourcePath string) (err error) {
	if err := ctx.Err(); err != nil {
		return err
	}
	password, err := requiredPassword("db password", target.DBPassword)
	if err != nil {
		return err
	}
	service, err := dbServiceName(target)
	if err != nil {
		return err
	}

	file, err := os.Open(sourcePath)
	if err != nil {
		return fmt.Errorf("open db backup file: %w", err)
	}
	defer closeResource(file, &err)

	reader, err := gzip.NewReader(file)
	if err != nil {
		return fmt.Errorf("open db backup gzip stream: %w", err)
	}
	defer closeResource(reader, &err)

	return runCompose(ctx, target, runOptions{
		env:   []string{"MYSQL_PWD=" + password},
		stdin: reader,
	},
		"exec", "-T", "-e", "MYSQL_PWD", service,
		"mariadb",
		"-u", target.DBUser,
		target.DBName,
	)
}

func (DockerCompose) DBPing(ctx context.Context, target Target) error {
	password, err := requiredPassword("db password", target.DBPassword)
	if err != nil {
		return err
	}
	return runMariaDBExec(ctx, target, password,
		"mariadb",
		"-u", target.DBUser,
		target.DBName,
		"-e", "SELECT 1;",
	)
}

type runOptions struct {
	stdin  io.Reader
	stdout io.Writer
	env    []string
}

func runCompose(ctx context.Context, target Target, opts runOptions, args ...string) error {
	cmd := composeCommand(ctx, target, opts, args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return commandError(target, "docker "+sanitizeComposeCommand(target, composeArgs(target, args...)), stderr.String(), err)
	}
	return nil
}

func composeCommand(ctx context.Context, target Target, opts runOptions, args ...string) *exec.Cmd {
	cmdArgs := composeArgs(target, args...)
	cmd := exec.CommandContext(ctx, "docker", cmdArgs...)
	cmd.Dir = strings.TrimSpace(target.ProjectDir)
	cmd.Stdin = opts.stdin
	if opts.stdout != nil {
		cmd.Stdout = opts.stdout
	}
	cmd.Env = commandEnv(os.Environ(), opts.env)
	return cmd
}

func composeArgs(target Target, args ...string) []string {
	out := []string{
		"compose",
		"--env-file", strings.TrimSpace(target.EnvFile),
		"-f", strings.TrimSpace(target.ComposeFile),
	}
	return append(out, args...)
}

func runTar(ctx context.Context, args ...string) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	cmd := exec.CommandContext(ctx, "tar", args...)
	cmd.Env = commandEnv(os.Environ(), nil)

	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return commandError(Target{}, "tar "+strings.Join(args, " "), stderr.String(), err)
	}
	return nil
}

func runMariaDBExec(ctx context.Context, target Target, password string, args ...string) error {
	service, err := dbServiceName(target)
	if err != nil {
		return err
	}
	return runCompose(ctx, target, runOptions{
		env: []string{"MYSQL_PWD=" + password},
	}, append([]string{"exec", "-T", "-e", "MYSQL_PWD", service}, args...)...)
}

func requiredPassword(label, value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", fmt.Errorf("%s is required", label)
	}
	return value, nil
}

func dbServiceName(target Target) (string, error) {
	service := strings.TrimSpace(target.DBService)
	if service == "" {
		return "", fmt.Errorf("db service is required")
	}
	return service, nil
}

func resetDatabaseSQL(name string) string {
	return fmt.Sprintf(
		"DROP DATABASE IF EXISTS %s; CREATE DATABASE %s CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;",
		quoteSQLIdentifier(name),
		quoteSQLIdentifier(name),
	)
}

func quoteSQLIdentifier(name string) string {
	return "`" + strings.ReplaceAll(name, "`", "``") + "`"
}

func commandError(target Target, command, stderr string, err error) error {
	message := sanitizeComposeText(target, strings.TrimSpace(stderr))
	command = sanitizeComposeText(target, command)
	if message == "" {
		return fmt.Errorf("%s: %w", command, err)
	}
	return fmt.Errorf("%s: %s: %w", command, message, err)
}

func commandEnv(base, overrides []string) []string {
	return mergeCommandEnv(allowedDockerEnv(base), explicitDockerEnv(overrides))
}

func allowedDockerEnv(env []string) []string {
	out := make([]string, 0, len(env))
	for _, entry := range env {
		key, _, ok := strings.Cut(entry, "=")
		if !ok || !isAllowedDockerEnvKey(key) {
			continue
		}
		out = append(out, entry)
	}
	return out
}

func isAllowedDockerEnvKey(key string) bool {
	switch key {
	case "PATH",
		"HOME",
		"DOCKER_HOST",
		"DOCKER_CONTEXT",
		"DOCKER_CONFIG",
		"DOCKER_CERT_PATH",
		"DOCKER_TLS_VERIFY",
		"SSH_AUTH_SOCK",
		"XDG_RUNTIME_DIR",
		"HTTP_PROXY",
		"HTTPS_PROXY",
		"NO_PROXY",
		"ALL_PROXY",
		"http_proxy",
		"https_proxy",
		"no_proxy",
		"all_proxy":
		return true
	default:
		return false
	}
}

func explicitDockerEnv(env []string) []string {
	out := make([]string, 0, len(env))
	for _, entry := range env {
		key, _, ok := strings.Cut(entry, "=")
		if ok && key == "MYSQL_PWD" {
			out = append(out, entry)
		}
	}
	return out
}

func mergeCommandEnv(base, overrides []string) []string {
	if len(overrides) == 0 {
		return append([]string(nil), base...)
	}

	indexByKey := make(map[string]int, len(base)+len(overrides))
	out := make([]string, 0, len(base)+len(overrides))
	for _, entry := range base {
		key, _, _ := strings.Cut(entry, "=")
		if idx, ok := indexByKey[key]; ok {
			out[idx] = entry
			continue
		}
		indexByKey[key] = len(out)
		out = append(out, entry)
	}
	for _, entry := range overrides {
		key, _, _ := strings.Cut(entry, "=")
		if idx, ok := indexByKey[key]; ok {
			out[idx] = entry
			continue
		}
		indexByKey[key] = len(out)
		out = append(out, entry)
	}
	return out
}

func sanitizeComposeCommand(target Target, args []string) string {
	sanitized := make([]string, len(args))
	for i, arg := range args {
		sanitized[i] = sanitizeComposeText(target, arg)
	}
	return strings.Join(sanitized, " ")
}

func sanitizeComposeText(target Target, text string) string {
	text = redactEnvAssignments(text, "MYSQL_PWD", "DB_PASSWORD", "DB_ROOT_PASSWORD")
	for _, secret := range []string{target.DBPassword, target.DBRootPassword} {
		secret = strings.TrimSpace(secret)
		if secret == "" {
			continue
		}
		text = strings.ReplaceAll(text, secret, redactedSecret)
	}
	return text
}

func redactEnvAssignments(text string, keys ...string) string {
	for _, key := range keys {
		text = redactEnvAssignment(text, key)
	}
	return text
}

func redactEnvAssignment(text, key string) string {
	marker := key + "="
	searchFrom := 0
	for {
		relativeStart := strings.Index(text[searchFrom:], marker)
		if relativeStart < 0 {
			return text
		}
		start := searchFrom + relativeStart
		end := start + len(marker)
		for end < len(text) && !isSecretDelimiter(text[end]) {
			end++
		}
		text = text[:start] + marker + redactedSecret + text[end:]
		searchFrom = start + len(marker) + len(redactedSecret)
	}
}

func isSecretDelimiter(b byte) bool {
	switch b {
	case ' ', '\t', '\n', '\r', '"', '\'', ',', ';', ')', '(':
		return true
	default:
		return false
	}
}

func serviceArgs(action string, services []string) ([]string, error) {
	out := make([]string, 1, len(services)+1)
	out[0] = action
	for _, service := range services {
		service = strings.TrimSpace(service)
		if service == "" {
			return nil, fmt.Errorf("%s service names must be non-empty", action)
		}
		out = append(out, service)
	}
	if len(out) == 1 {
		return nil, fmt.Errorf("%s requires at least one service", action)
	}
	return out, nil
}

func upServiceArgs(services []string) ([]string, error) {
	out := make([]string, 0, len(services)+2)
	out = append(out, "up", "-d")
	for _, service := range services {
		service = strings.TrimSpace(service)
		if service == "" {
			return nil, fmt.Errorf("up service names must be non-empty")
		}
		out = append(out, service)
	}
	if len(out) == 2 {
		return nil, fmt.Errorf("up requires at least one service")
	}
	return out, nil
}

func decodeServiceStatuses(raw []byte) ([]ServiceStatus, error) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 {
		return nil, io.ErrUnexpectedEOF
	}

	if raw[0] == '[' {
		var services []ServiceStatus
		if err := json.Unmarshal(raw, &services); err != nil {
			return nil, err
		}
		normalizeServiceStatuses(services)
		return services, nil
	}

	decoder := json.NewDecoder(bytes.NewReader(raw))
	services := make([]ServiceStatus, 0, 4)
	for {
		var service ServiceStatus
		if err := decoder.Decode(&service); err != nil {
			if err == io.EOF {
				break
			}
			return nil, err
		}
		normalizeServiceStatus(&service)
		services = append(services, service)
	}
	return services, nil
}

func normalizeServiceStatuses(services []ServiceStatus) {
	for i := range services {
		normalizeServiceStatus(&services[i])
	}
}

func normalizeServiceStatus(service *ServiceStatus) {
	if service == nil {
		return
	}
	service.Name = strings.TrimSpace(service.Name)
	service.State = strings.ToLower(strings.TrimSpace(service.State))
	service.Health = strings.ToLower(strings.TrimSpace(service.Health))
}

func requireHealthyServices(statuses []ServiceStatus, services []string) error {
	available := make(map[string]ServiceStatus, len(statuses))
	for _, status := range statuses {
		name := strings.TrimSpace(status.Name)
		if name == "" {
			continue
		}
		available[name] = status
	}

	for _, service := range services {
		name := strings.TrimSpace(service)
		if name == "" {
			return fmt.Errorf("service names must be non-empty")
		}

		status, ok := available[name]
		if !ok {
			return &ServiceHealthError{
				Service: name,
				Message: fmt.Sprintf("service %q not found in docker compose ps output", name),
			}
		}
		if status.State != "running" {
			return &ServiceHealthError{
				Service: name,
				State:   statusValue(status.State),
				Message: fmt.Sprintf("service %q state is %q (want \"running\")", name, statusValue(status.State)),
			}
		}
		if status.Health == "" {
			return &ServiceHealthError{
				Service: name,
				State:   status.State,
				Message: fmt.Sprintf("service %q has no docker compose health status", name),
			}
		}
		if status.Health == "starting" {
			return &ServiceHealthError{
				Service:   name,
				State:     status.State,
				Health:    status.Health,
				Message:   fmt.Sprintf("service %q health is %q (want \"healthy\")", name, status.Health),
				Retryable: true,
			}
		}
		if status.Health != "healthy" {
			return &ServiceHealthError{
				Service: name,
				State:   status.State,
				Health:  status.Health,
				Message: fmt.Sprintf("service %q health is %q (want \"healthy\")", name, status.Health),
			}
		}
	}

	return nil
}

func statusValue(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "unknown"
	}
	return value
}

func closeResource(closer io.Closer, errp *error) {
	if closer == nil {
		return
	}
	if closeErr := closer.Close(); closeErr != nil && *errp == nil {
		*errp = closeErr
	}
}
