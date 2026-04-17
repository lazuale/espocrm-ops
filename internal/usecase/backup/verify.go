package backup

import "github.com/lazuale/espocrm-ops/internal/platform/backupstore"

type VerifyRequest struct {
	ManifestPath string
}

type VerifyInfo struct {
	ManifestPath string
	Scope        string
	CreatedAt    string
	DBBackupPath string
	FilesPath    string
}

func Verify(req VerifyRequest) error {
	return classifyStoreError(backupstore.VerifyManifest(req.ManifestPath))
}

func VerifyDetailed(req VerifyRequest) (VerifyInfo, error) {
	info, err := backupstore.VerifyManifestDetailed(req.ManifestPath)
	if err != nil {
		return VerifyInfo{}, classifyStoreError(err)
	}

	return VerifyInfo{
		ManifestPath: info.ManifestPath,
		Scope:        info.Scope,
		CreatedAt:    info.CreatedAt,
		DBBackupPath: info.DBBackupPath,
		FilesPath:    info.FilesPath,
	}, nil
}
