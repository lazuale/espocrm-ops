package restore

import (
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/lazuale/espocrm-ops/internal/contract/apperr"
	domainbackup "github.com/lazuale/espocrm-ops/internal/domain/backup"
	"github.com/lazuale/espocrm-ops/internal/platform/backupstore"
	platformconfig "github.com/lazuale/espocrm-ops/internal/platform/config"
	platformdocker "github.com/lazuale/espocrm-ops/internal/platform/docker"
	backupusecase "github.com/lazuale/espocrm-ops/internal/usecase/backup"
	maintenanceusecase "github.com/lazuale/espocrm-ops/internal/usecase/maintenance"
)

const (
	DrillStepStatusCompleted = "completed"
	DrillStepStatusFailed    = "failed"
	DrillStepStatusNotRun    = "not_run"

	defaultRestoreDrillTimeoutSeconds = 600
)

var restoreDrillRuntimeServices = []string{
	"db",
	"espocrm",
	"espocrm-daemon",
	"espocrm-websocket",
}

type DrillRequest struct {
	Scope           string
	ProjectDir      string
	ComposeFile     string
	EnvFileOverride string
	EnvContourHint  string
	DBBackup        string
	FilesBackup     string
	TimeoutSeconds  int
	DrillAppPort    int
	DrillWSPort     int
	SkipHTTPProbe   bool
	KeepArtifacts   bool
	LogWriter       io.Writer
}

type DrillStep struct {
	Code    string
	Status  string
	Summary string
	Details string
	Action  string
}

type DrillInfo struct {
	Scope                  string
	ProjectDir             string
	ComposeFile            string
	SourceEnvFile          string
	SourceComposeProject   string
	SourceBackupRoot       string
	RequestedSelectionMode string
	SelectionMode          string
	SelectedPrefix         string
	SelectedStamp          string
	ManifestTXTPath        string
	ManifestJSONPath       string
	DBBackupPath           string
	FilesBackupPath        string
	TimeoutSeconds         int
	SkipHTTPProbe          bool
	KeepArtifacts          bool
	DrillAppPort           int
	DrillWSPort            int
	DrillEnvFile           string
	DrillComposeProject    string
	DrillBackupRoot        string
	DrillDBStorage         string
	DrillESPOStorage       string
	SiteURL                string
	WSPublicURL            string
	ReportTXTPath          string
	ReportJSONPath         string
	ServicesReady          []string
	Warnings               []string
	Steps                  []DrillStep
}

type drillSourceSelection struct {
	RequestedSelectionMode string
	SelectionMode          string
	SourceKind             string
	Prefix                 string
	Stamp                  string
	ManifestJSON           string
	ManifestTXT            string
	DBBackup               string
	FilesBackup            string
}

