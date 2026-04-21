package migrate

import (
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

func testMigrateService() Service {
	return NewService(Dependencies{
		Operations: testOperationService(),
		Env:        appadapter.EnvLoader{},
		Runtime:    appadapter.Runtime{},
		Files:      appadapter.Files{},
		Locks:      appadapter.Locks{},
		Store:      appadapter.BackupStore{},
	})
}

func Execute(req ExecuteRequest) (ExecuteInfo, error) {
	return testMigrateService().Execute(req)
}
