package restore

import (
	backupapp "github.com/lazuale/espocrm-ops/internal/app/backup"
	operationapp "github.com/lazuale/espocrm-ops/internal/app/operation"
	appadapter "github.com/lazuale/espocrm-ops/internal/platform/appadapter"
)

func testOperationService() operationapp.Service {
	return operationapp.NewService(operationapp.Dependencies{
		Env:   appadapter.EnvLoader{},
		Files: appadapter.Files{},
		Locks: appadapter.Locks{},
	})
}

func testBackupService() backupapp.Service {
	return backupapp.NewService(backupapp.Dependencies{
		Operations: testOperationService(),
		Env:        appadapter.EnvLoader{},
		Runtime:    appadapter.Runtime{},
		Files:      appadapter.Files{},
		Store:      appadapter.BackupStore{},
	})
}

func testRestoreService() Service {
	return NewService(Dependencies{
		Operations: testOperationService(),
		Backup:     testBackupService(),
	})
}

func Execute(req ExecuteRequest) (ExecuteInfo, error)                  { return testRestoreService().Execute(req) }
func RestoreDB(req RestoreDBRequest) (DBRestorePlan, error)            { return testRestoreService().RestoreDB(req) }
func RestoreFiles(req RestoreFilesRequest) (FilesRestorePlan, error)   { return testRestoreService().RestoreFiles(req) }
func PlanDBRestore(req RestoreDBRequest) (DBRestorePlan, error)        { return testRestoreService().PlanDBRestore(req) }
func PlanFilesRestore(req RestoreFilesRequest) (FilesRestorePlan, error) { return testRestoreService().PlanFilesRestore(req) }
func PreflightDBRestore(req DBPreflightRequest) (string, error)        { return testRestoreService().PreflightDBRestore(req) }
func PreflightFilesRestore(req FilesPreflightRequest) (string, error)  { return testRestoreService().PreflightFilesRestore(req) }
