package restore

import (
	"fmt"
	"strings"

	"github.com/lazuale/espocrm-ops/internal/contract/apperr"
	maintenanceusecase "github.com/lazuale/espocrm-ops/internal/usecase/maintenance"
	"github.com/lazuale/espocrm-ops/internal/usecase/reporting"
)

const (
	GateCheckPassed  = "passed"
	GateCheckBlocked = "blocked"
)

type DrillGateRequest struct {
	Scope           string
	ProjectDir      string
	ComposeFile     string
	EnvFileOverride string
	EnvContourHint  string
	DBBackup        string
	FilesBackup     string
	DrillAppPort    int
	DrillWSPort     int
	SkipHTTPProbe   bool
	KeepArtifacts   bool
}

type DrillGateCheck struct {
	Code    string
	Status  string
	Summary string
	Details string
	Action  string
}

type DrillGateInfo struct {
	Scope                  string
	ProjectDir             string
	ComposeFile            string
	EnvFile                string
	BackupRoot             string
	RequestedSelectionMode string
	SelectionMode          string
	SelectedPrefix         string
	SelectedStamp          string
	ManifestTXTPath        string
	ManifestJSONPath       string
	DBBackupPath           string
	FilesBackupPath        string
	DrillAppPort           int
	DrillWSPort            int
	Warnings               []string
	Checks                 []DrillGateCheck
}

