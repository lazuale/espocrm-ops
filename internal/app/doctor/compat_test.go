package doctor

import appadapter "github.com/lazuale/espocrm-ops/internal/platform/appadapter"

func testDoctorService() Service {
	return NewService(Dependencies{
		Env:     appadapter.EnvLoader{},
		Files:   appadapter.Files{},
		Locks:   appadapter.Locks{},
		Runtime: appadapter.Runtime{},
	})
}

func Diagnose(req Request) (Report, error) {
	return testDoctorService().Diagnose(req)
}
