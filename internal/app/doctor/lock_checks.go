package doctor

import (
	"fmt"
	lockport "github.com/lazuale/espocrm-ops/internal/app/ports/lockport"
	"strings"
)

func (s Service) checkSharedOperationLock(report *Report) {
	readiness, err := s.locks.CheckSharedOperationReadiness(report.ProjectDir)
	if err != nil {
		report.fail("", "shared_operation_lock", "Could not inspect the shared operation lock", err.Error(), "Check the filesystem permissions for the temporary lock directory and rerun doctor.")
		return
	}

	switch readiness.State {
	case lockport.Ready:
		report.ok("", "shared_operation_lock", "The shared operation lock is available", readiness.MetadataPath)
	case lockport.Stale:
		report.warn("", "shared_operation_lock", "The shared operation lock metadata is stale", readiness.MetadataPath, "Remove the stale lock metadata after verifying no toolkit operation is still running.")
	case lockport.Active:
		report.fail("", "shared_operation_lock", "Another toolkit operation is already running", lockOwnerDetails(readiness.MetadataPath, readiness.PID), "Wait for the active toolkit operation to finish before running a stateful command.")
	default:
		report.fail("", "shared_operation_lock", "The shared operation lock reported an unknown state", readiness.State, "Inspect the lock files under the system temp directory and rerun doctor.")
	}
}

func (s Service) checkMaintenanceLock(report *Report, scope, backupRoot string) {
	readiness, err := s.locks.CheckMaintenanceReadiness(backupRoot)
	if err != nil {
		report.fail(scope, "contour_operation_lock", "Could not inspect the contour operation lock", err.Error(), "Check the backup lock directory permissions and rerun doctor.")
		return
	}

	switch readiness.State {
	case lockport.Ready:
		report.ok(scope, "contour_operation_lock", "The contour operation lock is available", readiness.MetadataPath)
	case lockport.Stale:
		report.warn(scope, "contour_operation_lock", "Found stale contour operation lock metadata", strings.Join(readiness.StalePaths, "; "), "Remove the stale contour operation lock files after verifying that no recovery operation is still running.")
	case lockport.Active:
		report.fail(scope, "contour_operation_lock", "Another recovery operation is already running for this contour", lockOwnerDetails(readiness.MetadataPath, readiness.PID), "Wait for the running recovery operation to finish before starting a new one.")
	default:
		report.fail(scope, "contour_operation_lock", "The contour operation lock reported an unknown state", readiness.State, "Inspect the contour lock files and rerun doctor.")
	}
}

func lockOwnerDetails(path, pid string) string {
	if strings.TrimSpace(pid) == "" {
		return path
	}

	return fmt.Sprintf("%s (PID %s)", path, pid)
}
