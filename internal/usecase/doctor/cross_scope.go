package doctor

import (
	"fmt"
	"strings"

	platformconfig "github.com/lazuale/espocrm-ops/internal/platform/config"
)

func checkCrossScopeIsolation(report *Report, projectDir string, prodEnv, devEnv platformconfig.OperationEnv) {
	problems := []string{}

	if prodEnv.ComposeProject() == devEnv.ComposeProject() {
		problems = append(problems, fmt.Sprintf("COMPOSE_PROJECT_NAME matches in dev and prod: %s", prodEnv.ComposeProject()))
	}
	if prodEnv.Value("APP_PORT") == devEnv.Value("APP_PORT") {
		problems = append(problems, fmt.Sprintf("APP_PORT matches in dev and prod: %s", prodEnv.Value("APP_PORT")))
	}
	if prodEnv.Value("WS_PORT") == devEnv.Value("WS_PORT") {
		problems = append(problems, fmt.Sprintf("WS_PORT matches in dev and prod: %s", prodEnv.Value("WS_PORT")))
	}
	if sameResolvedPath(projectDir, prodEnv.DBStorageDir(), devEnv.DBStorageDir()) {
		problems = append(problems, fmt.Sprintf("DB_STORAGE_DIR resolves to the same path in dev and prod: %s", platformconfig.ResolveProjectPath(projectDir, prodEnv.DBStorageDir())))
	}
	if sameResolvedPath(projectDir, prodEnv.ESPOStorageDir(), devEnv.ESPOStorageDir()) {
		problems = append(problems, fmt.Sprintf("ESPO_STORAGE_DIR resolves to the same path in dev and prod: %s", platformconfig.ResolveProjectPath(projectDir, prodEnv.ESPOStorageDir())))
	}
	if sameResolvedPath(projectDir, prodEnv.BackupRoot(), devEnv.BackupRoot()) {
		problems = append(problems, fmt.Sprintf("BACKUP_ROOT resolves to the same path in dev and prod: %s", platformconfig.ResolveProjectPath(projectDir, prodEnv.BackupRoot())))
	}

	if len(problems) != 0 {
		report.fail("cross", "cross_scope_isolation", "Dev and prod are not isolated from each other", strings.Join(problems, "; "), "Separate the conflicting ports, project names, and storage paths before running operations against either contour.")
		return
	}

	report.ok("cross", "cross_scope_isolation", "Dev and prod keep isolated ports and storage", "")
}

func checkCrossScopeCompatibility(report *Report, prodEnv, devEnv platformconfig.OperationEnv) {
	problems := []string{}

	for _, mismatch := range platformconfig.MigrationCompatibilityMismatches(prodEnv, devEnv) {
		problems = append(problems, fmt.Sprintf("%s differs: prod=%s dev=%s", mismatch.Name, mismatch.LeftValue, mismatch.RightValue))
	}

	if len(problems) != 0 {
		report.fail("cross", "cross_scope_compatibility", "Dev and prod do not satisfy the migration compatibility contract", strings.Join(problems, "; "), "Align the shared runtime compatibility settings before relying on cross-contour migration or restore flows.")
		return
	}

	report.ok("cross", "cross_scope_compatibility", "Dev and prod satisfy the migration compatibility contract", "")
}

func sameResolvedPath(projectDir, left, right string) bool {
	return platformconfig.ResolveProjectPath(projectDir, left) == platformconfig.ResolveProjectPath(projectDir, right)
}