func ExecuteDrill(req DrillRequest) (info DrillInfo, err error) {
	info = DrillInfo{
		Scope:                  strings.TrimSpace(req.Scope),
		ProjectDir:             filepath.Clean(req.ProjectDir),
		ComposeFile:            filepath.Clean(req.ComposeFile),
		RequestedSelectionMode: drillRequestedSelectionMode(req),
		TimeoutSeconds:         req.TimeoutSeconds,
		SkipHTTPProbe:          req.SkipHTTPProbe,
		KeepArtifacts:          req.KeepArtifacts,
		Warnings:               drillFlagWarnings(req),
	}
	if info.TimeoutSeconds <= 0 {
		info.TimeoutSeconds = defaultRestoreDrillTimeoutSeconds
	}

	var (
		cleanupEnabled bool
		drillCfg       platformdocker.ComposeConfig
	)
	defer func() {
		if !cleanupEnabled {
			info.Warnings = dedupeStrings(info.Warnings)
			return
		}

		if info.KeepArtifacts {
			info.Warnings = append(info.Warnings,
				"Temporary restore-drill contour preserved because of --keep-artifacts.",
			)
			if strings.TrimSpace(info.DrillEnvFile) != "" {
				info.Warnings = append(info.Warnings, fmt.Sprintf("Drill contour env file: %s", info.DrillEnvFile))
			}
			if strings.TrimSpace(info.SiteURL) != "" {
				info.Warnings = append(info.Warnings, fmt.Sprintf("Drill contour URL: %s", info.SiteURL))
			}
			info.Warnings = dedupeStrings(info.Warnings)
			return
		}

		info.Warnings = append(info.Warnings, cleanupRestoreDrill(drillCfg, info)...)
		info.Warnings = dedupeStrings(info.Warnings)
	}()

	ctx, prepErr := maintenanceusecase.PrepareOperation(maintenanceusecase.OperationContextRequest{
		Scope:           info.Scope,
		Operation:       "restore-drill",
		ProjectDir:      info.ProjectDir,
		EnvFileOverride: strings.TrimSpace(req.EnvFileOverride),
		EnvContourHint:  strings.TrimSpace(req.EnvContourHint),
		LogWriter:       req.LogWriter,
	})
	if prepErr != nil {
		info.Steps = append(info.Steps,
			DrillStep{
				Code:    "operation_preflight",
				Status:  DrillStepStatusFailed,
				Summary: "Restore-drill preflight failed",
				Details: prepErr.Error(),
				Action:  "Resolve env, lock, or filesystem readiness before rerunning restore-drill.",
			},
			notRunDrillStep("source_selection", "Restore-drill source selection did not run because preflight failed"),
			notRunDrillStep("drill_prepare", "Restore-drill contour preparation did not run because preflight failed"),
			notRunDrillStep("db_start", "Temporary database start did not run because preflight failed"),
			notRunDrillStep("db_restore", "Database restore did not run because preflight failed"),
			notRunDrillStep("files_restore", "Files restore did not run because preflight failed"),
			notRunDrillStep("runtime_return", "Runtime return did not run because preflight failed"),
		)
		return info, wrapRestoreDrillError(prepErr)
	}
	defer func() {
		_ = ctx.Release()
	}()

	info.SourceEnvFile = ctx.Env.FilePath
	info.SourceComposeProject = ctx.ComposeProject
	info.SourceBackupRoot = ctx.BackupRoot
	info.Steps = append(info.Steps, DrillStep{
		Code:    "operation_preflight",
		Status:  DrillStepStatusCompleted,
		Summary: "Restore-drill preflight completed",
		Details: fmt.Sprintf("Using %s for contour %s.", info.SourceEnvFile, info.Scope),
	})

	logRestoreDrill(req.LogWriter, "Selecting the restore-drill source")
	source, resolveErr := resolveDrillSource(ctx.BackupRoot, req)
	if resolveErr != nil {
		info.Steps = append(info.Steps,
			DrillStep{
				Code:    "source_selection",
				Status:  DrillStepStatusFailed,
				Summary: failureSummary(resolveErr, "Restore-drill source selection failed"),
				Details: resolveErr.Error(),
				Action:  failureAction(resolveErr, "Resolve the restore-drill source selection first and rerun the drill."),
			},
			notRunDrillStep("drill_prepare", "Restore-drill contour preparation did not run because source selection failed"),
			notRunDrillStep("db_start", "Temporary database start did not run because source selection failed"),
			notRunDrillStep("db_restore", "Database restore did not run because source selection failed"),
			notRunDrillStep("files_restore", "Files restore did not run because source selection failed"),
			notRunDrillStep("runtime_return", "Runtime return did not run because source selection failed"),
		)
		return info, apperr.Wrap(apperr.KindValidation, "restore_drill_failed", resolveErr)
	}

	info.RequestedSelectionMode = source.RequestedSelectionMode
	info.SelectionMode = source.SelectionMode
	info.SelectedPrefix = source.Prefix
	info.SelectedStamp = source.Stamp
	info.ManifestTXTPath = source.ManifestTXT
	info.ManifestJSONPath = source.ManifestJSON
	info.DBBackupPath = source.DBBackup
	info.FilesBackupPath = source.FilesBackup
	info.ReportTXTPath, info.ReportJSONPath = nextRestoreDrillReportPaths(
		ctx.BackupRoot,
		resolvedBackupNamePrefix(ctx.Env),
		time.Now().UTC(),
	)
	info.Steps = append(info.Steps, DrillStep{
		Code:    "source_selection",
		Status:  DrillStepStatusCompleted,
		Summary: restoreDrillSourceSummary(source),
		Details: restoreDrillSourceDetails(source),
	})

	logRestoreDrill(req.LogWriter, "Preparing the temporary restore-drill contour")
	drillEnv, drillPrepareErr := prepareRestoreDrillEnv(ctx, req, source, &info)
	if drillPrepareErr != nil {
		info.Steps = append(info.Steps,
			DrillStep{
				Code:    "drill_prepare",
				Status:  DrillStepStatusFailed,
				Summary: "Restore-drill contour preparation failed",
				Details: drillPrepareErr.Error(),
				Action:  "Resolve the temporary contour preparation failure before rerunning restore-drill.",
			},
			notRunDrillStep("db_start", "Temporary database start did not run because restore-drill contour preparation failed"),
			notRunDrillStep("db_restore", "Database restore did not run because restore-drill contour preparation failed"),
			notRunDrillStep("files_restore", "Files restore did not run because restore-drill contour preparation failed"),
			notRunDrillStep("runtime_return", "Runtime return did not run because restore-drill contour preparation failed"),
		)
		return info, wrapRestoreDrillExternalError(drillPrepareErr)
	}

	info.DrillComposeProject = drillEnv.ComposeProject()
	info.DrillBackupRoot = platformconfig.ResolveProjectPath(info.ProjectDir, drillEnv.BackupRoot())
	info.DrillDBStorage = platformconfig.ResolveProjectPath(info.ProjectDir, drillEnv.DBStorageDir())
	info.DrillESPOStorage = platformconfig.ResolveProjectPath(info.ProjectDir, drillEnv.ESPOStorageDir())
	info.SiteURL = strings.TrimSpace(drillEnv.Value("SITE_URL"))
	info.WSPublicURL = strings.TrimSpace(drillEnv.Value("WS_PUBLIC_URL"))
	drillCfg = platformdocker.ComposeConfig{
		ProjectDir:  info.ProjectDir,
		ComposeFile: info.ComposeFile,
		EnvFile:     info.DrillEnvFile,
	}
	cleanupEnabled = true

	info.Steps = append(info.Steps, DrillStep{
		Code:    "drill_prepare",
		Status:  DrillStepStatusCompleted,
		Summary: "Restore-drill contour preparation completed",
		Details: fmt.Sprintf("Prepared %s with compose project %s, db storage %s, files storage %s, and backup root %s. Drill URL: %s.",
			info.DrillEnvFile,
			info.DrillComposeProject,
			info.DrillDBStorage,
			info.DrillESPOStorage,
			info.DrillBackupRoot,
			info.SiteURL,
		),
	})

	readinessBudget := info.TimeoutSeconds
	logRestoreDrill(req.LogWriter, "Starting the temporary restore-drill database")
	if drillPrepareErr = platformdocker.ComposeUp(drillCfg, "db"); drillPrepareErr != nil {
		info.Steps = append(info.Steps,
			DrillStep{
				Code:    "db_start",
				Status:  DrillStepStatusFailed,
				Summary: "Temporary database start failed",
				Details: drillPrepareErr.Error(),
				Action:  "Resolve the temporary database start failure before rerunning restore-drill.",
			},
			notRunDrillStep("db_restore", "Database restore did not run because the temporary database failed to start"),
			notRunDrillStep("files_restore", "Files restore did not run because the temporary database failed to start"),
			notRunDrillStep("runtime_return", "Runtime return did not run because the temporary database failed to start"),
		)
		return info, wrapRestoreDrillExternalError(drillPrepareErr)
	}
	if drillPrepareErr = waitRestoreDrillServiceReadyWithSharedTimeout(&readinessBudget, drillCfg, "db", "restore-drill"); drillPrepareErr != nil {
		info.Steps = append(info.Steps,
			DrillStep{
				Code:    "db_start",
				Status:  DrillStepStatusFailed,
				Summary: "Temporary database start failed",
				Details: drillPrepareErr.Error(),
				Action:  "Resolve the temporary database readiness failure before rerunning restore-drill.",
			},
			notRunDrillStep("db_restore", "Database restore did not run because the temporary database failed to start"),
			notRunDrillStep("files_restore", "Files restore did not run because the temporary database failed to start"),
			notRunDrillStep("runtime_return", "Runtime return did not run because the temporary database failed to start"),
		)
		return info, wrapRestoreDrillExternalError(drillPrepareErr)
	}
	info.Steps = append(info.Steps, DrillStep{
		Code:    "db_start",
		Status:  DrillStepStatusCompleted,
		Summary: "Temporary database start completed",
		Details: fmt.Sprintf("Started the temporary database and confirmed readiness. Readiness waits are sharing a %d second timeout budget.", info.TimeoutSeconds),
	})

	dbContainer, drillPrepareErr := platformdocker.ComposeServiceContainerID(drillCfg, "db")
	if drillPrepareErr != nil {
		info.Steps = append(info.Steps,
			DrillStep{
				Code:    "db_restore",
				Status:  DrillStepStatusFailed,
				Summary: "Database restore failed",
				Details: drillPrepareErr.Error(),
				Action:  "Resolve the temporary db container inspection failure before rerunning restore-drill.",
			},
			notRunDrillStep("files_restore", "Files restore did not run because the database restore failed"),
			notRunDrillStep("runtime_return", "Runtime return did not run because the database restore failed"),
		)
		return info, wrapRestoreDrillExternalError(drillPrepareErr)
	}
	dbContainer = strings.TrimSpace(dbContainer)
	if dbContainer == "" {
		drillPrepareErr = errors.New("could not resolve the temporary db container for restore-drill")
		info.Steps = append(info.Steps,
			DrillStep{
				Code:    "db_restore",
				Status:  DrillStepStatusFailed,
				Summary: "Database restore failed",
				Details: drillPrepareErr.Error(),
				Action:  "Resolve the temporary db container failure before rerunning restore-drill.",
			},
			notRunDrillStep("files_restore", "Files restore did not run because the database restore failed"),
			notRunDrillStep("runtime_return", "Runtime return did not run because the database restore failed"),
		)
		return info, wrapRestoreDrillExternalError(drillPrepareErr)
	}

	logRestoreDrill(req.LogWriter, "Restoring the selected database backup into the temporary contour")
	if _, drillPrepareErr = RestoreDB(buildRestoreDrillDBRequest(info.ProjectDir, drillEnv, source, dbContainer)); drillPrepareErr != nil {
		info.Steps = append(info.Steps,
			DrillStep{
				Code:    "db_restore",
				Status:  DrillStepStatusFailed,
				Summary: "Database restore failed",
				Details: drillPrepareErr.Error(),
				Action:  "Resolve the database restore failure before rerunning restore-drill.",
			},
			notRunDrillStep("files_restore", "Files restore did not run because the database restore failed"),
			notRunDrillStep("runtime_return", "Runtime return did not run because the database restore failed"),
		)
		return info, wrapRestoreDrillError(drillPrepareErr)
	}
	info.Steps = append(info.Steps, DrillStep{
		Code:    "db_restore",
		Status:  DrillStepStatusCompleted,
		Summary: "Database restore completed",
		Details: restoreDrillDBDetails(drillEnv, source, dbContainer),
	})

	logRestoreDrill(req.LogWriter, "Restoring the selected files backup into the temporary contour")
	filesReq := buildRestoreDrillFilesRequest(info.ProjectDir, drillEnv, source)
	if _, drillPrepareErr = RestoreFiles(filesReq); drillPrepareErr != nil {
		info.Steps = append(info.Steps,
			DrillStep{
				Code:    "files_restore",
				Status:  DrillStepStatusFailed,
				Summary: "Files restore failed",
				Details: drillPrepareErr.Error(),
				Action:  "Resolve the files restore failure before rerunning restore-drill.",
			},
			notRunDrillStep("runtime_return", "Runtime return did not run because the files restore failed"),
		)
		return info, wrapRestoreDrillError(drillPrepareErr)
	}
	if drillPrepareErr = platformdocker.ReconcileEspoStoragePermissions(
		filesReq.TargetDir,
		strings.TrimSpace(drillEnv.Value("MARIADB_TAG")),
		strings.TrimSpace(drillEnv.Value("ESPOCRM_IMAGE")),
	); drillPrepareErr != nil {
		info.Steps = append(info.Steps,
			DrillStep{
				Code:    "files_restore",
				Status:  DrillStepStatusFailed,
				Summary: "Files restore failed",
				Details: fmt.Sprintf("Files were restored but runtime permission reconciliation failed: %v", drillPrepareErr),
				Action:  "Resolve the permission reconciliation failure before rerunning restore-drill.",
			},
			notRunDrillStep("runtime_return", "Runtime return did not run because the files restore failed"),
		)
		return info, wrapRestoreDrillExternalError(drillPrepareErr)
	}
	info.Steps = append(info.Steps, DrillStep{
		Code:    "files_restore",
		Status:  DrillStepStatusCompleted,
		Summary: "Files restore completed",
		Details: restoreDrillFilesDetails(info.ProjectDir, drillEnv, source),
	})

	logRestoreDrill(req.LogWriter, "Returning the temporary restore-drill contour to service")
	if drillPrepareErr = platformdocker.ComposeUp(drillCfg); drillPrepareErr != nil {
		info.Steps = append(info.Steps, DrillStep{
			Code:    "runtime_return",
			Status:  DrillStepStatusFailed,
			Summary: "Runtime return failed",
			Details: drillPrepareErr.Error(),
			Action:  "Resolve the temporary contour start failure before rerunning restore-drill.",
		})
		return info, wrapRestoreDrillExternalError(drillPrepareErr)
	}

	info.ServicesReady = info.ServicesReady[:0]
	for _, service := range restoreDrillRuntimeServices {
		if drillPrepareErr = waitRestoreDrillServiceReadyWithSharedTimeout(&readinessBudget, drillCfg, service, "restore-drill"); drillPrepareErr != nil {
			info.Steps = append(info.Steps, DrillStep{
				Code:    "runtime_return",
				Status:  DrillStepStatusFailed,
				Summary: "Runtime return failed",
				Details: drillPrepareErr.Error(),
				Action:  "Resolve the temporary contour readiness failure before rerunning restore-drill.",
			})
			return info, wrapRestoreDrillExternalError(drillPrepareErr)
		}
		info.ServicesReady = append(info.ServicesReady, service)
	}

	runtimeReturnDetails := fmt.Sprintf("Confirmed readiness for %s.", strings.Join(info.ServicesReady, ", "))
	if info.SkipHTTPProbe {
		runtimeReturnDetails += " The final HTTP probe was skipped because of --skip-http-probe."
	} else {
		if drillPrepareErr = restoreDrillHTTPProbe(info.SiteURL); drillPrepareErr != nil {
			info.Steps = append(info.Steps, DrillStep{
				Code:    "runtime_return",
				Status:  DrillStepStatusFailed,
				Summary: "Runtime return failed",
				Details: drillPrepareErr.Error(),
				Action:  "Resolve the temporary contour HTTP probe failure before rerunning restore-drill.",
			})
			return info, wrapRestoreDrillExternalError(drillPrepareErr)
		}
		runtimeReturnDetails += fmt.Sprintf(" The final HTTP probe succeeded for %s.", info.SiteURL)
	}
	info.Steps = append(info.Steps, DrillStep{
		Code:    "runtime_return",
		Status:  DrillStepStatusCompleted,
		Summary: "Runtime return completed",
		Details: runtimeReturnDetails,
	})

	return info, nil
}

