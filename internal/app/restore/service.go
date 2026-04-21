package restore

import (
	backupapp "github.com/lazuale/espocrm-ops/internal/app/backup"
	operationapp "github.com/lazuale/espocrm-ops/internal/app/operation"
)

type Dependencies struct {
	Operations operationapp.Service
	Backup     backupapp.Service
}

type Service struct {
	operations operationapp.Service
	backup     backupapp.Service
}

func NewService(deps Dependencies) Service {
	return Service{
		operations: deps.Operations,
		backup:     deps.Backup,
	}
}
