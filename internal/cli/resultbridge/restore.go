package resultbridge

import (
	"fmt"
	"io"
	"strings"

	restoreusecase "github.com/lazuale/espocrm-ops/internal/app/restore"
	"github.com/lazuale/espocrm-ops/internal/contract/result"
	"github.com/lazuale/espocrm-ops/internal/model"
)

func RestoreResult(raw any) result.Result {
	switch info := raw.(type) {
	case model.RestoreResult:
		return restoreV2Result(info)
	case restoreusecase.ExecuteInfo:
		return restoreLegacyResult(info)
	default:
		return result.Result{
			Command: "restore",
			OK:      false,
			Error: &result.ErrorInfo{
				Code:    "internal_error",
				Message: fmt.Sprintf("unexpected restore result type %T", raw),
			},
		}
	}
}

func restoreV2Result(info model.RestoreResult) result.Result {
	items := make([]result.ItemPayload, 0, len(info.Items))
	for _, step := range info.Items {
		items = append(items, result.RestoreItem{
			SectionItem: result.SectionItem{
				Code:    step.Code,
				Status:  step.Status,
				Summary: restoreStepSummary(step.Code, step.Status),
			},
		})
	}

	return result.Result{
		Command:  "restore",
		OK:       info.OK,
		Message:  restoreMessage(info.OK),
		Warnings: append([]string(nil), info.Warnings...),
		Details: result.RestoreDetails{
			Scope:                  info.Details.Scope,
			Ready:                  info.Details.Ready,
			SelectionMode:          info.Details.SelectionMode,
			SourceKind:             info.Details.SourceKind,
			Steps:                  info.Details.Steps,
			Completed:              info.Details.Completed,
			Skipped:                info.Details.Skipped,
			Blocked:                info.Details.Blocked,
			Failed:                 info.Details.Failed,
			Warnings:               info.Details.Warnings,
			SnapshotEnabled:        info.Details.SnapshotEnabled,
			SkipDB:                 info.Details.SkipDB,
			SkipFiles:              info.Details.SkipFiles,
			NoStop:                 info.Details.NoStop,
			NoStart:                info.Details.NoStart,
			AppServicesWereRunning: info.Details.AppServicesWereRunning,
			StartedDBTemporarily:   false,
		},
		Artifacts: result.RestoreArtifacts{
			ProjectDir:            info.Artifacts.ProjectDir,
			ComposeFile:           info.Artifacts.ComposeFile,
			EnvFile:               info.Artifacts.EnvFile,
			BackupRoot:            info.Artifacts.BackupRoot,
			ManifestTXT:           info.Artifacts.ManifestTXT,
			ManifestJSON:          info.Artifacts.ManifestJSON,
			DBBackup:              info.Artifacts.DBBackup,
			FilesBackup:           info.Artifacts.FilesBackup,
			SnapshotManifestTXT:   info.Artifacts.SnapshotManifestTXT,
			SnapshotManifestJSON:  info.Artifacts.SnapshotManifestJSON,
			SnapshotDBBackup:      info.Artifacts.SnapshotDBBackup,
			SnapshotFilesBackup:   info.Artifacts.SnapshotFilesBackup,
			SnapshotDBChecksum:    info.Artifacts.SnapshotDBChecksum,
			SnapshotFilesChecksum: info.Artifacts.SnapshotFilesChecksum,
		},
		Items: items,
	}
}