func (i DrillInfo) Counts() (completed, failed, notRun int) {
	for _, step := range i.Steps {
		switch step.Status {
		case DrillStepStatusCompleted:
			completed++
		case DrillStepStatusFailed:
			failed++
		case DrillStepStatusNotRun:
			notRun++
		}
	}

	return completed, failed, notRun
}

func (i DrillInfo) Ready() bool {
	for _, step := range i.Steps {
		if step.Status == DrillStepStatusFailed || step.Status == DrillStepStatusNotRun {
			return false
		}
	}

	return true
}

func resolveDrillSource(backupRoot string, req DrillRequest) (drillSourceSelection, error) {
	dbBackup := strings.TrimSpace(req.DBBackup)
	filesBackup := strings.TrimSpace(req.FilesBackup)
	requestedMode := drillRequestedSelectionMode(req)

	switch {
	case dbBackup == "" && filesBackup == "":
		manifestPath, err := backupusecase.LatestCompleteManifest(backupRoot)
		if err != nil {
			return drillSourceSelection{}, executeFailure{
				Summary: "No complete backup set with checksums and manifests was found for restore-drill",
				Action:  "Create or repair a complete backup set, or pass explicit --db-backup and --files-backup values.",
				Err:     err,
			}
		}
		info, err := backupstore.VerifyManifestDetailed(manifestPath)
		if err != nil {
			return drillSourceSelection{}, executeFailure{
				Summary: "The selected restore-drill manifest is not valid",
				Action:  "Repair the selected backup set or pass explicit backup paths for restore-drill.",
				Err:     err,
			}
		}
		group, _, err := domainbackup.ParseManifestName(manifestPath)
		if err != nil {
			return drillSourceSelection{}, executeFailure{
				Summary: "The selected restore-drill manifest name is unsupported",
				Action:  "Rename the manifest to the canonical backup-set pattern or pass explicit backup paths.",
				Err:     err,
			}
		}
		return drillSourceSelection{
			RequestedSelectionMode: requestedMode,
			SelectionMode:          "auto_latest_valid",
			SourceKind:             "manifest",
			Prefix:                 group.Prefix,
			Stamp:                  group.Stamp,
			ManifestJSON:           filepath.Clean(manifestPath),
			ManifestTXT:            matchingManifestTXT(manifestPath),
			DBBackup:               info.DBBackupPath,
			FilesBackup:            info.FilesPath,
		}, nil
	case dbBackup != "" && filesBackup != "":
		dbBackup = filepath.Clean(dbBackup)
		filesBackup = filepath.Clean(filesBackup)
		if err := backupstore.VerifyDirectDBBackup(dbBackup); err != nil {
			return drillSourceSelection{}, executeFailure{
				Summary: "The selected database backup is not valid",
				Action:  "Choose a readable .sql.gz database backup with a valid .sha256 sidecar.",
				Err:     err,
			}
		}
		if err := backupstore.VerifyDirectFilesBackup(filesBackup); err != nil {
			return drillSourceSelection{}, executeFailure{
				Summary: "The selected files backup is not valid",
				Action:  "Choose a readable .tar.gz files backup with a valid .sha256 sidecar.",
				Err:     err,
			}
		}
		if err := validateDirectPair(dbBackup, filesBackup); err != nil {
			return drillSourceSelection{}, err
		}
		group, _ := domainbackup.ParseDBBackupName(dbBackup)
		return drillSourceSelection{
			RequestedSelectionMode: requestedMode,
			SelectionMode:          "explicit_pair",
			SourceKind:             "direct",
			Prefix:                 group.Prefix,
			Stamp:                  group.Stamp,
			ManifestJSON:           matchingDrillManifest(backupRoot, group),
			ManifestTXT:            matchingManifestTXT(matchingDrillManifest(backupRoot, group)),
			DBBackup:               dbBackup,
			FilesBackup:            filesBackup,
		}, nil
	case dbBackup != "":
		dbBackup = filepath.Clean(dbBackup)
		if err := backupstore.VerifyDirectDBBackup(dbBackup); err != nil {
			return drillSourceSelection{}, executeFailure{
				Summary: "The selected database backup is not valid",
				Action:  "Choose a readable .sql.gz database backup with a valid .sha256 sidecar.",
				Err:     err,
			}
		}
		group, err := domainbackup.ParseDBBackupName(dbBackup)
		if err != nil {
			return drillSourceSelection{}, executeFailure{
				Summary: "The selected database backup name is unsupported",
				Action:  "Choose a canonical .sql.gz backup path or pass both backup paths explicitly.",
				Err:     err,
			}
		}
		set := domainbackup.BuildBackupSet(backupRoot, group.Prefix, group.Stamp)
		if err := backupstore.VerifyDirectFilesBackup(set.FilesBackup.Path); err != nil {
			return drillSourceSelection{}, executeFailure{
				Summary: "No coherent files backup was found for restore-drill",
				Action:  "Repair the matching files backup or pass --files-backup explicitly.",
				Err:     err,
			}
		}
		return drillSourceSelection{
			RequestedSelectionMode: requestedMode,
			SelectionMode:          "explicit_db_auto_files",
			SourceKind:             "direct",
			Prefix:                 group.Prefix,
			Stamp:                  group.Stamp,
			ManifestJSON:           matchingDrillManifest(backupRoot, group),
			ManifestTXT:            matchingManifestTXT(matchingDrillManifest(backupRoot, group)),
			DBBackup:               dbBackup,
			FilesBackup:            set.FilesBackup.Path,
		}, nil
	default:
		filesBackup = filepath.Clean(filesBackup)
		if err := backupstore.VerifyDirectFilesBackup(filesBackup); err != nil {
			return drillSourceSelection{}, executeFailure{
				Summary: "The selected files backup is not valid",
				Action:  "Choose a readable .tar.gz files backup with a valid .sha256 sidecar.",
				Err:     err,
			}
		}
		group, err := domainbackup.ParseFilesBackupName(filesBackup)
		if err != nil {
			return drillSourceSelection{}, executeFailure{
				Summary: "The selected files backup name is unsupported",
				Action:  "Choose a canonical .tar.gz backup path or pass both backup paths explicitly.",
				Err:     err,
			}
		}
		set := domainbackup.BuildBackupSet(backupRoot, group.Prefix, group.Stamp)
		if err := backupstore.VerifyDirectDBBackup(set.DBBackup.Path); err != nil {
			return drillSourceSelection{}, executeFailure{
				Summary: "No coherent database backup was found for restore-drill",
				Action:  "Repair the matching database backup or pass --db-backup explicitly.",
				Err:     err,
			}
		}
		return drillSourceSelection{
			RequestedSelectionMode: requestedMode,
			SelectionMode:          "explicit_files_auto_db",
			SourceKind:             "direct",
			Prefix:                 group.Prefix,
			Stamp:                  group.Stamp,
			ManifestJSON:           matchingDrillManifest(backupRoot, group),
			ManifestTXT:            matchingManifestTXT(matchingDrillManifest(backupRoot, group)),
			DBBackup:               set.DBBackup.Path,
			FilesBackup:            filesBackup,
		}, nil
	}
}

