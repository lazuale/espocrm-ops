package restore

import (
	"fmt"
	"io"

	backupflow "github.com/lazuale/espocrm-ops/internal/app/internal/backupflow"
	runtimeport "github.com/lazuale/espocrm-ops/internal/app/ports/runtimeport"
	domainfailure "github.com/lazuale/espocrm-ops/internal/domain/failure"
)

type snapshotBackupRequest struct {
	TimeoutSeconds int
	LogWriter      io.Writer
	Backup         backupflow.Request
}

type snapshotBackupInfo struct {
	TimeoutSeconds         int
	StartedDBTemporarily   bool
	CreatedAt              string
	ConsistentSnapshot     bool
	AppServicesWereRunning bool
	ManifestTXTPath        string
	ManifestJSONPath       string
	DBBackupPath           string
	FilesBackupPath        string
	DBSidecarPath          string
	FilesSidecarPath       string
}

func (s Service) applySnapshotBackup(req snapshotBackupRequest) (snapshotBackupInfo, error) {
	info := snapshotBackupInfo{
		TimeoutSeconds: req.TimeoutSeconds,
	}

	target := runtimeport.Target{
		ProjectDir:  req.Backup.ProjectDir,
		ComposeFile: req.Backup.ComposeFile,
		EnvFile:     req.Backup.EnvFile,
	}

	state, err := s.runtime.ServiceState(target, "db")
	if err != nil {
		return info, executeFailure{Kind: domainfailure.KindExternal, Err: err}
	}

	if state.Status != "running" && state.Status != "healthy" {
		info.StartedDBTemporarily = true
		if req.LogWriter != nil {
			if _, err := fmt.Fprintln(req.LogWriter, "[info] The DB container was not running, starting db temporarily for the emergency recovery point"); err != nil {
				return info, executeFailure{Kind: domainfailure.KindIO, Err: err}
			}
		}
		if err := s.runtime.Up(target, "db"); err != nil {
			return info, executeFailure{Kind: domainfailure.KindExternal, Err: err}
		}
		if err := s.runtime.WaitForServicesReady(target, req.TimeoutSeconds, "db"); err != nil {
			return info, executeFailure{Kind: domainfailure.KindExternal, Err: err}
		}
	}

	backupInfo, err := s.backupFlow.Execute(req.Backup)
	if err != nil {
		return info, err
	}

	info.CreatedAt = backupInfo.CreatedAt
	info.ConsistentSnapshot = backupInfo.ConsistentSnapshot
	info.AppServicesWereRunning = backupInfo.AppServicesWereRunning
	info.ManifestTXTPath = backupInfo.ManifestTXTPath
	info.ManifestJSONPath = backupInfo.ManifestJSONPath
	info.DBBackupPath = backupInfo.DBBackupPath
	info.FilesBackupPath = backupInfo.FilesBackupPath
	info.DBSidecarPath = backupInfo.DBSidecarPath
	info.FilesSidecarPath = backupInfo.FilesSidecarPath

	return info, nil
}
