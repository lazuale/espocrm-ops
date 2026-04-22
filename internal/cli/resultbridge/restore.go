package resultbridge

import (
	"fmt"
	"io"
	"strings"

	restoreusecase "github.com/lazuale/espocrm-ops/internal/app/restore"
	"github.com/lazuale/espocrm-ops/internal/contract/result"
)

func RestoreResult(info restoreusecase.ExecuteInfo) result.Result {
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
