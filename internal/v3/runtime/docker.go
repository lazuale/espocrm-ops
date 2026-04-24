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

func (DockerCompose) DumpDatabase(ctx context.Context, target Target, destPath string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return platformdocker.DumpMariaDBDumpGzViaCompose(
		composeConfig(target),
		strings.TrimSpace(target.DBService),
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