func matchingDrillManifest(backupRoot string, group domainbackup.BackupGroup) string {
	path := domainbackup.BuildBackupSet(backupRoot, group.Prefix, group.Stamp).ManifestJSON.Path
	if err := backupstore.VerifyManifest(path); err != nil {
		return ""
	}

	return path
}

func prepareRestoreDrillEnv(ctx maintenanceusecase.OperationContext, req DrillRequest, source drillSourceSelection, info *DrillInfo) (platformconfig.OperationEnv, error) {
	sourceAppPort, err := parseRestoreDrillPort(ctx.Env.Value("APP_PORT"), "source APP_PORT")
	if err != nil {
		return platformconfig.OperationEnv{}, err
	}
	sourceWSPort, err := parseRestoreDrillPort(ctx.Env.Value("WS_PORT"), "source WS_PORT")
	if err != nil {
		return platformconfig.OperationEnv{}, err
	}

	if info.DrillAppPort == 0 {
		fallback := 28088
		if info.Scope == "prod" {
			fallback = 28080
		}
		info.DrillAppPort = deriveRestoreDrillPort(sourceAppPort, fallback)
	}
	if info.DrillWSPort == 0 {
		fallback := 28089
		if info.Scope == "prod" {
			fallback = 28081
		}
		info.DrillWSPort = deriveRestoreDrillPort(sourceWSPort, fallback)
	}

	if err := ensureRestoreDrillPortAvailable(info.DrillAppPort, "HTTP"); err != nil {
		return platformconfig.OperationEnv{}, err
	}
	if err := ensureRestoreDrillPortAvailable(info.DrillWSPort, "websocket"); err != nil {
		return platformconfig.OperationEnv{}, err
	}

	cacheDir := filepath.Join(info.ProjectDir, ".cache", "env")
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		return platformconfig.OperationEnv{}, fmt.Errorf("create restore-drill env dir %s: %w", cacheDir, err)
	}

	envFile, err := os.CreateTemp(cacheDir, fmt.Sprintf("restore-drill.%s.*.env", info.Scope))
	if err != nil {
		return platformconfig.OperationEnv{}, fmt.Errorf("create restore-drill env file: %w", err)
	}
	info.DrillEnvFile = envFile.Name()
	if err := envFile.Close(); err != nil {
		return platformconfig.OperationEnv{}, fmt.Errorf("close restore-drill env file %s: %w", info.DrillEnvFile, err)
	}

	values := restoreDrillEnvValues(ctx.Env, info.Scope, info.DrillAppPort, info.DrillWSPort)
	if err := writeRestoreDrillEnvFile(info.DrillEnvFile, values); err != nil {
		return platformconfig.OperationEnv{}, err
	}

	drillEnv, err := platformconfig.LoadOperationEnv(info.ProjectDir, info.Scope, info.DrillEnvFile, strings.TrimSpace(req.EnvContourHint))
	if err != nil {
		return platformconfig.OperationEnv{}, err
	}

	if err := ensureRestoreDrillRuntimeDirs(info.ProjectDir, drillEnv); err != nil {
		return platformconfig.OperationEnv{}, err
	}

	cfg := platformdocker.ComposeConfig{
		ProjectDir:  info.ProjectDir,
		ComposeFile: info.ComposeFile,
		EnvFile:     info.DrillEnvFile,
	}
	if err := platformdocker.ComposeDown(cfg); err != nil {
		return platformconfig.OperationEnv{}, err
	}

	drillDBStorage := platformconfig.ResolveProjectPath(info.ProjectDir, drillEnv.DBStorageDir())
	drillESPOStorage := platformconfig.ResolveProjectPath(info.ProjectDir, drillEnv.ESPOStorageDir())
	drillBackupRoot := platformconfig.ResolveProjectPath(info.ProjectDir, drillEnv.BackupRoot())
	if err := removeRestoreDrillTree(info.ProjectDir, drillDBStorage); err != nil {
		return platformconfig.OperationEnv{}, err
	}
	if err := removeRestoreDrillTree(info.ProjectDir, drillESPOStorage); err != nil {
		return platformconfig.OperationEnv{}, err
	}
	if err := removeRestoreDrillTree(info.ProjectDir, drillBackupRoot); err != nil {
		return platformconfig.OperationEnv{}, err
	}
	if err := ensureRestoreDrillRuntimeDirs(info.ProjectDir, drillEnv); err != nil {
		return platformconfig.OperationEnv{}, err
	}

	_ = source

	return drillEnv, nil
}

