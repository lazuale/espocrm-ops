package doctor

import (
	"fmt"
	"strings"

	platformlocks "github.com/lazuale/espocrm-ops/internal/platform/locks"
)

func checkSharedOperationLock(report *Report) {
	readiness, err := platformlocks.CheckSharedOperationReadiness(report.ProjectDir)
	if err != nil {
		report.fail("", "shared_operation_lock", "Could not inspect the shared operation lock", err.Error(), "Check the filesystem permissions for the temporary lock directory and rerun doctor.")
		return
	}

	switch readiness.State {
	case platformlocks.LockReady:
		report.ok("", "shared_operation_lock", "The shared operation lock is available", readiness.MetadataPath)
	case platformlocks.LockStale:
		report.warn("", "shared_operation_lock", "The shared operation lock metadata is stale", readiness.MetadataPath, "Remove the stale lock metadata after verifying no toolkit operation is still running.")
	case platformlocks.LockActive:
		report.fail("", "shared_operation_lock", "Another toolkit operation is already running", lockOwnerDetails(readiness.MetadataPath, readiness.PID), "Wait for the active toolkit operation to finish before running a stateful command.")
	case platformlocks.LockLegacy:
		report.fail("", "shared_operation_lock", "A legacy shared lock blocks safe readiness checks", lockOwnerDetails(readiness.MetadataPath, readiness.PID), "Remove the legacy lock only after verifying that no toolkit process still owns it.")
	default:
		report.fail("", "shared_operation_lock", "The shared operation lock reported an unknown state", readiness.State, "Inspect the lock files under the system temp directory and rerun doctor.")
	}
}

func checkMaintenanceLock(report *Report, scope, backupRoot string) {
	readiness, err := platformlocks.CheckMaintenanceReadiness(backupRoot)
	if err != nil {
		report.fail(scope, "contour_operation_lock", "Could not inspect the contour operation lock", err.Error(), "Check the backup lock directory permissions and rerun doctor.")
		return
	}

	switch readiness.State {
	case platformlocks.LockReady:
		report.ok(scope, "contour_operation_lock", "The contour operation lock is available", readiness.MetadataPath)
	case platformlocks.LockStale:
		report.warn(scope, "contour_operation_lock", "Found stale contour operation lock metadata", strings.Join(readiness.StalePaths, "; "), "Remove the stale contour operation lock files after verifying that no recovery operation is still running.")
	case platformlocks.LockActive:
		report.fail(scope, "contour_operation_lock", "Another recovery operation is already running for this contour", lockOwnerDetails(readiness.MetadataPath, readiness.PID), "Wait for the running recovery operation to finish before starting a new one.")
	case platformlocks.LockLegacy:
		report.fail(scope, "contour_operation_lock", "A legacy contour operation lock blocks safe readiness checks", lockOwnerDetails(readiness.MetadataPath, readiness.PID), "Remove the legacy contour operation lock only after verifying that no toolkit process still owns it.")
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
