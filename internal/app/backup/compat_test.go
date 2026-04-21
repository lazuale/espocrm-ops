package backup

import (
	backupverifyapp "github.com/lazuale/espocrm-ops/internal/app/backupverify"
	backupflow "github.com/lazuale/espocrm-ops/internal/app/internal/backupflow"
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

func testBackupFlow() backupflow.Service {
	return backupflow.NewService(backupflow.Dependencies{
		Env:     appadapter.EnvLoader{},
		Runtime: appadapter.Runtime{},
		Files:   appadapter.Files{},
		Store:   appadapter.BackupStore{},
	})
}

func testBackupVerifyService() backupverifyapp.Service {
	return backupverifyapp.NewService(backupverifyapp.Dependencies{
		Store: appadapter.BackupStore{},
	})
}

type PreparedRequest = backupflow.Request
type VerifyRequest = backupverifyapp.Request
type VerifyInfo = backupverifyapp.Report
type ManifestBuildRequest = manifestBuildRequest
type FinalizeRequest = finalizeRequest
type FinalizeInfo = finalizeInfo
type ManifestCandidate = backupstoreManifestCandidate

func Execute(req Request) (ExecuteInfo, error)                 { return testBackupService().Execute(req) }
func ExecutePrepared(req PreparedRequest) (ExecuteInfo, error) { return testBackupFlow().Execute(req) }
func Verify(req VerifyRequest) error {
	_, err := testBackupVerifyService().Diagnose(req)
	return err
}
func VerifyDetailed(req VerifyRequest) (VerifyInfo, error) {
	return testBackupVerifyService().Diagnose(req)
}
func LoadManifest(path string) (domainbackup.Manifest, error) {
	return appadapter.BackupStore{}.LoadManifest(path)
}
func LatestCompleteManifest(root string) (string, error) {
	report, err := testBackupVerifyService().Diagnose(backupverifyapp.Request{BackupRoot: root})
	if err != nil {
		return "", err
	}
	return report.ManifestPath, nil
}
func ManifestCandidates(root string) ([]ManifestCandidate, error) {
	candidates, err := appadapter.BackupStore{}.ManifestCandidates(root)
	if err != nil {
		return nil, err
	}
	out := make([]ManifestCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		out = append(out, backupstoreManifestCandidate(candidate))
	}
	return out, nil
}
func BuildManifest(req ManifestBuildRequest) (domainbackup.Manifest, error) {
	return testBackupService().buildManifest(req)
}
func FinalizeBackup(req FinalizeRequest) (FinalizeInfo, error) {
	return testBackupService().finalizeBackup(req)
}
func WriteManifest(path string, manifest domainbackup.Manifest) error {
	return appadapter.BackupStore{}.WriteManifest(path, manifest)
}
func WriteSHA256Sidecar(filePath, checksum, sidecarPath string) error {
	return appadapter.BackupStore{}.WriteSHA256Sidecar(filePath, checksum, sidecarPath)
}

type backupstoreManifestCandidate struct {
	Prefix       string
	Stamp        string
	ManifestPath string
}