func restoreDrillEnvValues(source platformconfig.OperationEnv, scope string, appPort, wsPort int) map[string]string {
	values := map[string]string{}
	for key, value := range source.Values {
		values[key] = value
	}

	values["COMPOSE_PROJECT_NAME"] = "espo-restore-drill-" + scope
	values["DB_STORAGE_DIR"] = "./storage/restore-drill/" + scope + "/db"
	values["ESPO_STORAGE_DIR"] = "./storage/restore-drill/" + scope + "/espo"
	values["BACKUP_ROOT"] = "./backups/restore-drill/" + scope
	values["BACKUP_NAME_PREFIX"] = "espocrm-restore-drill-" + scope
	values["APP_PORT"] = strconv.Itoa(appPort)
	values["WS_PORT"] = strconv.Itoa(wsPort)
	values["SITE_URL"] = fmt.Sprintf("http://127.0.0.1:%d", appPort)
	values["WS_PUBLIC_URL"] = fmt.Sprintf("ws://127.0.0.1:%d", wsPort)
	values["DB_NAME"] = "espocrm_restore_drill_" + scope
	values["DB_ROOT_PASSWORD"] = "restore_drill_" + scope + "_root"
	values["DB_PASSWORD"] = "restore_drill_" + scope + "_db"
	values["ADMIN_PASSWORD"] = "restore_drill_" + scope + "_admin"

	return values
}

