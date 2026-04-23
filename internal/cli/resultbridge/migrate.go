package resultbridge

import (
	"fmt"
	"io"
	"strings"

	"github.com/lazuale/espocrm-ops/internal/contract/result"
	"github.com/lazuale/espocrm-ops/internal/model"
)

func MigrateResult(info model.MigrateResult) result.Result {
	items := make([]result.ItemPayload, 0, len(info.Items))
	for _, step := range info.Items {
		items = append(items, result.MigrateItem{
			SectionItem: result.SectionItem{
				Code:    step.Code,
				Status:  step.Status,
				Summary: migrateStepSummary(step.Code, step.Status),
			},
		})
	}

	return result.Result{
		Command:  "migrate",
		OK:       info.OK,
		Message:  migrateMessage(info.OK),
		Warnings: append([]string(nil), info.Warnings...),
		Details: result.MigrateDetails{
			SourceScope:            info.Details.SourceScope,
			TargetScope:            info.Details.TargetScope,
			Ready:                  info.Details.Ready,
			SelectionMode:          info.Details.SelectionMode,
			SourceKind:             info.Details.SourceKind,
			SnapshotEnabled:        info.Details.SnapshotEnabled,
			Steps:                  info.Details.Steps,
			Planned:                info.Details.Planned,
			Completed:              info.Details.Completed,
			Skipped:                info.Details.Skipped,
			Blocked:                info.Details.Blocked,
			Failed:                 info.Details.Failed,
			Warnings:               info.Details.Warnings,
			SkipDB:                 info.Details.SkipDB,
			SkipFiles:              info.Details.SkipFiles,
			NoStart:                info.Details.NoStart,
			AppServicesWereRunning: info.Details.AppServicesWereRunning,
			StartedDBTemporarily:   info.Details.StartedDBTemporarily,
		},
		Artifacts: result.MigrateArtifacts{
			ProjectDir:            info.Artifacts.ProjectDir,
			ComposeFile:           info.Artifacts.ComposeFile,
			SourceEnvFile:         info.Artifacts.SourceEnvFile,
			TargetEnvFile:         info.Artifacts.TargetEnvFile,
			SourceBackupRoot:      info.Artifacts.SourceBackupRoot,
			TargetBackupRoot:      info.Artifacts.TargetBackupRoot,
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

func migrateMessage(ok bool) string {
	if ok {
		return "migrate завершён"
	}
	return "migrate завершился ошибкой"
}

func migrateStepSummary(code, status string) string {
	if status == model.StatusBlocked {
		switch code {
		case model.MigrateStepCompatibility:
			return "Compatibility migrate заблокирована"
		case model.MigrateStepTargetSnapshot:
			return "Target snapshot заблокирован"
		case model.StepRuntimePrepare:
			return "Подготовка runtime заблокирована"
		case model.MigrateStepDBApply:
			return "Database migrate заблокирован"
		case model.MigrateStepFilesApply:
			return "Files migrate заблокирован"
		case model.MigrateStepPermission:
			return "Согласование прав заблокировано"
		case model.StepRuntimeReturn:
			return "Возврат runtime заблокирован"
		case model.MigrateStepPostCheck:
			return "Пост-проверка migrate заблокирована"
		}
	}
	if status == model.StatusSkipped {
		switch code {
		case model.MigrateStepTargetSnapshot:
			return "Target snapshot пропущен"
		case model.MigrateStepDBApply:
			return "Database migrate пропущен"
		case model.MigrateStepFilesApply:
			return "Files migrate пропущен"
		case model.MigrateStepPermission:
			return "Согласование прав пропущено"
		case model.StepRuntimeReturn:
			return "Возврат runtime пропущен"
		}
	}
	if status == model.StatusFailed {
		switch code {
		case model.MigrateStepSourceSelection:
			return "Источник migrate не прошёл проверку"
		case model.MigrateStepCompatibility:
			return "Compatibility migrate завершилась ошибкой"
		case model.MigrateStepTargetSnapshot:
			return "Target snapshot завершился ошибкой"
		case model.StepRuntimePrepare:
			return "Подготовка runtime завершилась ошибкой"
		case model.MigrateStepDBApply:
			return "Database migrate завершился ошибкой"
		case model.MigrateStepFilesApply:
			return "Files migrate завершился ошибкой"
		case model.MigrateStepPermission:
			return "Согласование прав завершилось ошибкой"
		case model.StepRuntimeReturn:
			return "Возврат runtime завершился ошибкой"
		case model.MigrateStepPostCheck:
			return "Пост-проверка migrate завершилась ошибкой"
		}
	}

	switch code {
	case model.MigrateStepSourceSelection:
		return "Источник migrate выбран"
	case model.MigrateStepCompatibility:
		return "Compatibility migrate подтверждена"
	case model.MigrateStepTargetSnapshot:
		return "Target snapshot создан"
	case model.StepRuntimePrepare:
		return "Runtime подготовлен"
	case model.MigrateStepDBApply:
		return "База данных перенесена"
	case model.MigrateStepFilesApply:
		return "Файлы перенесены"
	case model.MigrateStepPermission:
		return "Права согласованы"
	case model.StepRuntimeReturn:
		return "Runtime возвращён"
	case model.MigrateStepPostCheck:
		return "Пост-проверка migrate выполнена"
	default:
		return code
	}
}

func RenderMigrateText(w io.Writer, res result.Result) error {
	details, ok := res.Details.(result.MigrateDetails)
	if !ok {
		return result.Render(w, res, false)
	}

	artifacts, ok := res.Artifacts.(result.MigrateArtifacts)
	if !ok {
		return fmt.Errorf("unexpected migrate artifacts type %T", res.Artifacts)
	}

	if _, err := fmt.Fprintln(w, "EspoCRM migrate"); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "Source contour: %s\n", details.SourceScope); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "Source env file: %s\n", artifacts.SourceEnvFile); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "Target contour: %s\n", details.TargetScope); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "Target env file: %s\n", artifacts.TargetEnvFile); err != nil {
		return err
	}
	if strings.TrimSpace(details.SelectionMode) != "" {
		if _, err := fmt.Fprintf(w, "Selection: %s\n", details.SelectionMode); err != nil {
			return err
		}
	}
	if strings.TrimSpace(details.SourceKind) != "" {
		if _, err := fmt.Fprintf(w, "Source kind: %s\n", details.SourceKind); err != nil {
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
	if strings.TrimSpace(artifacts.SnapshotManifestJSON) != "" {
		if _, err := fmt.Fprintf(w, "Target snapshot manifest: %s\n", artifacts.SnapshotManifestJSON); err != nil {
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
	if _, err := fmt.Fprintf(w, "  Warnings:     %d\n", details.Warnings); err != nil {
		return err
	}

	return renderStepItemsBlock(w, res.Items, migrateExecutionItem, stepRenderOptions{
		Title:      "Steps",
		StatusText: upperStatusText,
	})
}
