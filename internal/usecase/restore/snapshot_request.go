package restore

import (
	"fmt"
	"path/filepath"
	"strings"

	platformconfig "github.com/lazuale/espocrm-ops/internal/platform/config"
	backupusecase "github.com/lazuale/espocrm-ops/internal/usecase/backup"
	maintenanceusecase "github.com/lazuale/espocrm-ops/internal/usecase/maintenance"
)

func buildSnapshotRequest(ctx maintenanceusecase.OperationContext, req ExecuteRequest) snapshotBackupRequest {
	return snapshotBackupRequest{
		TimeoutSeconds: defaultRestoreReadinessTimeoutSeconds,
		LogWriter:      req.LogWriter,
		Backup: backupusecase.ExecuteRequest{
			Scope:          ctx.Scope,
			ProjectDir:     ctx.ProjectDir,
			ComposeFile:    filepath.Clean(req.ComposeFile),
			EnvFile:        ctx.Env.FilePath,
			BackupRoot:     ctx.BackupRoot,
			StorageDir:     platformconfig.ResolveProjectPath(ctx.ProjectDir, ctx.Env.ESPOStorageDir()),
			NamePrefix:     resolvedBackupNamePrefix(ctx.Env),
			RetentionDays:  resolvedRetentionDays(ctx.Env),
			ComposeProject: ctx.ComposeProject,
			DBUser:         strings.TrimSpace(ctx.Env.Value("DB_USER")),
			DBPassword:     ctx.Env.Value("DB_PASSWORD"),
			DBName:         strings.TrimSpace(ctx.Env.Value("DB_NAME")),
			EspoCRMImage:   strings.TrimSpace(ctx.Env.Value("ESPOCRM_IMAGE")),
			MariaDBTag:     strings.TrimSpace(ctx.Env.Value("MARIADB_TAG")),
			SkipDB:         req.SkipDB,
			SkipFiles:      req.SkipFiles,
			NoStop:         req.NoStop,
			LogWriter:      req.LogWriter,
			ErrWriter:      req.LogWriter,
		},
	}
}

func resolvedBackupNamePrefix(env platformconfig.OperationEnv) string {
	if value := strings.TrimSpace(env.Value("BACKUP_NAME_PREFIX")); value != "" {
		return value
	}
	return strings.TrimSpace(env.ComposeProject())
}

func resolvedRetentionDays(env platformconfig.OperationEnv) int {
	value := strings.TrimSpace(env.Value("BACKUP_RETENTION_DAYS"))
	if value == "" {
		return 7
	}
	var days int
	if _, err := fmt.Sscanf(value, "%d", &days); err != nil || days <= 0 {
		return 7
	}
	return days
}
