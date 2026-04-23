package resultbridge

import (
	"fmt"
	"io"
	"strings"

	"github.com/lazuale/espocrm-ops/internal/contract/result"
	"github.com/lazuale/espocrm-ops/internal/model"
)

func BackupResult(info model.BackupResult) result.Result {
	message := "backup completed"
	if !info.Details.Ready {
		message = "backup failed"
	}

	items := make([]result.ItemPayload, 0, len(info.Items))
	for _, step := range info.Items {
		items = append(items, result.BackupItem{
			SectionItem: result.SectionItem{
				Code:    step.Code,
				Status:  step.Status,
				Summary: backupStepSummary(step.Code, step.Status),
			},
		})
	}

	return result.Result{
		Command: "backup",
		OK:      info.OK,
		Message: message,
		Details: result.BackupDetails{
			Scope:                  info.Details.Scope,
			Ready:                  info.Details.Ready,
			CreatedAt:              info.Details.CreatedAt,
			Steps:                  info.Details.Steps,
			Completed:              info.Details.Completed,
			Skipped:                info.Details.Skipped,
			Blocked:                info.Details.Blocked,
			Failed:                 info.Details.Failed,
			SkipDB:                 info.Details.SkipDB,
			SkipFiles:              info.Details.SkipFiles,
			NoStop:                 info.Details.NoStop,
			ConsistentSnapshot:     info.Details.ConsistentSnapshot,
			AppServicesWereRunning: info.Details.AppServicesWereRunning,
			RetentionDays:          info.Details.RetentionDays,
			Warnings:               len(info.Warnings),
		},
		Artifacts: result.BackupArtifacts{
			ProjectDir:    info.Artifacts.ProjectDir,
			ComposeFile:   info.Artifacts.ComposeFile,
			EnvFile:       info.Artifacts.EnvFile,
			BackupRoot:    info.Artifacts.BackupRoot,
			ManifestTXT:   info.Artifacts.ManifestText,
			ManifestJSON:  info.Artifacts.ManifestJSON,
			DBBackup:      info.Artifacts.DBBackup,
			FilesBackup:   info.Artifacts.FilesBackup,
			DBChecksum:    info.Artifacts.DBChecksum,
			FilesChecksum: info.Artifacts.FilesChecksum,
		},
		Warnings: append([]string(nil), info.Warnings...),
		Items:    items,
	}
}

func backupStepSummary(code, status string) string {
	if status == "skipped" {
		switch code {
		case "runtime_prepare":
			return "Подготовка runtime пропущена"
		case "db_backup":
			return "Backup базы данных пропущен"
		case "files_backup":
			return "Backup файлов пропущен"
		case "runtime_return":
			return "Возврат runtime пропущен"
		}
	}

	switch code {
	case "artifact_allocation":
		return "Artifacts подготовлены"
	case "runtime_prepare":
		return "Runtime подготовлен"
	case "db_backup":
		return "Backup базы данных создан"
	case "files_backup":
		return "Backup файлов создан"
	case "finalize":
		return "Artifacts финализированы"
	case "retention":
		return "Retention выполнен"
	case "runtime_return":
		return "Runtime возвращён"
	default:
		return code
	}
}

func RenderBackupText(w io.Writer, res result.Result) error {
	details, ok := res.Details.(result.BackupDetails)
	if !ok {
		return result.Render(w, res, false)
	}

	artifacts, ok := res.Artifacts.(result.BackupArtifacts)
	if !ok {
		return fmt.Errorf("unexpected backup artifacts type %T", res.Artifacts)
	}

	if _, err := fmt.Fprintln(w, "EspoCRM backup"); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "Contour: %s\n", details.Scope); err != nil {
		return err
	}
	if strings.TrimSpace(artifacts.EnvFile) != "" {
		if _, err := fmt.Fprintf(w, "Env file: %s\n", artifacts.EnvFile); err != nil {
			return err
		}
	}
	if strings.TrimSpace(artifacts.BackupRoot) != "" {
		if _, err := fmt.Fprintf(w, "Backup root: %s\n", artifacts.BackupRoot); err != nil {
			return err
		}
	}
	if strings.TrimSpace(details.CreatedAt) != "" {
		if _, err := fmt.Fprintf(w, "Created at: %s\n", details.CreatedAt); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintln(w, "Summary:"); err != nil {
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

	if _, err := fmt.Fprintln(w, "\nArtifacts:"); err != nil {
		return err
	}
	if strings.TrimSpace(artifacts.DBBackup) != "" {
		if _, err := fmt.Fprintf(w, "  DB backup:    %s\n", artifacts.DBBackup); err != nil {
			return err
		}
	}
	if strings.TrimSpace(artifacts.FilesBackup) != "" {
		if _, err := fmt.Fprintf(w, "  Files backup: %s\n", artifacts.FilesBackup); err != nil {
			return err
		}
	}
	if strings.TrimSpace(artifacts.DBChecksum) != "" {
		if _, err := fmt.Fprintf(w, "  DB checksum:  %s\n", artifacts.DBChecksum); err != nil {
			return err
		}
	}
	if strings.TrimSpace(artifacts.FilesChecksum) != "" {
		if _, err := fmt.Fprintf(w, "  Files checksum: %s\n", artifacts.FilesChecksum); err != nil {
			return err
		}
	}
	if strings.TrimSpace(artifacts.ManifestTXT) != "" {
		if _, err := fmt.Fprintf(w, "  Text manifest: %s\n", artifacts.ManifestTXT); err != nil {
			return err
		}
	}
	if strings.TrimSpace(artifacts.ManifestJSON) != "" {
		if _, err := fmt.Fprintf(w, "  JSON manifest: %s\n", artifacts.ManifestJSON); err != nil {
			return err
		}
	}

	return renderStepItemsBlock(w, res.Items, backupExecutionItem, stepRenderOptions{
		Title:      "Steps",
		StatusText: upperStatusText,
	})
}

func backupExecutionItem(raw result.ItemPayload) (result.SectionItem, error) {
	item, ok := raw.(result.BackupItem)
	if !ok {
		return result.SectionItem{}, fmt.Errorf("unexpected backup item type %T", raw)
	}

	return item.SectionItem, nil
}
