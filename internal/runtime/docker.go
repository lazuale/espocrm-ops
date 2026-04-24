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
		service = "db"
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

	service := strings.TrimSpace(target.DBService)
	if service == "" {
		service = "db"
	}

	if err := runCompose(ctx, target, runOptions{
		stdout: gz,
	},
		"exec", "-T", "-e", "MYSQL_PWD="+target.DBPassword, service,
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
	service := strings.TrimSpace(target.DBService)
	if service == "" {
		service = "db"
	}

	return runCompose(ctx, target, runOptions{
	},
		"exec", "-T", "-e", "MYSQL_PWD="+target.DBPassword, service,
		"mariadb",
		"-u", target.DBUser,
		target.DBName,
		"-e", "SELECT 1;",
	)
}

func (DockerCompose) RestoreDatabase(ctx context.Context, target Target, reader io.Reader) error {
	service := strings.TrimSpace(target.DBService)
	if service == "" {
		service = "db"
	}

	return runCompose(ctx, target, runOptions{
		stdin: reader,
	},
		"exec", "-T", "-e", "MYSQL_PWD="+target.DBPassword, service,
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
	cmd.Env = append(os.Environ(), opts.env...)

	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		message := strings.TrimSpace(stderr.String())
		if message == "" {
			return fmt.Errorf("docker %s: %w", strings.Join(cmdArgs, " "), err)
		}
		return fmt.Errorf("docker %s: %s: %w", strings.Join(cmdArgs, " "), message, err)
	}

	return nil
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
