package restore

import (
	"strings"

	maintenanceusecase "github.com/lazuale/espocrm-ops/internal/app/operation"
	envport "github.com/lazuale/espocrm-ops/internal/app/ports/envport"
)

func (s Service) BuildDBRestoreRequest(ctx maintenanceusecase.OperationContext, manifestPath, dbBackup, dbContainer string) RestoreDBRequest {
	req := RestoreDBRequest{
		DBContainer:    dbContainer,
		DBName:         strings.TrimSpace(ctx.Env.Value("DB_NAME")),
		DBUser:         strings.TrimSpace(ctx.Env.Value("DB_USER")),
		DBPassword:     ctx.Env.Value("DB_PASSWORD"),
		DBRootPassword: ctx.Env.Value("DB_ROOT_PASSWORD"),
	}
	if cleaned := strings.TrimSpace(manifestPath); cleaned != "" {
		req.ManifestPath = cleaned
		return req
	}
	req.DBBackup = strings.TrimSpace(dbBackup)
	return req
}

func (s Service) BuildFilesRestoreRequest(ctx maintenanceusecase.OperationContext, manifestPath, filesBackup string) RestoreFilesRequest {
	req := RestoreFilesRequest{
		TargetDir: s.env.ResolveProjectPath(ctx.ProjectDir, ctx.Env.ESPOStorageDir()),
	}
	if cleaned := strings.TrimSpace(manifestPath); cleaned != "" {
		req.ManifestPath = cleaned
		return req
	}
	req.FilesBackup = strings.TrimSpace(filesBackup)
	return req
}

func (s Service) resolveDBPassword(req RestoreDBRequest) (string, error) {
	return s.env.ResolveDBPassword(envport.DBPasswordRequest{
		Container:    req.DBContainer,
		Name:         req.DBName,
		User:         req.DBUser,
		Password:     req.DBPassword,
		PasswordFile: req.DBPasswordFile,
	})
}

func (s Service) resolveDBRootPassword(req RestoreDBRequest) (string, error) {
	return s.env.ResolveDBRootPassword(envport.DBPasswordRequest{
		Container:    req.DBContainer,
		Name:         req.DBName,
		User:         "root",
		Password:     req.DBRootPassword,
		PasswordFile: req.DBRootPasswordFile,
	})
}
