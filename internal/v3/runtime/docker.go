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
	Name  string `json:"Service"`
	State string `json:"State"`
}

type DockerCompose struct{}

func (DockerCompose) Validate(ctx context.Context, target Target) error {
	return runCompose(ctx, target, runOptions{}, "config")
}

func (DockerCompose) Services(ctx context.Context, target Target) ([]Service, error) {
	var stdout bytes.Buffer
	if err := runCompose(ctx, target, runOptions{stdout: &stdout}, "ps", "--format", "json"); err != nil {
		return nil, err
	}

	services := []Service{}
	if err := json.Unmarshal(stdout.Bytes(), &services); err != nil {
		return nil, fmt.Errorf("decode docker compose ps json: %w", err)
	}
	return services, nil
}

func (DockerCompose) Stop(ctx context.Context, target Target, services ...string) error {
	args := append([]string{"stop"}, trimServices(services)...)
	return runCompose(ctx, target, runOptions{}, args...)
}

func (DockerCompose) Start(ctx context.Context, target Target, services ...string) error {
	args := append([]string{"start"}, trimServices(services)...)
	return runCompose(ctx, target, runOptions{}, args...)
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
		env:    []string{"MYSQL_PWD=" + target.DBPassword},
	},
		"exec", "-T", service,
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

func trimServices(services []string) []string {
	out := make([]string, 0, len(services))
	for _, service := range services {
		service = strings.TrimSpace(service)
		if service == "" {
			continue
		}
		out = append(out, service)
	}
	return out
}

func closeResource(closer io.Closer, errp *error) {
	if closer == nil {
		return
	}
	if closeErr := closer.Close(); closeErr != nil && *errp == nil {
		*errp = closeErr
	}
}