func restoreLegacyResult(info restoreusecase.ExecuteInfo) result.Result {
	planned, completed, skipped, blocked, failed := info.Counts()
	message := "restore completed"
	if info.DryRun {
		message = "restore dry-run plan completed"
	}
	if !info.Ready() {
		if info.DryRun {
			message = "restore dry-run plan found blocking conditions"
		} else {
			message = "restore failed"
		}
	}

	return result.Result{
		Command:  "restore",
		OK:       info.Ready(),
		Message:  message,
		DryRun:   info.DryRun,
		Warnings: append([]string(nil), info.Warnings...),
		Details: result.RestoreDetails{
			Scope:                  info.Scope,
			Ready:                  info.Ready(),
			SelectionMode:          info.SelectionMode,
			SourceKind:             info.SourceKind,
			Steps:                  len(info.Steps),
			Planned:                planned,
			Completed:              completed,
			Skipped:                skipped,
			Blocked:                blocked,
			Failed:                 failed,
			Warnings:               len(info.Warnings),
			SnapshotEnabled:        info.SnapshotEnabled,
			SkipDB:                 info.SkipDB,
			SkipFiles:              info.SkipFiles,
			NoStop:                 info.NoStop,
			NoStart:                info.NoStart,
			AppServicesWereRunning: info.AppServicesWereRunning,
			StartedDBTemporarily:   info.StartedDBTemporarily,
		},
		Artifacts: result.RestoreArtifacts{
			ProjectDir:            info.ProjectDir,
			ComposeFile:           info.ComposeFile,
			EnvFile:               info.EnvFile,
			BackupRoot:            info.BackupRoot,
			ManifestTXT:           info.ManifestTXTPath,
			ManifestJSON:          info.ManifestJSONPath,
			DBBackup:              info.DBBackupPath,
			FilesBackup:           info.FilesBackupPath,
			SnapshotManifestTXT:   info.SnapshotManifestTXT,
			SnapshotManifestJSON:  info.SnapshotManifestJSON,
			SnapshotDBBackup:      info.SnapshotDBBackup,
			SnapshotFilesBackup:   info.SnapshotFilesBackup,
			SnapshotDBChecksum:    info.SnapshotDBChecksum,
			SnapshotFilesChecksum: info.SnapshotFilesChecksum,
		},
		Items: restoreExecutionItems(info.Steps),
	}
}

func restoreMessage(ok bool) string {
	if ok {
		return "restore completed"
	}
	return "restore failed"
}

func restoreStepSummary(code, status string) string {
	if status == model.StatusBlocked {
		switch code {
		case model.StepRuntimePrepare:
			return "Подготовка runtime заблокирована"
		case model.RestoreStepSnapshot:
			return "Аварийный snapshot заблокирован"
		case model.RestoreStepDBRestore:
			return "Восстановление базы данных заблокировано"
		case model.RestoreStepFilesRestore:
			return "Восстановление файлов заблокировано"
		case model.RestoreStepPermission:
			return "Согласование прав заблокировано"
		case model.StepRuntimeReturn:
			return "Возврат runtime заблокирован"
		case model.RestoreStepPostCheck:
			return "Пост-проверка restore заблокирована"
		}
	}
	if status == model.StatusSkipped {
		switch code {
		case model.RestoreStepSnapshot:
			return "Аварийный snapshot пропущен"
		case model.RestoreStepDBRestore:
			return "Восстановление базы данных пропущено"
		case model.RestoreStepFilesRestore:
			return "Восстановление файлов пропущено"
		case model.RestoreStepPermission:
			return "Согласование прав пропущено"
		case model.StepRuntimeReturn:
			return "Возврат runtime пропущен"
		}
	}
	if status == model.StatusFailed {
		switch code {
		case model.RestoreStepSourceResolution:
			return "Источник restore не прошёл проверку"
		case model.StepRuntimePrepare:
			return "Подготовка runtime завершилась ошибкой"
		case model.RestoreStepSnapshot:
			return "Аварийный snapshot завершился ошибкой"
		case model.RestoreStepDBRestore:
			return "Восстановление базы данных завершилось ошибкой"
		case model.RestoreStepFilesRestore:
			return "Восстановление файлов завершилось ошибкой"
		case model.RestoreStepPermission:
			return "Согласование прав завершилось ошибкой"
		case model.StepRuntimeReturn:
			return "Возврат runtime завершился ошибкой"
		case model.RestoreStepPostCheck:
			return "Пост-проверка restore завершилась ошибкой"
		}
	}

	switch code {
	case model.RestoreStepSourceResolution:
		return "Источник restore выбран"
	case model.StepRuntimePrepare:
		return "Runtime подготовлен"
	case model.RestoreStepSnapshot:
		return "Аварийный snapshot создан"
	case model.RestoreStepDBRestore:
		return "База данных восстановлена"
	case model.RestoreStepFilesRestore:
		return "Файлы восстановлены"
	case model.RestoreStepPermission:
		return "Права согласованы"
	case model.StepRuntimeReturn:
		return "Runtime возвращён"
	case model.RestoreStepPostCheck:
		return "Пост-проверка restore выполнена"
	default:
		return code
	}
}

