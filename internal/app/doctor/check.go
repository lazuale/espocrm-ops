package doctor

import (
	"path/filepath"
	"strings"

	domainenv "github.com/lazuale/espocrm-ops/internal/domain/env"
)

type Request struct {
	Scope           string
	ProjectDir      string
	ComposeFile     string
	EnvFileOverride string
	PathCheckMode   PathCheckMode
}

type Check struct {
	Scope   string
	Code    string
	Status  string
	Summary string
	Details string
	Action  string
}

type ScopeArtifact struct {
	Scope      string
	EnvFile    string
	BackupRoot string
}

type Report struct {
	TargetScope string
	ProjectDir  string
	ComposeFile string
	Checks      []Check
	Scopes      []ScopeArtifact
}

type dockerState struct {
	cliVersion     string
	serverVersion  string
	composeVersion string
	cliReady       bool
	daemonReady    bool
	composeReady   bool
}

func (s Service) Diagnose(req Request) (Report, error) {
	pathMode := normalizePathCheckMode(req.PathCheckMode)
	report := Report{
		TargetScope: strings.TrimSpace(req.Scope),
		ProjectDir:  filepath.Clean(req.ProjectDir),
		ComposeFile: filepath.Clean(req.ComposeFile),
	}

	s.checkComposeFile(&report)
	s.checkSharedOperationLock(&report)
	docker := s.checkDocker(&report)

	loaded := map[string]domainenv.OperationEnv{}
	for _, scope := range requestedScopes(report.TargetScope) {
		env, ok := s.diagnoseScope(&report, req, scope, docker, pathMode)
		if ok {
			loaded[scope] = env
		}
	}

	if report.TargetScope == "all" {
		prodEnv, prodOK := loaded["prod"]
		devEnv, devOK := loaded["dev"]
		if prodOK && devOK {
			s.checkCrossScopeIsolation(&report, report.ProjectDir, prodEnv, devEnv)
			checkCrossScopeCompatibility(&report, prodEnv, devEnv)
		}
	}

	return report, nil
}

func (s Service) checkComposeFile(report *Report) {
	if _, err := s.files.EnsureNonEmptyFile("compose file", report.ComposeFile); err != nil {
		report.fail("", "compose_file", "Compose file is not ready", err.Error(), "Set --compose-file to a readable compose.yaml path before running doctor.")
		return
	}

	report.ok("", "compose_file", "Compose file is ready", report.ComposeFile)
}