func writeRestoreDrillEnvFile(path string, values map[string]string) error {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	lines := make([]string, 0, len(keys))
	for _, key := range keys {
		lines = append(lines, key+"="+quoteRestoreDrillEnvValue(values[key]))
	}

	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		return fmt.Errorf("write restore-drill env file %s: %w", path, err)
	}

	return nil
}

func quoteRestoreDrillEnvValue(value string) string {
	if value == "" {
		return `""`
	}
	if strings.ContainsAny(value, " \t") || strings.ContainsAny(value, `"'#$`) {
		escaped := strings.ReplaceAll(value, `\`, `\\`)
		escaped = strings.ReplaceAll(escaped, `"`, `\"`)
		return `"` + escaped + `"`
	}

	return value
}

func ensureRestoreDrillRuntimeDirs(projectDir string, env platformconfig.OperationEnv) error {
	paths := []string{
		platformconfig.ResolveProjectPath(projectDir, env.DBStorageDir()),
		platformconfig.ResolveProjectPath(projectDir, env.ESPOStorageDir()),
		filepath.Join(platformconfig.ResolveProjectPath(projectDir, env.BackupRoot()), "db"),
		filepath.Join(platformconfig.ResolveProjectPath(projectDir, env.BackupRoot()), "files"),
		filepath.Join(platformconfig.ResolveProjectPath(projectDir, env.BackupRoot()), "locks"),
		filepath.Join(platformconfig.ResolveProjectPath(projectDir, env.BackupRoot()), "manifests"),
		filepath.Join(platformconfig.ResolveProjectPath(projectDir, env.BackupRoot()), "reports"),
		filepath.Join(platformconfig.ResolveProjectPath(projectDir, env.BackupRoot()), "support"),
	}

	for _, path := range paths {
		if err := os.MkdirAll(path, 0o755); err != nil {
			return fmt.Errorf("create restore-drill runtime directory %s: %w", path, err)
		}
	}

	return nil
}

func removeRestoreDrillTree(projectDir, target string) error {
	target = strings.TrimSpace(target)
	if target == "" {
		return nil
	}
	rel, err := filepath.Rel(projectDir, target)
	if err != nil {
		return fmt.Errorf("resolve restore-drill cleanup path %s: %w", target, err)
	}
	if rel == "." || rel == "" {
		return fmt.Errorf("refusing to remove the restore-drill project root %s", target)
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return fmt.Errorf("refusing to remove a restore-drill path outside the project directory: %s", target)
	}
	if err := os.RemoveAll(target); err != nil {
		return fmt.Errorf("remove restore-drill path %s: %w", target, err)
	}

	return nil
}

func cleanupRestoreDrill(cfg platformdocker.ComposeConfig, info DrillInfo) []string {
	warnings := []string{}
	if strings.TrimSpace(cfg.EnvFile) != "" {
		if err := platformdocker.ComposeDown(cfg); err != nil {
			warnings = append(warnings, fmt.Sprintf("Could not automatically stop the temporary restore-drill contour: %v", err))
		}
		if err := os.Remove(cfg.EnvFile); err != nil && !os.IsNotExist(err) {
			warnings = append(warnings, fmt.Sprintf("Could not remove the temporary restore-drill env file %s: %v", cfg.EnvFile, err))
		}
	}
	for _, target := range []string{info.DrillDBStorage, info.DrillESPOStorage, info.DrillBackupRoot} {
		if err := removeRestoreDrillTree(info.ProjectDir, target); err != nil {
			warnings = append(warnings, err.Error())
		}
	}

	return warnings
}

func buildRestoreDrillDBRequest(projectDir string, env platformconfig.OperationEnv, source drillSourceSelection, dbContainer string) RestoreDBRequest {
	req := RestoreDBRequest{
		DBContainer:    dbContainer,
		DBName:         strings.TrimSpace(env.Value("DB_NAME")),
		DBUser:         strings.TrimSpace(env.Value("DB_USER")),
		DBPassword:     env.Value("DB_PASSWORD"),
		DBRootPassword: env.Value("DB_ROOT_PASSWORD"),
	}

	if strings.TrimSpace(source.ManifestJSON) != "" && source.SourceKind == "manifest" {
		req.ManifestPath = source.ManifestJSON
		return req
	}

	req.DBBackup = source.DBBackup
	_ = projectDir
	return req
}

func buildRestoreDrillFilesRequest(projectDir string, env platformconfig.OperationEnv, source drillSourceSelection) RestoreFilesRequest {
	req := RestoreFilesRequest{
		TargetDir: platformconfig.ResolveProjectPath(projectDir, env.ESPOStorageDir()),
	}

	if strings.TrimSpace(source.ManifestJSON) != "" && source.SourceKind == "manifest" {
		req.ManifestPath = source.ManifestJSON
		return req
	}

	req.FilesBackup = source.FilesBackup
	return req
}

func restoreDrillSourceSummary(source drillSourceSelection) string {
	switch source.SelectionMode {
	case "auto_latest_valid":
		return "Automatic restore-drill source selection completed"
	case "explicit_db_auto_files":
		return "Explicit database backup selection completed with automatic files resolution"
	case "explicit_files_auto_db":
		return "Explicit files backup selection completed with automatic database resolution"
	default:
		return "Explicit restore-drill source selection completed"
	}
}

func restoreDrillSourceDetails(source drillSourceSelection) string {
	switch source.SelectionMode {
	case "auto_latest_valid":
		return fmt.Sprintf("Selected prefix %s at %s with manifest %s.", source.Prefix, source.Stamp, source.ManifestJSON)
	case "explicit_db_auto_files":
		return fmt.Sprintf("Using explicit database backup %s and matching files backup %s from %s.", source.DBBackup, source.FilesBackup, source.Prefix+"_"+source.Stamp)
	case "explicit_files_auto_db":
		return fmt.Sprintf("Using matching database backup %s and explicit files backup %s from %s.", source.DBBackup, source.FilesBackup, source.Prefix+"_"+source.Stamp)
	default:
		if strings.TrimSpace(source.ManifestJSON) != "" {
			return fmt.Sprintf("Using explicit database backup %s and files backup %s. Matching manifest %s is available for the selected set.", source.DBBackup, source.FilesBackup, source.ManifestJSON)
		}
		return fmt.Sprintf("Using explicit database backup %s and files backup %s.", source.DBBackup, source.FilesBackup)
	}
}

func restoreDrillDBDetails(env platformconfig.OperationEnv, source drillSourceSelection, dbContainer string) string {
	details := fmt.Sprintf("Restored database %s in container %s from %s.", strings.TrimSpace(env.Value("DB_NAME")), dbContainer, source.DBBackup)
	if strings.TrimSpace(source.ManifestJSON) != "" {
		details += fmt.Sprintf(" The selected backup set is anchored by %s.", source.ManifestJSON)
	}
	return details
}

func restoreDrillFilesDetails(projectDir string, env platformconfig.OperationEnv, source drillSourceSelection) string {
	targetDir := platformconfig.ResolveProjectPath(projectDir, env.ESPOStorageDir())
	details := fmt.Sprintf("Replaced %s from %s and reconciled the storage permissions to the runtime image contract.", targetDir, source.FilesBackup)
	if strings.TrimSpace(source.ManifestJSON) != "" {
		details += fmt.Sprintf(" The selected backup set is anchored by %s.", source.ManifestJSON)
	}
	return details
}

func drillRequestedSelectionMode(req DrillRequest) string {
	if strings.TrimSpace(req.DBBackup) != "" || strings.TrimSpace(req.FilesBackup) != "" {
		return "explicit"
	}

	return "auto_latest_valid"
}

func drillFlagWarnings(req DrillRequest) []string {
	warnings := []string{}
	if req.SkipHTTPProbe {
		warnings = append(warnings, "Restore-drill will skip the final HTTP probe because of --skip-http-probe.")
	}

	return warnings
}

func notRunDrillStep(code, summary string) DrillStep {
	return DrillStep{
		Code:    code,
		Status:  DrillStepStatusNotRun,
		Summary: summary,
	}
}

func deriveRestoreDrillPort(sourcePort, fallback int) int {
	if sourcePort > 0 && sourcePort+20000 <= 65535 {
		return sourcePort + 20000
	}

	return fallback
}

func parseRestoreDrillPort(value, label string) (int, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, nil
	}

	port, err := strconv.Atoi(value)
	if err != nil {
		return 0, executeFailure{
			Summary: fmt.Sprintf("The %s setting is not numeric", label),
			Action:  "Fix the contour port configuration before rerunning restore-drill.",
			Err:     fmt.Errorf("%s must be numeric, got %q", label, value),
		}
	}

	return port, nil
}

