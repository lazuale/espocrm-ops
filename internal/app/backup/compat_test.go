package backup

import (
	operationapp "github.com/lazuale/espocrm-ops/internal/app/operation"
	domainbackup "github.com/lazuale/espocrm-ops/internal/domain/backup"
	appadapter "github.com/lazuale/espocrm-ops/internal/platform/appadapter"
)

func testOperationService() operationapp.Service {
	return operationapp.NewService(operationapp.Dependencies{
		Env:   appadapter.EnvLoader{},
		Files: appadapter.Files{},
		Locks: appadapter.Locks{},
	})
}

func testBackupService() Service {
	return NewService(Dependencies{
		Operations: testOperationService(),
		Env:        appadapter.EnvLoader{},
		Runtime:    appadapter.Runtime{},
		Files:      appadapter.Files{},
		Store:      appadapter.BackupStore{},
	})
}

func Execute(req Request) (ExecuteInfo, error)                 { return testBackupService().Execute(req) }
func ExecutePrepared(req PreparedRequest) (ExecuteInfo, error) { return testBackupService().ExecutePrepared(req) }
func Verify(req VerifyRequest) error                           { return testBackupService().Verify(req) }
func VerifyDetailed(req VerifyRequest) (VerifyInfo, error)     { return testBackupService().VerifyDetailed(req) }
func LoadManifest(path string) (domainbackup.Manifest, error)  { return testBackupService().LoadManifest(path) }
func LatestCompleteManifest(root string) (string, error)       { return testBackupService().LatestCompleteManifest(root) }
func ManifestCandidates(root string) ([]ManifestCandidate, error) { return testBackupService().ManifestCandidates(root) }
func BuildManifest(req ManifestBuildRequest) (domainbackup.Manifest, error) {
	return testBackupService().BuildManifest(req)
}
func FinalizeBackup(req FinalizeRequest) (FinalizeInfo, error) { return testBackupService().FinalizeBackup(req) }
func WriteManifest(path string, manifest domainbackup.Manifest) error {
	return appadapter.BackupStore{}.WriteManifest(path, manifest)
}
func WriteSHA256Sidecar(filePath, checksum, sidecarPath string) error {
	return appadapter.BackupStore{}.WriteSHA256Sidecar(filePath, checksum, sidecarPath)
}
