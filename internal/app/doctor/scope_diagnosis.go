package doctor

import (
	"errors"
	"fmt"
	"strings"

	domainenv "github.com/lazuale/espocrm-ops/internal/domain/env"
	domainfailure "github.com/lazuale/espocrm-ops/internal/domain/failure"
)

func requestedScopes(target string) []string {
	if target == "all" {
		return []string{"prod", "dev"}
	}

	return []string{target}
}

func (s Service) diagnoseScope(report *Report, req Request, scope string, docker dockerState, pathMode PathCheckMode) (domainenv.OperationEnv, bool) {
	env, err := s.env.LoadOperationEnv(report.ProjectDir, scope, req.EnvFileOverride)
	if err != nil {
		report.fail(scope, "env_resolution", fmt.Sprintf("Could not resolve the %s env file", scope), err.Error(), envAction(err, report.ProjectDir, scope))
		return domainenv.OperationEnv{}, false
	}

	backupRoot := s.env.ResolveProjectPath(report.ProjectDir, env.BackupRoot())
	report.Scopes = append(report.Scopes, ScopeArtifact{
		Scope:      scope,
		EnvFile:    env.FilePath,
		BackupRoot: backupRoot,
	})
	report.ok(scope, "env_resolution", fmt.Sprintf("Loaded %s env file", scope), fmt.Sprintf("Using %s", env.FilePath))

	checkEnvContract(report, scope, env)

	minFreeMB, hasMinFree := parseInteger(env.Value("MIN_FREE_DISK_MB"))
	s.checkRuntimePath(report, scope, "db_storage_dir", "DB_STORAGE_DIR", s.env.ResolveProjectPath(report.ProjectDir, env.DBStorageDir()), minFreeMB, hasMinFree, pathMode)
	s.checkRuntimePath(report, scope, "espo_storage_dir", "ESPO_STORAGE_DIR", s.env.ResolveProjectPath(report.ProjectDir, env.ESPOStorageDir()), minFreeMB, hasMinFree, pathMode)
	s.checkRuntimePath(report, scope, "backup_root", "BACKUP_ROOT", backupRoot, minFreeMB, hasMinFree, pathMode)
	s.checkMaintenanceLock(report, scope, backupRoot)

	if docker.composeReady && docker.daemonReady {
		s.checkComposeConfig(report, scope, env.FilePath)
		s.checkRunningServices(report, scope, env.FilePath)
	}

	return env, true
}

func envAction(err error, projectDir, scope string) string {
	var failure domainfailure.Failure
	if errors.As(err, &failure) {
		switch failure.Kind {
		case domainfailure.KindValidation, domainfailure.KindManifest:
			return fmt.Sprintf("Fix or provide %s/.env.%s, or pass --env-file to point doctor at the correct file.", projectDir, scope)
		case domainfailure.KindIO:
			return "Check the env file path and permissions, then rerun doctor."
		}
	}

	if strings.TrimSpace(scope) != "" {
		return fmt.Sprintf("Check the env file for contour %s and rerun doctor.", scope)
	}

	return "Fix the env file contents and rerun doctor."
}
