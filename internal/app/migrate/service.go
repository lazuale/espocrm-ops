package migrate

import (
	backupapp "github.com/lazuale/espocrm-ops/internal/app/backup"
	operationapp "github.com/lazuale/espocrm-ops/internal/app/operation"
	restoreapp "github.com/lazuale/espocrm-ops/internal/app/restore"
)

type Dependencies struct {
	Operations operationapp.Service
	Restore    restoreapp.Service
	Backup     backupapp.Service
}

type Service struct {
	operations operationapp.Service
	restore    restoreapp.Service
	backup     backupapp.Service
}

func NewService(deps Dependencies) Service {
	return Service{
		operations: deps.Operations,
		restore:    deps.Restore,
		backup:     deps.Backup,
	}
}
