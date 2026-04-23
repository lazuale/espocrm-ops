package runtime

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/lazuale/espocrm-ops/internal/model"
	platformdocker "github.com/lazuale/espocrm-ops/internal/platform/docker"
	platformfs "github.com/lazuale/espocrm-ops/internal/platform/fs"
)

type Docker struct{}

func (Docker) RunningServices(ctx context.Context, target model.RuntimeTarget) ([]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return platformdocker.ComposeRunningServices(composeConfig(target))
}

func (Docker) StopServices(ctx context.Context, target model.RuntimeTarget, services ...string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return platformdocker.ComposeStop(composeConfig(target), services...)
}

func (Docker) StartServices(ctx context.Context, target model.RuntimeTarget, services ...string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return platformdocker.ComposeUp(composeConfig(target), services...)
}

func (Docker) DumpDatabase(ctx context.Context, target model.RuntimeTarget) (io.ReadCloser, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	tmp, err := os.CreateTemp("", "espops-backup-v2-db-*.sql.gz")
	if err != nil {
		return nil, fmt.Errorf("создать временный db artifact: %w", err)
	}
	tmpPath := tmp.Name()
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return nil, fmt.Errorf("закрыть временный db artifact: %w", err)
	}

	dbService := strings.TrimSpace(target.DBService)
	if dbService == "" {
		dbService = "db"
	}
	if err := platformdocker.DumpMySQLDumpGz(composeConfig(target), dbService, target.DBUser, target.DBPassword, target.DBName, tmpPath); err != nil {
		_ = os.Remove(tmpPath)
		return nil, err
	}
	return openRemoveOnClose(tmpPath)
}

func (Docker) ArchiveFiles(ctx context.Context, target model.RuntimeTarget) (io.ReadCloser, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	tmp, err := os.CreateTemp("", "espops-backup-v2-files-*.tar.gz")
	if err != nil {
		return nil, fmt.Errorf("создать временный files artifact: %w", err)
	}
	tmpPath := tmp.Name()
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return nil, fmt.Errorf("закрыть временный files artifact: %w", err)
	}
	if err := platformfs.CreateTarGz(target.StorageDir, tmpPath); err != nil {
		_ = os.Remove(tmpPath)
		return nil, err
	}
	return openRemoveOnClose(tmpPath)
}

func (Docker) ArchiveFilesWithHelper(ctx context.Context, target model.RuntimeTarget, contract model.HelperArchiveContract) (io.ReadCloser, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	tmp, err := os.CreateTemp("", "espops-backup-v2-files-helper-*.tar.gz")
	if err != nil {
		return nil, fmt.Errorf("создать временный files artifact для helper: %w", err)
	}
	tmpPath := tmp.Name()
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return nil, fmt.Errorf("закрыть временный files artifact для helper: %w", err)
	}
	if err := platformdocker.CreateTarArchiveViaHelper(target.StorageDir, tmpPath, contract.Image); err != nil {
		_ = os.Remove(tmpPath)
		return nil, err
	}
	return openRemoveOnClose(tmpPath)
}

func composeConfig(target model.RuntimeTarget) platformdocker.ComposeConfig {
	return platformdocker.ComposeConfig{
		ProjectDir:  target.ProjectDir,
		ComposeFile: target.ComposeFile,
		EnvFile:     target.EnvFile,
	}
}

type removeOnClose struct {
	*os.File
	path string
}

func openRemoveOnClose(path string) (*removeOnClose, error) {
	file, err := os.Open(path)
	if err != nil {
		_ = os.Remove(path)
		return nil, err
	}
	return &removeOnClose{File: file, path: path}, nil
}

func (r *removeOnClose) Close() error {
	if r == nil || r.File == nil {
		return nil
	}
	closeErr := r.File.Close()
	removeErr := os.Remove(r.path)
	if closeErr != nil {
		return closeErr
	}
	return removeErr
}
