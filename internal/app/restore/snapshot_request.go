package restore

import (
	"fmt"
	"path/filepath"
	"strings"

	backupusecase "github.com/lazuale/espocrm-ops/internal/app/backup"
	maintenanceusecase "github.com/lazuale/espocrm-ops/internal/app/operation"
	domainenv "github.com/lazuale/espocrm-ops/internal/domain/env"
	domainfailure "github.com/lazuale/espocrm-ops/internal/domain/failure"
	domainruntime "github.com/lazuale/espocrm-ops/internal/domain/runtime"
	platformconfig "github.com/lazuale/espocrm-ops/internal/platform/config"
)

func buildSnapshotRequest(ctx maintenanceusecase.OperationContext, req ExecuteRequest) (snapshotBackupRequest, error) {
	retentionDays, err := domainenv.BackupRetentionDays(ctx.Env)
	if err != nil {
		return snapshotBackupRequest{}, domainfailure.Failure{
			Kind: domainfailure.KindValidation,
			Code: "restore_failed",
			Err:  fmt.Errorf("resolve backup retention days: %w", err),
		}
	}

	return snapshotBackupRequest{
		TimeoutSeconds: domainruntime.DefaultReadinessTimeoutSeconds,
		LogWriter:      req.LogWriter,
		Backup: backupusecase.PreparedRequest{
			Scope:          ctx.Scope,
			ProjectDir:     ctx.ProjectDir,
			ComposeFile:    filepath.Clean(req.ComposeFile),
			EnvFile:        ctx.Env.FilePath,
			BackupRoot:     ctx.BackupRoot,
			StorageDir:     platformconfig.ResolveProjectPath(ctx.ProjectDir, ctx.Env.ESPOStorageDir()),
			NamePrefix:     domainenv.BackupNamePrefix(ctx.Env),
			RetentionDays:  retentionDays,
			ComposeProject: ctx.ComposeProject,
			DBUser:         strings.TrimSpace(ctx.Env.Value("DB_USER")),
			DBPassword:     ctx.Env.Value("DB_PASSWORD"),
			DBName:         strings.TrimSpace(ctx.Env.Value("DB_NAME")),
			EspoCRMImage:   strings.TrimSpace(ctx.Env.Value("ESPOCRM_IMAGE")),
			MariaDBTag:     strings.TrimSpace(ctx.Env.Value("MARIADB_TAG")),
			SkipDB:         req.SkipDB,
			SkipFiles:      req.SkipFiles,
			NoStop:         req.NoStop,
			Now:            req.Now,
		},
	}, nil
}
