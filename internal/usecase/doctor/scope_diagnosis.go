package doctor

import (
	"fmt"

	platformconfig "github.com/lazuale/espocrm-ops/internal/platform/config"
	platformdocker "github.com/lazuale/espocrm-ops/internal/platform/docker"
)

func requestedScopes(target string) []string {
	if target == "all" {
		return []string{"prod", "dev"}
	}

	return []string{target}
}

func diagnoseScope(report *Report, req Request, scope string, docker dockerState, pathMode PathCheckMode) (platformconfig.OperationEnv, bool) {
	env, err := platformconfig.LoadOperationEnv(report.ProjectDir, scope, req.EnvFileOverride)
	if err != nil {
		report.fail(scope, "env_resolution", fmt.Sprintf("Could not resolve the %s env file", scope), err.Error(), envAction(err, report.ProjectDir, scope))
		return platformconfig.OperationEnv{}, false
	}

	backupRoot := platformconfig.ResolveProjectPath(report.ProjectDir, env.BackupRoot())
	report.Scopes = append(report.Scopes, ScopeArtifact{
		Scope:      scope,
		EnvFile:    env.FilePath,
		BackupRoot: backupRoot,
	})
	report.ok(scope, "env_resolution", fmt.Sprintf("Loaded %s env file", scope), fmt.Sprintf("Using %s", env.FilePath))

	checkEnvContract(report, scope, env)

	minFreeMB, hasMinFree := parseInteger(env.Value("MIN_FREE_DISK_MB"))
	checkRuntimePath(report, scope, "db_storage_dir", "DB_STORAGE_DIR", platformconfig.ResolveProjectPath(report.ProjectDir, env.DBStorageDir()), minFreeMB, hasMinFree, pathMode)
	checkRuntimePath(report, scope, "espo_storage_dir", "ESPO_STORAGE_DIR", platformconfig.ResolveProjectPath(report.ProjectDir, env.ESPOStorageDir()), minFreeMB, hasMinFree, pathMode)
	checkRuntimePath(report, scope, "backup_root", "BACKUP_ROOT", backupRoot, minFreeMB, hasMinFree, pathMode)
	checkMaintenanceLock(report, scope, backupRoot)

	if docker.composeReady && docker.daemonReady {
		cfg := platformdocker.ComposeConfig{
			ProjectDir:  report.ProjectDir,
			ComposeFile: report.ComposeFile,
			EnvFile:     env.FilePath,
		}
		checkComposeConfig(report, scope, cfg)
		checkRunningServices(report, scope, cfg)
	}

	return env, true
}

func envAction(err error, projectDir, scope string) string {
	switch err.(type) {
	case platformconfig.MissingEnvFileError:
		return fmt.Sprintf("Create %s/.env.%s from env/.env.%s.example or pass --env-file to point doctor at the correct file.", projectDir, scope, scope)
	case platformconfig.InvalidEnvFileError, platformconfig.EnvParseError, platformconfig.MissingEnvValueError:
		return "Fix the env file contents and rerun doctor."
	default:
		return "Check the env file path and permissions, then rerun doctor."
	}
}
