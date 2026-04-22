package resultbridge

import (
	"fmt"
	"io"
	"strings"

	migrateusecase "github.com/lazuale/espocrm-ops/internal/app/migrate"
	"github.com/lazuale/espocrm-ops/internal/contract/result"
)

func MigrateResult(info migrateusecase.ExecuteInfo) result.Result {
	completed, skipped, blocked, failed := info.Counts()
	message := "backup migration completed"
	if !info.Ready() {
		message = "backup migration failed"
	}

	items := make([]result.ItemPayload, 0, len(info.Steps))
	for _, step := range info.Steps {
		items = append(items, result.MigrateItem{
			SectionItem: newSectionItem(step.Code, step.Status, step.Summary, step.Details, step.Action),
		})
	}

	return result.Result{
		Command:  "migrate",
		OK:       info.Ready(),
		Message:  message,
		Warnings: append([]string(nil), info.Warnings...),
		Details: result.MigrateDetails{
			SourceScope:            info.SourceScope,
			TargetScope:            info.TargetScope,
			Ready:                  info.Ready(),
			SelectionMode:          info.SelectionMode,
			RequestedSelectionMode: info.RequestedSelectionMode,
			Steps:                  len(info.Steps),
			Completed:              completed,
			Skipped:                skipped,
			Blocked:                blocked,
			Failed:                 failed,
			Warnings:               len(info.Warnings),
			SkipDB:                 info.SkipDB,
			SkipFiles:              info.SkipFiles,
			NoStart:                info.NoStart,
			StartedDBTemporarily:   info.StartedDBTemporarily,
		},
		Artifacts: result.MigrateArtifacts{
			ProjectDir:           info.ProjectDir,
			ComposeFile:          info.ComposeFile,
			SourceEnvFile:        info.SourceEnvFile,
			TargetEnvFile:        info.TargetEnvFile,
			SourceBackupRoot:     info.SourceBackupRoot,
			TargetBackupRoot:     info.TargetBackupRoot,
			RequestedDBBackup:    info.RequestedDBBackup,
			RequestedFilesBackup: info.RequestedFilesBackup,
			SelectedPrefix:       info.SelectedPrefix,
			SelectedStamp:        info.SelectedStamp,
			ManifestTXT:          info.ManifestTXTPath,
			ManifestJSON:         info.ManifestJSONPath,
			DBBackup:             info.DBBackupPath,
			FilesBackup:          info.FilesBackupPath,
		},
		Items: items,
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

	if _, err := fmt.Fprintln(w, "EspoCRM backup migrate"); err != nil {
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
