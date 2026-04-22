package backupflow

import (
	"fmt"
	"path/filepath"
	"strings"

	operationapp "github.com/lazuale/espocrm-ops/internal/app/operation"
	domainenv "github.com/lazuale/espocrm-ops/internal/domain/env"
	domainfailure "github.com/lazuale/espocrm-ops/internal/domain/failure"
)

func (s Service) BuildRequest(ctx operationapp.OperationContext, opts Options) (Request, error) {
	retentionDays, err := domainenv.BackupRetentionDays(ctx.Env)
	if err != nil {
		return Request{}, domainfailure.Failure{
			Kind: domainfailure.KindValidation,
			Code: "backup_failed",
			Err:  err,
		}
	}

	runtimeContract, err := ctx.Env.RuntimeContract()
	if err != nil {
		return Request{}, domainfailure.Failure{
			Kind: domainfailure.KindValidation,
			Code: "backup_failed",
			Err:  err,
		}
	}

	prepared := Request{
		Scope:          ctx.Scope,
		ProjectDir:     ctx.ProjectDir,
		ComposeFile:    filepath.Clean(opts.ComposeFile),
		EnvFile:        ctx.Env.FilePath,
		BackupRoot:     ctx.BackupRoot,
		StorageDir:     s.env.ResolveProjectPath(ctx.ProjectDir, ctx.Env.ESPOStorageDir()),
		NamePrefix:     domainenv.BackupNamePrefix(ctx.Env),
		RetentionDays:  retentionDays,
		ComposeProject: ctx.ComposeProject,
		DBUser:         strings.TrimSpace(ctx.Env.Value("DB_USER")),
		DBPassword:     ctx.Env.Value("DB_PASSWORD"),
		DBName:         strings.TrimSpace(ctx.Env.Value("DB_NAME")),
		EspoCRMImage:   strings.TrimSpace(ctx.Env.Value("ESPOCRM_IMAGE")),
		HelperImage:    runtimeContract.HelperImage,
		MariaDBTag:     strings.TrimSpace(ctx.Env.Value("MARIADB_TAG")),
		SkipDB:         opts.SkipDB,
		SkipFiles:      opts.SkipFiles,
		NoStop:         opts.NoStop,
		Now:            opts.Now,
	}

	if opts.SkipDB {
		return prepared, nil
	}

	for _, field := range []struct {
		name  string
		value string
	}{
		{name: "DB_USER", value: prepared.DBUser},
		{name: "DB_PASSWORD", value: prepared.DBPassword},
		{name: "DB_NAME", value: prepared.DBName},
	} {
		if strings.TrimSpace(field.value) == "" {
			return Request{}, domainfailure.Failure{
				Kind: domainfailure.KindValidation,
				Code: "backup_failed",
				Err:  fmt.Errorf("%s is not set in %s", field.name, ctx.Env.FilePath),
			}
		}
	}

	return prepared, nil
}
