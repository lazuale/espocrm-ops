package operation

import (
	appadapter "github.com/lazuale/espocrm-ops/internal/platform/appadapter"
	envadapter "github.com/lazuale/espocrm-ops/internal/platform/envadapter"
)

func testService() Service {
	return NewService(Dependencies{
		Env:   envadapter.EnvLoader{},
		Files: appadapter.Files{},
		Locks: appadapter.Locks{},
	})
}
