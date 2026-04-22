package restore

import (
	restoreflow "github.com/lazuale/espocrm-ops/internal/app/internal/restoreflow"
	appadapter "github.com/lazuale/espocrm-ops/internal/platform/appadapter"
	backupstoreadapter "github.com/lazuale/espocrm-ops/internal/platform/backupstoreadapter"
	envadapter "github.com/lazuale/espocrm-ops/internal/platform/envadapter"
	runtimeadapter "github.com/lazuale/espocrm-ops/internal/platform/runtimeadapter"
)

func testRestoreFlow() restoreflow.Service {
	return restoreflow.NewService(restoreflow.Dependencies{
		Env:     envadapter.EnvLoader{},
		Runtime: runtimeadapter.Runtime{},
		Files:   appadapter.Files{},
		Locks:   appadapter.Locks{},
		Store:   backupstoreadapter.BackupStore{},
	})
}
