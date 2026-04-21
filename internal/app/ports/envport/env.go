package envport

import domainenv "github.com/lazuale/espocrm-ops/internal/domain/env"

type DBPasswordRequest struct {
	Container    string
	Name         string
	User         string
	Password     string
	PasswordFile string
}

type Loader interface {
	LoadOperationEnv(projectDir, scope, overridePath string) (domainenv.OperationEnv, error)
	ResolveProjectPath(projectDir, value string) string
	ResolveDBPassword(req DBPasswordRequest) (string, error)
	ResolveDBRootPassword(req DBPasswordRequest) (string, error)
}
