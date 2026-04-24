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
	ProjectDir  string
	ComposeFile string
	EnvFile     string
	DBService   string
	DBUser      string
	DBPassword  string
	DBName      string
}

type Service struct {
	Name string `json:"Service"`
}

type DockerCompose struct{}

func (DockerCompose) ComposeConfig(ctx context.Context, target Target) error {
	return runCompose(ctx, target, runOptions{}, "config")
}

func (DockerCompose) Validate(ctx context.Context, target Target) error {
	return DockerCompose{}.ComposeConfig(ctx, target)
}

func (DockerCompose) Services(ctx context.Context, target Target) ([]Service, error) {
	var stdout bytes.Buffer
	if err := runCompose(ctx, target, runOptions{stdout: &stdout}, "ps", "--format", "json"); err != nil {
		return nil, err
	}

	services, err := decodeServices(stdout.Bytes())
	if err != nil {
		return nil, fmt.Errorf("decode docker compose ps output: %w", err)
	}
	return services, nil
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

	if err := runCompose(ctx, target, runOptions{
		stdout: gz,
		env:    []string{"MYSQL_PWD=" + target.DBPassword},
	},
		"exec", "-T", "-e", "MYSQL_PWD", service,
		"mariadb-dump",
		"--single-transaction",
		"--quick",
		"--routines",
		"--triggers",
		"--events",
		"-u", target.DBUser,
		target.DBName,
	); err != nil {
		return err
	}

	return nil
}

func (DockerCompose) DBPing(ctx context.Context, target Target) error {
	service, err := dbServiceName(target)
	if err != nil {
		return err
	}

	return runCompose(ctx, target, runOptions{
		env: []string{"MYSQL_PWD=" + target.DBPassword},
	},
		"exec", "-T", "-e", "MYSQL_PWD", service,
		"mariadb",
		"-u", target.DBUser,
		target.DBName,
		"-e", "SELECT 1;",
	)
}

func (DockerCompose) RestoreDatabase(ctx context.Context, target Target, reader io.Reader) error {
	service, err := dbServiceName(target)
	if err != nil {
		return err
	}

	return runCompose(ctx, target, runOptions{
		stdin: reader,
		env:   []string{"MYSQL_PWD=" + target.DBPassword},
	},
		"exec", "-T", "-e", "MYSQL_PWD", service,
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
	cmd.Env = mergeCommandEnv(os.Environ(), opts.env)

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

func dbServiceName(target Target) (string, error) {
	service := strings.TrimSpace(target.DBService)
	if service == "" {
		return "", fmt.Errorf("db service is required")
	}
	return service, nil
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
	text = redactEnvAssignments(text, "MYSQL_PWD", "DB_PASSWORD")

	secret := target.DBPassword
	if secret == "" {
		return text
	}
	return strings.ReplaceAll(text, secret, redactedSecret)
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

func decodeServices(raw []byte) ([]Service, error) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 {
		return nil, io.ErrUnexpectedEOF
	}

	if raw[0] == '[' {
		var services []Service
		if err := json.Unmarshal(raw, &services); err != nil {
			return nil, err
		}
		return services, nil
	}

	decoder := json.NewDecoder(bytes.NewReader(raw))
	services := make([]Service, 0, 4)
	for {
		var service Service
		if err := decoder.Decode(&service); err != nil {
			if err == io.EOF {
				break
			}
			return nil, err
		}
		services = append(services, service)
	}
	return services, nil
}
