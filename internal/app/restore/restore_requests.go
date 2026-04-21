package restore

import (
	"fmt"
	"strings"

	platformconfig "github.com/lazuale/espocrm-ops/internal/platform/config"
	maintenanceusecase "github.com/lazuale/espocrm-ops/internal/app/operation"
)

func buildDBRestoreRequest(ctx maintenanceusecase.OperationContext, source executeSource, dbContainer string) RestoreDBRequest {
	return RestoreDBRequest{
		DBBackup:       source.DBBackup,
		DBContainer:    dbContainer,
		DBName:         strings.TrimSpace(ctx.Env.Value("DB_NAME")),
		DBUser:         strings.TrimSpace(ctx.Env.Value("DB_USER")),
		DBPassword:     ctx.Env.Value("DB_PASSWORD"),
		DBRootPassword: ctx.Env.Value("DB_ROOT_PASSWORD"),
	}
}

func buildFilesRestoreRequest(ctx maintenanceusecase.OperationContext, source executeSource) RestoreFilesRequest {
	return RestoreFilesRequest{
		FilesBackup: source.FilesBackup,
		TargetDir:   platformconfig.ResolveProjectPath(ctx.ProjectDir, ctx.Env.ESPOStorageDir()),
	}
}

func configResolveDBPassword(req RestoreDBRequest) (string, error) {
	password, err := platformconfig.ResolveDBPassword(platformconfig.DBConfig{
		Container:    req.DBContainer,
		Name:         req.DBName,
		User:         req.DBUser,
		Password:     req.DBPassword,
		PasswordFile: req.DBPasswordFile,
	})
	if err != nil {
		return "", PreflightError{Err: fmt.Errorf("resolve db password: %w", err)}
	}
	return password, nil
}
