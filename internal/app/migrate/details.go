package migrate

import (
	"errors"
	"fmt"
	"strings"

	platformconfig "github.com/lazuale/espocrm-ops/internal/platform/config"
	maintenanceusecase "github.com/lazuale/espocrm-ops/internal/app/operation"
)

func sourceSelectionSummary(selection sourceSelection) string {
	switch selection.SelectionMode {
	case "explicit_pair":
		return "Explicit source backup pair selection completed"
	case "paired_from_db":
		return "Source backup pairing from the explicit database backup completed"
	case "paired_from_files":
		return "Source backup pairing from the explicit files backup completed"
	case "explicit_db_only":
		return "Explicit database backup selection completed"
	case "explicit_files_only":
		return "Explicit files backup selection completed"
	case "auto_latest_db":
		return "Automatic database backup selection completed"
	case "auto_latest_files":
		return "Automatic files backup selection completed"
	default:
		return "Automatic source backup selection completed"
	}
}

func sourceSelectionDetails(selection sourceSelection) string {
	switch selection.SelectionMode {
	case "explicit_db_only", "auto_latest_db":
		return fmt.Sprintf("Selected database backup %s.", selection.DBBackup)
	case "explicit_files_only", "auto_latest_files":
		return fmt.Sprintf("Selected files backup %s.", selection.FilesBackup)
	case "explicit_pair", "paired_from_db", "paired_from_files":
		details := fmt.Sprintf("Selected DB backup %s and files backup %s.", selection.DBBackup, selection.FilesBackup)
		if strings.TrimSpace(selection.ManifestJSON) != "" {
			details += fmt.Sprintf(" Matching manifest %s is available for the same backup set.", selection.ManifestJSON)
		}
		return details
	default:
		details := fmt.Sprintf("Selected prefix %s at %s with DB backup %s and files backup %s.", selection.Prefix, selection.Stamp, selection.DBBackup, selection.FilesBackup)
		if strings.TrimSpace(selection.ManifestJSON) != "" {
			details += fmt.Sprintf(" Matching manifest %s is available for the same backup set.", selection.ManifestJSON)
		}
		return details
	}
}

func runtimePrepareDetails(info runtimePrepareInfo) string {
	dbMode := "The target db service was already running and ready."
	if info.StartedDBTemporarily {
		dbMode = "The target db service was started temporarily and confirmed ready."
	}
	appMode := "Target application services were already stopped."
	if len(info.StoppedAppServices) != 0 {
		appMode = fmt.Sprintf("Stopped the target application services: %s.", strings.Join(info.StoppedAppServices, ", "))
	}

	return dbMode + " " + appMode
}

func dbRestoreDetails(ctx maintenanceusecase.OperationContext, info ExecuteInfo) string {
	return fmt.Sprintf("Restored database %s for target contour %s from %s.", strings.TrimSpace(ctx.Env.Value("DB_NAME")), info.TargetScope, migrateSourceLabel(info))
}

func filesRestoreDetails(ctx maintenanceusecase.OperationContext, info ExecuteInfo) string {
	targetDir := platformconfig.ResolveProjectPath(ctx.ProjectDir, ctx.Env.ESPOStorageDir())
	return fmt.Sprintf("Restored %s for target contour %s from %s.", targetDir, info.TargetScope, migrateSourceLabel(info))
}

func migrateSourceLabel(info ExecuteInfo) string {
	if strings.TrimSpace(info.ManifestJSONPath) != "" {
		return info.ManifestJSONPath
	}
	switch {
	case strings.TrimSpace(info.DBBackupPath) != "" && strings.TrimSpace(info.FilesBackupPath) != "":
		return fmt.Sprintf("%s and %s", info.DBBackupPath, info.FilesBackupPath)
	case strings.TrimSpace(info.DBBackupPath) != "":
		return info.DBBackupPath
	default:
		return info.FilesBackupPath
	}
}

func requestedSelectionMode(req ExecuteRequest) string {
	dbBackup := strings.TrimSpace(req.DBBackup)
	filesBackup := strings.TrimSpace(req.FilesBackup)

	switch {
	case req.SkipDB:
		if filesBackup != "" {
			return "explicit_files_only"
		}
		return "auto_latest_files"
	case req.SkipFiles:
		if dbBackup != "" {
			return "explicit_db_only"
		}
		return "auto_latest_db"
	case dbBackup == "" && filesBackup == "":
		return "auto_latest_complete"
	case dbBackup != "" && filesBackup != "":
		return "explicit_pair"
	case dbBackup != "":
		return "paired_from_db"
	default:
		return "paired_from_files"
	}
}

func flagWarnings(req ExecuteRequest) []string {
	warnings := []string{}
	if req.SkipDB {
		warnings = append(warnings, "Backup migration will skip the database restore because of --skip-db.")
	}
	if req.SkipFiles {
		warnings = append(warnings, "Backup migration will skip the files restore because of --skip-files.")
	}
	if req.NoStart {
		warnings = append(warnings, "Backup migration will leave the target application services stopped because of --no-start.")
	}

	return warnings
}

func notRunMigrateStep(code, summary string) ExecuteStep {
	return ExecuteStep{
		Code:    code,
		Status:  MigrateStepStatusNotRun,
		Summary: summary,
	}
}

func failureSummary(err error, fallback string) string {
	var failure executeFailure
	if errors.As(err, &failure) && strings.TrimSpace(failure.Summary) != "" {
		return failure.Summary
	}

	return fallback
}

func failureAction(err error, fallback string) string {
	var failure executeFailure
	if errors.As(err, &failure) && strings.TrimSpace(failure.Action) != "" {
		return failure.Action
	}

	return fallback
}
