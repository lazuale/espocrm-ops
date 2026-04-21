package restore

import (
	"fmt"
	"path/filepath"
	"strings"

	domainfailure "github.com/lazuale/espocrm-ops/internal/domain/failure"
)

const (
	RestoreSourceManifest     = "manifest"
	RestoreSourceDirectBackup = "direct_backup"

	RestoreCheckPassed  = "passed"
	RestoreCheckPending = "pending"
)

type RestorePlanCheck struct {
	Name    string
	Status  string
	Details string
}

type RestorePlan struct {
	SourceKind  string
	SourcePath  string
	Destructive bool
	Changes     []string
	NonChanges  []string
	Checks      []RestorePlanCheck
	NextStep    string
}

type DBRestorePlan struct {
	Plan RestorePlan
}

type FilesRestorePlan struct {
	Plan RestorePlan
}

func buildDBRestorePlan(req RestoreDBRequest, dbPath string, rootPasswordCheck RestorePlanCheck) DBRestorePlan {
	checks := []RestorePlanCheck{
		{
			Name:    "db_password_source",
			Status:  RestoreCheckPassed,
			Details: "database password source resolved",
		},
		{
			Name:    "backup_source",
			Status:  RestoreCheckPassed,
			Details: fmt.Sprintf("validated db backup at %s", dbPath),
		},
		{
			Name:    "docker",
			Status:  RestoreCheckPassed,
			Details: "docker daemon is available",
		},
		{
			Name:    "db_container",
			Status:  RestoreCheckPassed,
			Details: fmt.Sprintf("container %s is running", req.DBContainer),
		},
		{
			Name:    "restore_lock",
			Status:  RestoreCheckPassed,
			Details: "restore-db lock is currently acquirable",
		},
		rootPasswordCheck,
	}

	nextStep := "run status and application checks after the database restore"
	if req.DryRun {
		nextStep = "rerun without --dry-run to execute the database restore"
		if rootPasswordCheck.Status == RestoreCheckPending {
			nextStep = "provide database root password source and rerun without --dry-run to execute the database restore"
		}
	}

	return DBRestorePlan{
		Plan: RestorePlan{
			SourceKind:  restoreSourceKind(req.ManifestPath),
			SourcePath:  dbPath,
			Destructive: true,
			Changes: []string{
				fmt.Sprintf("reset database %s in container %s", req.DBName, req.DBContainer),
				fmt.Sprintf("import backup archive from %s", dbPath),
			},
			NonChanges: []string{
				"does not modify files storage",
				"does not manage contour stop/start orchestration",
			},
			Checks:   checks,
			NextStep: nextStep,
		},
	}
}

func buildFilesRestorePlan(req RestoreFilesRequest, filesPath string) FilesRestorePlan {
	parentDir := filepath.Dir(req.TargetDir)
	nextStep := "run status and application checks after the files restore"
	if req.DryRun {
		nextStep = "rerun without --dry-run to execute the files restore"
	}

	return FilesRestorePlan{
		Plan: RestorePlan{
			SourceKind:  restoreSourceKind(req.ManifestPath),
			SourcePath:  filesPath,
			Destructive: true,
			Changes: []string{
				fmt.Sprintf("replace target directory tree at %s", req.TargetDir),
				fmt.Sprintf("unpack backup archive from %s into staged restore data", filesPath),
			},
			NonChanges: []string{
				"does not modify database contents",
				"does not manage contour stop/start orchestration",
			},
			Checks: []RestorePlanCheck{
				{
					Name:    "backup_source",
					Status:  RestoreCheckPassed,
					Details: fmt.Sprintf("validated files backup at %s", filesPath),
				},
				{
					Name:    "target_parent",
					Status:  RestoreCheckPassed,
					Details: fmt.Sprintf("parent directory %s is writable", parentDir),
				},
				{
					Name:    "free_space",
					Status:  RestoreCheckPassed,
					Details: fmt.Sprintf("free space check passed for %s", parentDir),
				},
				{
					Name:    "restore_lock",
					Status:  RestoreCheckPassed,
					Details: "restore-files lock is currently acquirable",
				},
			},
			NextStep: nextStep,
		},
	}
}

func (s Service) resolveDBRootPasswordForPlan(req RestoreDBRequest) (string, RestorePlanCheck, error) {
	if !hasDBRootPasswordSource(req) {
		if req.DryRun {
			return "", RestorePlanCheck{
				Name:    "db_root_password_source",
				Status:  RestoreCheckPending,
				Details: "provide database root password source before executing without --dry-run",
			}, nil
		}

		return "", RestorePlanCheck{}, domainfailure.Failure{
			Kind: domainfailure.KindValidation,
			Code: "preflight_failed",
			Err:  fmt.Errorf("resolve db root password: database root password is required"),
		}
	}

	rootPassword, err := s.resolveDBRootPassword(req)
	if err != nil {
		return "", RestorePlanCheck{}, restoreFailure(domainfailure.KindValidation, "preflight_failed", fmt.Errorf("resolve db root password: %w", err))
	}

	return rootPassword, RestorePlanCheck{
		Name:    "db_root_password_source",
		Status:  RestoreCheckPassed,
		Details: "database root password source resolved",
	}, nil
}

func restoreSourceKind(manifestPath string) string {
	if strings.TrimSpace(manifestPath) != "" {
		return RestoreSourceManifest
	}

	return RestoreSourceDirectBackup
}

func hasDBRootPasswordSource(req RestoreDBRequest) bool {
	return strings.TrimSpace(req.DBRootPassword) != "" ||
		strings.TrimSpace(req.DBRootPasswordFile) != ""
}
