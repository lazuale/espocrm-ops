package backupverify

import (
	backupstoreport "github.com/lazuale/espocrm-ops/internal/app/ports/backupstoreport"
)

type Dependencies struct {
	Store backupstoreport.Store
}

type Service struct {
	store backupstoreport.Store
}

type Request struct {
	ManifestPath string
	BackupRoot   string
}

type Report struct {
	ManifestPath string
	Scope        string
	CreatedAt    string
	DBBackupPath string
	FilesPath    string
}

func NewService(deps Dependencies) Service {
	return Service{
		store: deps.Store,
	}
}
