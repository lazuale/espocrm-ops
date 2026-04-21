package migrate

import (
	operationapp "github.com/lazuale/espocrm-ops/internal/app/operation"
	lockport "github.com/lazuale/espocrm-ops/internal/app/ports/lockport"
	appadapter "github.com/lazuale/espocrm-ops/internal/platform/appadapter"
)

func testOperationService(locks lockport.Locks) operationapp.Service {
	if locks == nil {
		locks = appadapter.Locks{}
	}

	return operationapp.NewService(operationapp.Dependencies{
		Env:   appadapter.EnvLoader{},
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
		Env:        appadapter.EnvLoader{},
		Runtime:    appadapter.Runtime{},
		Files:      appadapter.Files{},
		Locks:      locks,
		Store:      appadapter.BackupStore{},
	})
}
