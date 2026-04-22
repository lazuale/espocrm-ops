package doctor

import (
	appadapter "github.com/lazuale/espocrm-ops/internal/platform/appadapter"
	envadapter "github.com/lazuale/espocrm-ops/internal/platform/envadapter"
	runtimeadapter "github.com/lazuale/espocrm-ops/internal/platform/runtimeadapter"
)

func testDoctorService() Service {
	return NewService(Dependencies{
		Env:     envadapter.EnvLoader{},
		Files:   appadapter.Files{},
		Locks:   appadapter.Locks{},
		Runtime: runtimeadapter.Runtime{},
	})
}
