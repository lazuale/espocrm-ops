package restore

import (
	"path/filepath"

	backupusecase "github.com/lazuale/espocrm-ops/internal/app/backup"
	maintenanceusecase "github.com/lazuale/espocrm-ops/internal/app/operation"
	domainfailure "github.com/lazuale/espocrm-ops/internal/domain/failure"
	domainruntime "github.com/lazuale/espocrm-ops/internal/domain/runtime"
)

func (s Service) buildSnapshotRequest(ctx maintenanceusecase.OperationContext, req ExecuteRequest) (snapshotBackupRequest, error) {
	prepared, err := s.backup.BuildPreparedRequest(ctx, backupusecase.PreparedOptions{
		ComposeFile: filepath.Clean(req.ComposeFile),
		SkipDB:      req.SkipDB,
		SkipFiles:   req.SkipFiles,
		NoStop:      req.NoStop,
		Now:         req.Now,
	})
	if err != nil {
		return snapshotBackupRequest{}, restoreFailure(domainfailure.KindValidation, "restore_failed", err)
	}

	return snapshotBackupRequest{
		TimeoutSeconds: domainruntime.DefaultReadinessTimeoutSeconds,
		LogWriter:      req.LogWriter,
		Backup:         prepared,
	}, nil
}