func ensureRestoreDrillPortAvailable(port int, label string) error {
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return executeFailure{
			Summary: fmt.Sprintf("The requested %s port is not available", label),
			Action:  "Choose a free restore-drill port and rerun the drill.",
			Err:     fmt.Errorf("port %d is already in use, restore-drill cannot use it for %s", port, label),
		}
	}
	if closeErr := listener.Close(); closeErr != nil {
		return fmt.Errorf("close restore-drill port probe %s: %w", addr, closeErr)
	}

	return nil
}

func waitRestoreDrillServiceReadyWithSharedTimeout(timeoutBudget *int, cfg platformdocker.ComposeConfig, service, scope string) error {
	if timeoutBudget == nil {
		return errors.New("shared readiness timeout budget is not configured")
	}
	if *timeoutBudget <= 0 {
		return fmt.Errorf("shared readiness timeout for %s was exhausted before service '%s'", scope, service)
	}

	started := time.Now().UTC()
	if err := waitForServiceReady(cfg, service, *timeoutBudget); err != nil {
		return err
	}

	elapsed := int(time.Since(started).Seconds())
	*timeoutBudget -= elapsed
	if *timeoutBudget < 0 {
		*timeoutBudget = 0
	}

	return nil
}

func restoreDrillHTTPProbe(siteURL string) error {
	if strings.TrimSpace(siteURL) == "" {
		return errors.New("SITE_URL is required for the restore-drill HTTP probe")
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(siteURL)
	if err != nil {
		return fmt.Errorf("http probe failed: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusBadRequest {
		return fmt.Errorf("http probe failed: unexpected status %s", resp.Status)
	}

	return nil
}

func nextRestoreDrillReportPaths(backupRoot, prefix string, now time.Time) (string, string) {
	reportsDir := filepath.Join(backupRoot, "reports")
	base := prefix
	if strings.TrimSpace(base) == "" {
		base = "espocrm"
	}

	stampTime := now.UTC()
	for {
		stamp := stampTime.Format("2006-01-02_15-04-05")
		txtPath := filepath.Join(reportsDir, fmt.Sprintf("%s_restore-drill_%s.txt", base, stamp))
		jsonPath := filepath.Join(reportsDir, fmt.Sprintf("%s_restore-drill_%s.json", base, stamp))
		if !restoreDrillPathExists(txtPath) && !restoreDrillPathExists(jsonPath) {
			return txtPath, jsonPath
		}
		stampTime = stampTime.Add(time.Second)
	}
}

func restoreDrillPathExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func logRestoreDrill(w io.Writer, message string) {
	if w == nil {
		return
	}

	_, _ = fmt.Fprintln(w, message)
}

func wrapRestoreDrillError(err error) error {
	if kind, ok := apperr.KindOf(err); ok {
		return apperr.Wrap(kind, "restore_drill_failed", err)
	}

	return apperr.Wrap(apperr.KindInternal, "restore_drill_failed", err)
}

func wrapRestoreDrillExternalError(err error) error {
	if kind, ok := apperr.KindOf(err); ok && kind != "" {
		return apperr.Wrap(kind, "restore_drill_failed", err)
	}

	return apperr.Wrap(apperr.KindExternal, "restore_drill_failed", err)
}
