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

func CreateTarGz(ctx context.Context, sourceDir, destPath string, entries []string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if strings.TrimSpace(sourceDir) == "" {
		return fmt.Errorf("tar source dir is required")
	}
	if strings.TrimSpace(destPath) == "" {
		return fmt.Errorf("tar destination path is required")
	}
	if len(entries) == 0 {
		return fmt.Errorf("tar entries are required")
	}

	var stdin bytes.Buffer
	for _, entry := range entries {
		if entry == "" {
			return fmt.Errorf("tar entries must be non-empty")
		}
		if strings.IndexByte(entry, 0) >= 0 {
			return fmt.Errorf("tar entry contains NUL byte")
		}
		stdin.WriteString(entry)
		stdin.WriteByte(0)
	}

	return runNative(ctx, "tar", runOptions{stdin: &stdin},
		"-C", sourceDir,
		"--no-recursion",
		"--null",
		"-T", "-",
		"-czf", destPath,
	)
}

func (DockerCompose) ComposeConfig(ctx context.Context, target Target) error {
	return runCompose(ctx, target, runOptions{}, "config")
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

func (rt DockerCompose) RequireStoppedServices(ctx context.Context, target Target, services []string) error {
	statuses, err := rt.ServiceStatuses(ctx, target)
	if err != nil {
		return err
	}
	return requireStoppedServices(statuses, services)
}

func (DockerCompose) StopServices(ctx context.Context, target Target, services []string) error {
	args, err := serviceArgs("stop", services)
	if err != nil {
		return err
	}
	return runCompose(ctx, target, runOptions{}, args...)
}

func (DockerCompose) StartServices(ctx context.Context, target Target, services []string) error {
	args, err := serviceArgs("start", services)
	if err != nil {
		return err
	}
	return runCompose(ctx, target, runOptions{}, args...)
}

func (DockerCompose) UpService(ctx context.Context, target Target, service string) error {
	service = strings.TrimSpace(service)
	if service == "" {
		return fmt.Errorf("service is required")
	}
	return runCompose(ctx, target, runOptions{}, "up", "-d", service)
}

func (DockerCompose) DumpDatabase(ctx context.Context, target Target, destPath string) (err error) {
	file, err := os.Create(destPath)
	if err != nil {
		return fmt.Errorf("create db backup file: %w", err)
	}
	defer closeResource(file, &err)

	gz := gzip.NewWriter(file)
	defer closeResource(gz, &err)

	service, err := dbServiceName(target)
	if err != nil {
		return err
	}
	password, err := requiredPassword("db password", target.DBPassword)
	if err != nil {
		return err
	}

	if err := runCompose(ctx, target, runOptions{
		stdout: gz,
		env:    []string{"MYSQL_PWD=" + password},
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
	); err != nil {
		return err
	}

	return nil
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

func (DockerCompose) RestoreDatabase(ctx context.Context, target Target, reader io.Reader) error {
	password, err := requiredPassword("db password", target.DBPassword)
	if err != nil {
		return err
	}
	return runMariaDBExecWithOptions(ctx, target, password, runOptions{
		stdin: reader,
	},
		"mariadb",
		"-u", target.DBUser,
		target.DBName,
	)
}

type runOptions struct {
	stdin  io.Reader
	stdout io.Writer
	env    []string
}

func runCompose(ctx context.Context, target Target, opts runOptions, args ...string) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	cmdArgs := append([]string{
		"compose",
		"--env-file", strings.TrimSpace(target.EnvFile),
		"-f", strings.TrimSpace(target.ComposeFile),
	}, args...)

	cmd := exec.CommandContext(ctx, "docker", cmdArgs...)
	cmd.Dir = strings.TrimSpace(target.ProjectDir)
	cmd.Stdin = opts.stdin
	if opts.stdout != nil {
		cmd.Stdout = opts.stdout
	} else {
		cmd.Stdout = io.Discard
	}
	cmd.Env = commandEnv(os.Environ(), opts.env)

	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		command := sanitizeComposeCommand(target, cmdArgs)
		message := sanitizeComposeText(target, strings.TrimSpace(stderr.String()))
		if message == "" {
			return fmt.Errorf("docker %s: %w", command, err)
		}
		return fmt.Errorf("docker %s: %s: %w", command, message, err)
	}

	return nil
}

func runNative(ctx context.Context, name string, opts runOptions, args ...string) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdin = opts.stdin
	if opts.stdout != nil {
		cmd.Stdout = opts.stdout
	} else {
		cmd.Stdout = io.Discard
	}
	cmd.Env = commandEnv(os.Environ(), opts.env)

	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		command := nativeCommand(name, args)
		message := strings.TrimSpace(stderr.String())
		if message == "" {
			return fmt.Errorf("%s: %w", command, err)
		}
		return fmt.Errorf("%s: %s: %w", command, message, err)
	}

	return nil
}

func nativeCommand(name string, args []string) string {
	command := make([]string, 0, len(args)+1)
	command = append(command, name)
	command = append(command, args...)
	return strings.Join(command, " ")
}

func dbServiceName(target Target) (string, error) {
	service := strings.TrimSpace(target.DBService)
	if service == "" {
		return "", fmt.Errorf("db service is required")
	}
	return service, nil
}

func runMariaDBExec(ctx context.Context, target Target, password string, args ...string) error {
	return runMariaDBExecWithOptions(ctx, target, password, runOptions{}, args...)
}

func runMariaDBExecWithOptions(ctx context.Context, target Target, password string, opts runOptions, args ...string) error {
	service, err := dbServiceName(target)
	if err != nil {
		return err
	}

	opts.env = append([]string{"MYSQL_PWD=" + password}, opts.env...)
	composeArgs := append([]string{"exec", "-T", "-e", "MYSQL_PWD", service}, args...)
	return runCompose(ctx, target, opts, composeArgs...)
}

func requiredPassword(label, value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", fmt.Errorf("%s is required", label)
	}
	return value, nil
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

func closeResource(closer io.Closer, errp *error) {
	if closer == nil {
		return
	}
	if closeErr := closer.Close(); closeErr != nil && *errp == nil {
		*errp = closeErr
	}
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

func requireStoppedServices(statuses []ServiceStatus, services []string) error {
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
			continue
		}
		if !isStoppedServiceState(status.State) {
			return fmt.Errorf("service %q state is %q (want stopped)", name, statusValue(status.State))
		}
	}

	return nil
}

func isStoppedServiceState(state string) bool {
	switch strings.ToLower(strings.TrimSpace(state)) {
	case "exited", "created", "dead":
		return true
	default:
		return false
	}
}

func statusValue(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "unknown"
	}
	return value
}
