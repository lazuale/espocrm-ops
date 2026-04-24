package runtime

import (
	"context"
	"strings"

	platformdocker "github.com/lazuale/espocrm-ops/internal/platform/docker"
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
	if err := ctx.Err(); err != nil {
		return err
	}
	return platformdocker.ValidateComposeConfig(composeConfig(target))
}

func (DockerCompose) RunningServices(ctx context.Context, target Target) ([]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return platformdocker.ComposeRunningServices(composeConfig(target))
}

func (DockerCompose) StopServices(ctx context.Context, target Target, services ...string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return platformdocker.ComposeStop(composeConfig(target), services...)
}

func (DockerCompose) StartServices(ctx context.Context, target Target, services ...string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return platformdocker.ComposeUp(composeConfig(target), services...)
}

func (DockerCompose) DumpDatabase(ctx context.Context, target Target, destPath string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	service := strings.TrimSpace(target.DBService)
	if service == "" {
		service = "db"
	}
	return platformdocker.DumpMySQLDumpGz(
		composeConfig(target),
		service,
		target.DBUser,
		target.DBPassword,
		target.DBName,
		destPath,
	)
}

func composeConfig(target Target) platformdocker.ComposeConfig {
	return platformdocker.ComposeConfig{
		ProjectDir:  strings.TrimSpace(target.ProjectDir),
		ComposeFile: strings.TrimSpace(target.ComposeFile),
		EnvFile:     strings.TrimSpace(target.EnvFile),
	}
}
