package resultbridge

import (
	"fmt"
	"io"
	"strings"

	backupusecase "github.com/lazuale/espocrm-ops/internal/app/backup"
	"github.com/lazuale/espocrm-ops/internal/contract/result"
)

func BackupResult(info backupusecase.ExecuteInfo) result.Result {
	completed, skipped, blocked, failed := info.Counts()
	message := "backup completed"
	if !info.Ready() {
		message = "backup failed"
	}

	items := make([]result.ItemPayload, 0, len(info.Steps))
	for _, step := range info.Steps {
		items = append(items, result.BackupItem{
			SectionItem: newSectionItem(step.Code, step.Status, step.Summary, step.Details, step.Action),
		})
	}

	return result.Result{
		Command:  "backup",
		OK:       info.Ready(),
		Message:  message,
		Warnings: append([]string(nil), info.Warnings...),
		Details: result.BackupDetails{
			Scope:                  info.Scope,
			Ready:                  info.Ready(),
			CreatedAt:              info.CreatedAt,
			Steps:                  len(info.Steps),
			Completed:              completed,
			Skipped:                skipped,
			Blocked:                blocked,
			Failed:                 failed,
			Warnings:               len(info.Warnings),
			SkipDB:                 info.SkipDB,
			SkipFiles:              info.SkipFiles,
			NoStop:                 info.NoStop,
			ConsistentSnapshot:     info.ConsistentSnapshot,
			AppServicesWereRunning: info.AppServicesWereRunning,
			RetentionDays:          info.RetentionDays,
		},
		Artifacts: result.BackupArtifacts{
			ProjectDir:    info.ProjectDir,
			ComposeFile:   info.ComposeFile,
			EnvFile:       info.EnvFile,
			BackupRoot:    info.BackupRoot,
			ManifestTXT:   info.ManifestTXTPath,
			ManifestJSON:  info.ManifestJSONPath,
			DBBackup:      info.DBBackupPath,
			FilesBackup:   info.FilesBackupPath,
			DBChecksum:    info.DBSidecarPath,
			FilesChecksum: info.FilesSidecarPath,
		},
		Items: items,
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