func PreflightDrill(req DrillGateRequest) (DrillGateInfo, error) {
	info := DrillGateInfo{
		Scope:                  strings.TrimSpace(req.Scope),
		ProjectDir:             strings.TrimSpace(req.ProjectDir),
		ComposeFile:            strings.TrimSpace(req.ComposeFile),
		RequestedSelectionMode: drillRequestedSelectionMode(DrillRequest{DBBackup: req.DBBackup, FilesBackup: req.FilesBackup}),
		Warnings:               drillGateWarnings(req),
	}

	if req.DrillAppPort != 0 && req.DrillWSPort != 0 && req.DrillAppPort == req.DrillWSPort {
		err := executeFailure{
			Summary: "Restore-drill port selection is invalid",
			Action:  "Choose different APP and websocket ports before rerunning operation-gate.",
			Err:     fmt.Errorf("restore-drill APP and websocket ports must differ"),
		}
		info.Checks = append(info.Checks, DrillGateCheck{
			Code:    "port_readiness",
			Status:  GateCheckBlocked,
			Summary: failureSummary(err, "Restore-drill port readiness failed"),
			Details: err.Error(),
			Action:  failureAction(err, "Choose different restore-drill ports before rerunning operation-gate."),
		})
		return info, wrapRestoreDrillError(err)
	}

	ctx, err := maintenanceusecase.PrepareOperation(maintenanceusecase.OperationContextRequest{
		Scope:           info.Scope,
		Operation:       "restore-drill",
		ProjectDir:      info.ProjectDir,
		EnvFileOverride: strings.TrimSpace(req.EnvFileOverride),
		EnvContourHint:  strings.TrimSpace(req.EnvContourHint),
	})
	if err != nil {
		info.Checks = append(info.Checks, DrillGateCheck{
			Code:    "operation_preflight",
			Status:  GateCheckBlocked,
			Summary: "Restore-drill preflight failed",
			Details: err.Error(),
			Action:  "Resolve env, lock, or filesystem readiness before rerunning restore-drill.",
		})
		return info, wrapRestoreDrillError(err)
	}
	defer func() {
		_ = ctx.Release()
	}()

	info.EnvFile = ctx.Env.FilePath
	info.BackupRoot = ctx.BackupRoot
	info.Checks = append(info.Checks, DrillGateCheck{
		Code:    "operation_preflight",
		Status:  GateCheckPassed,
		Summary: "Restore-drill preflight passed",
		Details: fmt.Sprintf("Using %s for contour %s.", info.EnvFile, info.Scope),
	})

	source, err := resolveDrillSource(ctx.BackupRoot, DrillRequest{
		DBBackup:    req.DBBackup,
		FilesBackup: req.FilesBackup,
	})
	if err != nil {
		info.Checks = append(info.Checks, DrillGateCheck{
			Code:    "source_selection",
			Status:  GateCheckBlocked,
			Summary: failureSummary(err, "Restore-drill source selection failed"),
			Details: err.Error(),
			Action:  failureAction(err, "Resolve the restore-drill source selection first and rerun the drill."),
		})
		return info, apperr.Wrap(apperr.KindValidation, "restore_drill_failed", err)
	}

	info.SelectionMode = source.SelectionMode
	info.SelectedPrefix = source.Prefix
	info.SelectedStamp = source.Stamp
	info.ManifestTXTPath = source.ManifestTXT
	info.ManifestJSONPath = source.ManifestJSON
	info.DBBackupPath = source.DBBackup
	info.FilesBackupPath = source.FilesBackup
	info.Checks = append(info.Checks, DrillGateCheck{
		Code:    "source_selection",
		Status:  GateCheckPassed,
		Summary: restoreDrillSourceSummary(source),
		Details: restoreDrillSourceDetails(source),
	})

	sourceAppPort, err := parseRestoreDrillPort(ctx.Env.Value("APP_PORT"), "source APP_PORT")
	if err != nil {
		info.Checks = append(info.Checks, DrillGateCheck{
			Code:    "port_readiness",
			Status:  GateCheckBlocked,
			Summary: failureSummary(err, "Restore-drill port readiness failed"),
			Details: err.Error(),
			Action:  failureAction(err, "Fix the contour port configuration before rerunning restore-drill."),
		})
		return info, wrapRestoreDrillError(err)
	}
	sourceWSPort, err := parseRestoreDrillPort(ctx.Env.Value("WS_PORT"), "source WS_PORT")
	if err != nil {
		info.Checks = append(info.Checks, DrillGateCheck{
			Code:    "port_readiness",
			Status:  GateCheckBlocked,
			Summary: failureSummary(err, "Restore-drill port readiness failed"),
			Details: err.Error(),
			Action:  failureAction(err, "Fix the contour port configuration before rerunning restore-drill."),
		})
		return info, wrapRestoreDrillError(err)
	}

	info.DrillAppPort = req.DrillAppPort
	info.DrillWSPort = req.DrillWSPort
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
		info.Checks = append(info.Checks, DrillGateCheck{
			Code:    "port_readiness",
			Status:  GateCheckBlocked,
			Summary: failureSummary(err, "Restore-drill port readiness failed"),
			Details: err.Error(),
			Action:  failureAction(err, "Choose a free restore-drill port and rerun the drill."),
		})
		return info, wrapRestoreDrillError(err)
	}
	if err := ensureRestoreDrillPortAvailable(info.DrillWSPort, "websocket"); err != nil {
		info.Checks = append(info.Checks, DrillGateCheck{
			Code:    "port_readiness",
			Status:  GateCheckBlocked,
			Summary: failureSummary(err, "Restore-drill port readiness failed"),
			Details: err.Error(),
			Action:  failureAction(err, "Choose a free restore-drill port and rerun the drill."),
		})
		return info, wrapRestoreDrillError(err)
	}

	info.Checks = append(info.Checks, DrillGateCheck{
		Code:    "port_readiness",
		Status:  GateCheckPassed,
		Summary: "Restore-drill port readiness passed",
		Details: fmt.Sprintf("HTTP %d and websocket %d are available for restore-drill.", info.DrillAppPort, info.DrillWSPort),
	})
	info.Warnings = reporting.DedupeStrings(info.Warnings)

	return info, nil
}

func drillGateWarnings(req DrillGateRequest) []string {
	warnings := drillFlagWarnings(DrillRequest{SkipHTTPProbe: req.SkipHTTPProbe})
	if req.KeepArtifacts {
		warnings = append(warnings, "Restore-drill will preserve temporary contour artifacts because of --keep-artifacts.")
	}
	return warnings
}
