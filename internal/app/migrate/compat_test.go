package migrate

import (
	backupapp "github.com/lazuale/espocrm-ops/internal/app/backup"
	operationapp "github.com/lazuale/espocrm-ops/internal/app/operation"
	restoreapp "github.com/lazuale/espocrm-ops/internal/app/restore"
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

func testRestoreService() restoreapp.Service {
	return restoreapp.NewService(restoreapp.Dependencies{
		Operations: testOperationService(),
		Backup:     testBackupService(),
		Env:        appadapter.EnvLoader{},
		Runtime:    appadapter.Runtime{},
		Files:      appadapter.Files{},
		Locks:      appadapter.Locks{},
		Store:      appadapter.BackupStore{},
	})
}

func testMigrateService() Service {
	return NewService(Dependencies{
		Operations: testOperationService(),
		Restore:    testRestoreService(),
		Env:        appadapter.EnvLoader{},
		Runtime:    appadapter.Runtime{},
		Store:      appadapter.BackupStore{},
	})
}

func Execute(req ExecuteRequest) (ExecuteInfo, error) {
	return testMigrateService().Execute(req)
}
