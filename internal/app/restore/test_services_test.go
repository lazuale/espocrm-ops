package restore

import (
	restoreflow "github.com/lazuale/espocrm-ops/internal/app/internal/restoreflow"
	appadapter "github.com/lazuale/espocrm-ops/internal/platform/appadapter"
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
