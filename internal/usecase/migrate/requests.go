package migrate

import (
	"strings"

	platformconfig "github.com/lazuale/espocrm-ops/internal/platform/config"
	maintenanceusecase "github.com/lazuale/espocrm-ops/internal/usecase/maintenance"
	restoreusecase "github.com/lazuale/espocrm-ops/internal/usecase/restore"
)

func buildDBRestoreRequest(ctx maintenanceusecase.OperationContext, info ExecuteInfo, dbContainer string) restoreusecase.RestoreDBRequest {
	req := restoreusecase.RestoreDBRequest{
		DBContainer:    dbContainer,
		DBName:         strings.TrimSpace(ctx.Env.Value("DB_NAME")),
		DBUser:         strings.TrimSpace(ctx.Env.Value("DB_USER")),
		DBPassword:     ctx.Env.Value("DB_PASSWORD"),
		DBRootPassword: ctx.Env.Value("DB_ROOT_PASSWORD"),
	}

	if strings.TrimSpace(info.ManifestJSONPath) != "" {
		req.ManifestPath = info.ManifestJSONPath
		return req
	}

	req.DBBackup = info.DBBackupPath
	return req
}

func buildFilesRestoreRequest(ctx maintenanceusecase.OperationContext, info ExecuteInfo) restoreusecase.RestoreFilesRequest {
	req := restoreusecase.RestoreFilesRequest{
		TargetDir: platformconfig.ResolveProjectPath(ctx.ProjectDir, ctx.Env.ESPOStorageDir()),
	}

	if strings.TrimSpace(info.ManifestJSONPath) != "" {
		req.ManifestPath = info.ManifestJSONPath
		return req
	}

	req.FilesBackup = info.FilesBackupPath
	return req
}
