package resultbridge

import (
	"fmt"
	"io"

	backupverifyapp "github.com/lazuale/espocrm-ops/internal/app/backupverify"
	"github.com/lazuale/espocrm-ops/internal/contract/result"
)

func BackupVerifyPendingResult(manifestPath string) result.Result {
	return result.Result{
		Artifacts: result.BackupVerifyArtifacts{
			Manifest: manifestPath,
		},
	}
}

func BackupVerifyResult(report backupverifyapp.Report) result.Result {
	return result.Result{
		Command: "backup verify",
		OK:      true,
		Message: "backup verification passed",
		Artifacts: result.BackupVerifyArtifacts{
			Manifest:    report.ManifestPath,
			DBBackup:    report.DBBackupPath,
			FilesBackup: report.FilesPath,
		},
		Details: result.BackupVerifyDetails{
			Scope:     report.Scope,
			CreatedAt: report.CreatedAt,
		},
	}
}

func RenderBackupVerifyText(w io.Writer, res result.Result) error {
	artifacts, ok := res.Artifacts.(result.BackupVerifyArtifacts)
	if !ok {
		return result.Render(w, res, false)
	}

	if _, err := fmt.Fprintln(w, "Verifying backup set"); err != nil {
		return err
	}
	if artifacts.DBBackup != "" {
		if _, err := fmt.Fprintf(w, "Database backup: %s\n", artifacts.DBBackup); err != nil {
			return err
		}
	}
	if artifacts.FilesBackup != "" {
		if _, err := fmt.Fprintf(w, "Files backup: %s\n", artifacts.FilesBackup); err != nil {
			return err
		}
	}
	if artifacts.Manifest != "" {
		if _, err := fmt.Fprintf(w, "JSON manifest: %s\n", artifacts.Manifest); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintln(w, "[ok] Backup set: manifest, checksums, and archive readability confirmed"); err != nil {
		return err
	}
	_, err := fmt.Fprintln(w, "Backup-file verification completed successfully")
	return err
}
