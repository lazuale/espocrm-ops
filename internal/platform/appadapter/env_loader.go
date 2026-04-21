package appadapter

import (
	envport "github.com/lazuale/espocrm-ops/internal/app/ports/envport"
	domainenv "github.com/lazuale/espocrm-ops/internal/domain/env"
	domainfailure "github.com/lazuale/espocrm-ops/internal/domain/failure"
	platformconfig "github.com/lazuale/espocrm-ops/internal/platform/config"
)

type EnvLoader struct{}

func (EnvLoader) LoadOperationEnv(projectDir, scope, overridePath string) (env domainenv.OperationEnv, err error) {
	env, err = platformconfig.LoadOperationEnv(projectDir, scope, overridePath)
	if err == nil {
		return env, nil
	}

	switch err.(type) {
	case platformconfig.MissingEnvFileError, platformconfig.InvalidEnvFileError, platformconfig.EnvParseError, platformconfig.MissingEnvValueError, platformconfig.UnsupportedContourError:
		return env, domainfailure.Failure{Kind: domainfailure.KindValidation, Code: "operation_execute_failed", Err: err}
	default:
		return env, domainfailure.Failure{Kind: domainfailure.KindIO, Code: "operation_execute_failed", Err: err}
	}
}

func (EnvLoader) ResolveProjectPath(projectDir, value string) string {
	return platformconfig.ResolveProjectPath(projectDir, value)
}

func (EnvLoader) ResolveDBPassword(req envport.DBPasswordRequest) (string, error) {
	return platformconfig.ResolveDBPassword(platformconfig.DBConfig{
		Container:    req.Container,
		Name:         req.Name,
		User:         req.User,
		Password:     req.Password,
		PasswordFile: req.PasswordFile,
	})
}

func (EnvLoader) ResolveDBRootPassword(req envport.DBPasswordRequest) (string, error) {
	return platformconfig.ResolveDBRootPassword(platformconfig.DBConfig{
		Container:    req.Container,
		Name:         req.Name,
		User:         req.User,
		Password:     req.Password,
		PasswordFile: req.PasswordFile,
	})
}
