package backup

import (
	backupverifyapp "github.com/lazuale/espocrm-ops/internal/app/backupverify"
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

func testBackupService() Service {
	return NewService(Dependencies{
		Operations: testOperationService(),
		Env:        appadapter.EnvLoader{},
		Runtime:    appadapter.Runtime{},
		Files:      appadapter.Files{},
		Store:      appadapter.BackupStore{},
	})
}

func testBackupVerifyService() backupverifyapp.Service {
	return backupverifyapp.NewService(backupverifyapp.Dependencies{
		Store: appadapter.BackupStore{},
	})
}
