package restore

import (
	restoreflow "github.com/lazuale/espocrm-ops/internal/app/internal/restoreflow"
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

func testRestoreService() Service {
	return NewService(Dependencies{
		Operations: testOperationService(),
		Env:        appadapter.EnvLoader{},
		Runtime:    appadapter.Runtime{},
		Files:      appadapter.Files{},
		Locks:      appadapter.Locks{},
		Store:      appadapter.BackupStore{},
	})
}

type RestoreDBRequest = restoreflow.DBRequest
type RestoreFilesRequest = restoreflow.FilesRequest
type FilesPreflightRequest = restoreflow.FilesPreflightRequest
type DBPreflightRequest = restoreflow.DBPreflightRequest
type RestorePlanCheck = restoreflow.PlanCheck
type RestorePlan = restoreflow.Plan
type DBRestorePlan = restoreflow.DBPlan
type FilesRestorePlan = restoreflow.FilesPlan

const (
	RestoreSourceManifest     = restoreflow.RestoreSourceManifest
	RestoreSourceDirectBackup = restoreflow.RestoreSourceDirectBackup
	RestoreCheckPassed        = restoreflow.RestoreCheckPassed
	RestoreCheckPending       = restoreflow.RestoreCheckPending
)

func testRestoreFlow() restoreflow.Service {
	return restoreflow.NewService(restoreflow.Dependencies{
		Env:     appadapter.EnvLoader{},
		Runtime: appadapter.Runtime{},
		Files:   appadapter.Files{},
		Locks:   appadapter.Locks{},
		Store:   appadapter.BackupStore{},
	})
}

func Execute(req ExecuteRequest) (ExecuteInfo, error) { return testRestoreService().Execute(req) }
func RestoreDB(req RestoreDBRequest) (DBRestorePlan, error) {
	return testRestoreFlow().RestoreDB(req)
}
func RestoreFiles(req RestoreFilesRequest) (FilesRestorePlan, error) {
	return testRestoreFlow().RestoreFiles(req)
}
func PlanDBRestore(req RestoreDBRequest) (DBRestorePlan, error) {
	return testRestoreFlow().PlanDB(req)
}
func PlanFilesRestore(req RestoreFilesRequest) (FilesRestorePlan, error) {
	return testRestoreFlow().PlanFiles(req)
}
func PreflightDBRestore(req DBPreflightRequest) (string, error) {
	return testRestoreFlow().PreflightDB(req)
}
func PreflightFilesRestore(req FilesPreflightRequest) (string, error) {
	return testRestoreFlow().PreflightFiles(req)
}
