package operation

import appadapter "github.com/lazuale/espocrm-ops/internal/platform/appadapter"

func testService() Service {
	return NewService(Dependencies{
		Env:   appadapter.EnvLoader{},
		Files: appadapter.Files{},
		Locks: appadapter.Locks{},
	})
}
