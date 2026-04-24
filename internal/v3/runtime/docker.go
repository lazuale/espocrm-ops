package runtime

import (
	"bytes"
	"compress/gzip"
	"context"
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

type DockerCompose struct{}

func (DockerCompose) Validate(ctx context.Context, target Target) error {
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
	args, err := serviceArgs("start", services)
	if err != nil {
		return err
	}
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
