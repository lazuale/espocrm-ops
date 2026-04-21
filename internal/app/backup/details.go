package backup

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	domainfailure "github.com/lazuale/espocrm-ops/internal/domain/failure"
	domainworkflow "github.com/lazuale/espocrm-ops/internal/domain/workflow"
)

func allocationDetails(state backupExecutionState, info ExecuteInfo) string {
	details := []string{
		fmt.Sprintf("Reserved backup set for %s at %s.", info.Scope, state.createdAt.UTC().Format(time.RFC3339)),
		fmt.Sprintf("Text manifest: %s.", info.ManifestTXTPath),
		fmt.Sprintf("JSON manifest: %s.", info.ManifestJSONPath),
	}
	if strings.TrimSpace(info.DBBackupPath) != "" {
		details = append(details, fmt.Sprintf("Database backup: %s.", info.DBBackupPath))
	}
	if strings.TrimSpace(info.FilesBackupPath) != "" {
		details = append(details, fmt.Sprintf("Files backup: %s.", info.FilesBackupPath))
	}

	return strings.Join(details, " ")
}

func runtimePrepareDetails(info runtimePrepareInfo) string {
	if len(info.StoppedAppServices) == 0 {
		return "Application services were already stopped before backup."
	}

	return fmt.Sprintf("Stopped application services before backup: %s.", strings.Join(info.StoppedAppServices, ", "))
}

func runtimePrepareSkippedDetails(req PreparedRequest) string {
	return "Application services remained running because of --no-stop."
}

func dbBackupDetails(state backupExecutionState) string {
	return fmt.Sprintf("Created compressed database dump %s.", state.set.DBBackup.Path)
}

func filesBackupDetails(state backupExecutionState, archiveInfo filesArchiveInfo) string {
	details := fmt.Sprintf("Created application files archive %s.", state.set.FilesBackup.Path)
	if archiveInfo.UsedDockerHelper {
		details += " The Docker helper fallback produced the archive after local archiving failed."
	}

	return details
}

func finalizeDetails(info ExecuteInfo) string {
	details := []string{
		fmt.Sprintf("Wrote text manifest %s.", info.ManifestTXTPath),
		fmt.Sprintf("Wrote JSON manifest %s.", info.ManifestJSONPath),
	}
	if strings.TrimSpace(info.DBSidecarPath) != "" {
		details = append(details, fmt.Sprintf("Database checksum sidecar: %s.", info.DBSidecarPath))
	}
	if strings.TrimSpace(info.FilesSidecarPath) != "" {
		details = append(details, fmt.Sprintf("Files checksum sidecar: %s.", info.FilesSidecarPath))
	}

	return strings.Join(details, " ")
}

func retentionDetails(root string, retentionDays int) string {
	return fmt.Sprintf("Pruned backup artifacts older than %d days under %s.", retentionDays, filepath.Clean(root))
}

func runtimeReturnDetails(info runtimeReturnInfo) string {
	if len(info.RestartedAppServices) == 0 {
		return "Application services were already in the requested post-backup state."
	}

	return fmt.Sprintf("Restarted application services after backup: %s.", strings.Join(info.RestartedAppServices, ", "))
}

func runtimeReturnSkippedDetails(req PreparedRequest, prep runtimePrepareInfo) string {
	switch {
	case req.NoStop:
		return "Application services remained running because of --no-stop."
	case len(prep.StoppedAppServices) == 0:
		return "Application services were already stopped before backup."
	default:
		return "The contour runtime already matched the requested post-backup state."
	}
}

func flagWarnings(req PreparedRequest) []string {
	warnings := []string{}
	if req.NoStop {
		warnings = append(warnings, "Backup will run without stopping application services because of --no-stop.")
	}
	if req.SkipDB {
		warnings = append(warnings, "Backup will skip the database backup because of --skip-db.")
	}
	if req.SkipFiles {
		warnings = append(warnings, "Backup will skip the files backup because of --skip-files.")
	}

	return warnings
}

func notRunBackupStep(code, summary string) domainworkflow.Step {
	return domainworkflow.NewStep(code, domainworkflow.StatusNotRun, summary, "", "")
}

func failureSummary(err error, fallback string) string {
	var failure domainfailure.Failure
	if errors.As(err, &failure) && strings.TrimSpace(failure.Summary) != "" {
		return failure.Summary
	}

	return fallback
}

func failureAction(err error, fallback string) string {
	var failure domainfailure.Failure
	if errors.As(err, &failure) && strings.TrimSpace(failure.Action) != "" {
		return failure.Action
	}

	return fallback
}
