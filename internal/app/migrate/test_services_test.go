package migrate

import (
	operationapp "github.com/lazuale/espocrm-ops/internal/app/operation"
	lockport "github.com/lazuale/espocrm-ops/internal/app/ports/lockport"
	appadapter "github.com/lazuale/espocrm-ops/internal/platform/appadapter"
	backupstoreadapter "github.com/lazuale/espocrm-ops/internal/platform/backupstoreadapter"
	envadapter "github.com/lazuale/espocrm-ops/internal/platform/envadapter"
	runtimeadapter "github.com/lazuale/espocrm-ops/internal/platform/runtimeadapter"
)

func testOperationService(locks lockport.Locks) operationapp.Service {
	if locks == nil {
		locks = appadapter.Locks{}
	}

	return operationapp.NewService(operationapp.Dependencies{
		Env:   envadapter.EnvLoader{},
		Files: appadapter.Files{},
		Locks: locks,
	})
}

func testMigrateService(locks lockport.Locks) Service {
	if locks == nil {
		locks = appadapter.Locks{}
	}

	return NewService(Dependencies{
		Operations: testOperationService(locks),
		Env:        envadapter.EnvLoader{},
		Runtime:    runtimeadapter.Runtime{},
		Files:      appadapter.Files{},
		Locks:      locks,
		Store:      backupstoreadapter.BackupStore{},
	})
}