func RenderRestoreText(w io.Writer, res result.Result) error {
	details, ok := res.Details.(result.RestoreDetails)
	if !ok {
		return result.Render(w, res, false)
	}
	artifacts, ok := res.Artifacts.(result.RestoreArtifacts)
	if !ok {
		return fmt.Errorf("unexpected restore artifacts type %T", res.Artifacts)
	}

	if _, err := fmt.Fprintln(w, "EspoCRM restore"); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "Contour: %s\n", details.Scope); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "Env file: %s\n", artifacts.EnvFile); err != nil {
		return err
	}
	if strings.TrimSpace(details.SelectionMode) != "" {
		if _, err := fmt.Fprintf(w, "Selection: %s\n", details.SelectionMode); err != nil {
			return err
		}
	}
	if strings.TrimSpace(artifacts.ManifestJSON) != "" {
		if _, err := fmt.Fprintf(w, "Manifest JSON: %s\n", artifacts.ManifestJSON); err != nil {
			return err
		}
	}
	if strings.TrimSpace(artifacts.DBBackup) != "" {
		if _, err := fmt.Fprintf(w, "DB backup: %s\n", artifacts.DBBackup); err != nil {
			return err
		}
	}
	if strings.TrimSpace(artifacts.FilesBackup) != "" {
		if _, err := fmt.Fprintf(w, "Files backup: %s\n", artifacts.FilesBackup); err != nil {
			return err
		}
	}

	if _, err := fmt.Fprintln(w, "\nSummary:"); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "  Ready:        %t\n", details.Ready); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "  Steps:        %d\n", details.Steps); err != nil {
		return err
	}
	if details.Planned != 0 || res.DryRun {
		if _, err := fmt.Fprintf(w, "  Planned:      %d\n", details.Planned); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintf(w, "  Completed:    %d\n", details.Completed); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "  Skipped:      %d\n", details.Skipped); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "  Blocked:      %d\n", details.Blocked); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "  Failed:       %d\n", details.Failed); err != nil {
		return err
	}

	if strings.TrimSpace(artifacts.SnapshotManifestJSON) != "" {
		if _, err := fmt.Fprintln(w, "\nEmergency Recovery Point:"); err != nil {
			return err
		}
		if strings.TrimSpace(artifacts.SnapshotManifestJSON) != "" {
			if _, err := fmt.Fprintf(w, "  Manifest JSON: %s\n", artifacts.SnapshotManifestJSON); err != nil {
				return err
			}
		}
		if strings.TrimSpace(artifacts.SnapshotDBBackup) != "" {
			if _, err := fmt.Fprintf(w, "  DB backup:     %s\n", artifacts.SnapshotDBBackup); err != nil {
				return err
			}
		}
		if strings.TrimSpace(artifacts.SnapshotFilesBackup) != "" {
			if _, err := fmt.Fprintf(w, "  Files backup:  %s\n", artifacts.SnapshotFilesBackup); err != nil {
				return err
			}
		}
	}

	if err := renderStepItemsBlock(w, res.Items, restoreExecutionItem, stepRenderOptions{
		Title:            "Steps",
		IgnoreUnexpected: true,
		StatusText:       upperStatusText,
	}); err != nil {
		return err
	}

	return nil
}
