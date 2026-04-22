package backup

import (
	backupverifyapp "github.com/lazuale/espocrm-ops/internal/app/backupverify"
	operationapp "github.com/lazuale/espocrm-ops/internal/app/operation"
	appadapter "github.com/lazuale/espocrm-ops/internal/platform/appadapter"
	backupstoreadapter "github.com/lazuale/espocrm-ops/internal/platform/backupstoreadapter"
	envadapter "github.com/lazuale/espocrm-ops/internal/platform/envadapter"
	runtimeadapter "github.com/lazuale/espocrm-ops/internal/platform/runtimeadapter"
)

func testOperationService() operationapp.Service {
	return operationapp.NewService(operationapp.Dependencies{
		Env:   envadapter.EnvLoader{},
		Files: appadapter.Files{},
		Locks: appadapter.Locks{},
	})
}

func testBackupService() Service {
	return NewService(Dependencies{
		Operations: testOperationService(),
		Env:        envadapter.EnvLoader{},
		Runtime:    runtimeadapter.Runtime{},
		Files:      appadapter.Files{},
		Store:      backupstoreadapter.BackupStore{},
	})
}

func testBackupVerifyService() backupverifyapp.Service {
	return backupverifyapp.NewService(backupverifyapp.Dependencies{
		Store: backupstoreadapter.BackupStore{},
	})
}
