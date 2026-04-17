package cli

import (
	"fmt"
	"io"
	"strings"

	"github.com/lazuale/espocrm-ops/internal/contract/exitcode"
	"github.com/lazuale/espocrm-ops/internal/contract/result"
	"github.com/lazuale/espocrm-ops/internal/usecase/backup"
	"github.com/spf13/cobra"
)

func newBackupAuditCmd() *cobra.Command {
	var backupRoot string
	var skipDB bool
	var skipFiles bool
	var noVerifyChecksum bool
	var dbMaxAgeHours int
	var filesMaxAgeHours int

	cmd := &cobra.Command{
		Use:   "backup-audit",
		Short: "Audit the latest relevant backup set",
		Args:  noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			app := appForCommand(cmd)
			in := backupAuditInput{
				backupRoot:       backupRoot,
				skipDB:           skipDB,
				skipFiles:        skipFiles,
				verifyChecksum:   !noVerifyChecksum,
				dbMaxAgeHours:    dbMaxAgeHours,
				filesMaxAgeHours: filesMaxAgeHours,
			}
			if err := validateBackupAuditInput(&in); err != nil {
				return err
			}

			return RunCommand(cmd, CommandSpec{
				Name:       "backup-audit",
				ErrorCode:  "backup_audit_failed",
				ExitCode:   exitcode.ValidationError,
				RenderText: renderBackupAuditText,
			}, func() (result.Result, error) {
				info, err := backup.Audit(backup.AuditRequest{
					BackupRoot:       in.backupRoot,
					SkipDB:           in.skipDB,
					SkipFiles:        in.skipFiles,
					VerifyChecksum:   in.verifyChecksum,
					DBMaxAgeHours:    in.dbMaxAgeHours,
					FilesMaxAgeHours: in.filesMaxAgeHours,
					Now:              app.runtime.Now(),
				})
				if err != nil {
					return result.Result{}, err
				}

				message := "backup audit passed"
				if !info.Success {
					message = "backup audit completed with failures"
				}

				return result.Result{
					Message: message,
					Details: result.BackupAuditDetails{
						BackupRoot:          info.BackupRoot,
						Success:             info.Success,
						VerifyChecksum:      info.VerifyChecksum,
						SelectedPrefix:      info.SelectedSet.Prefix,
						SelectedStamp:       info.SelectedSet.Stamp,
						DBMaxAgeHours:       info.Thresholds.DBMaxAgeHours,
						FilesMaxAgeHours:    info.Thresholds.FilesMaxAgeHours,
						ManifestMaxAgeHours: info.Thresholds.ManifestMaxAgeHours,
						FailureFindings:     backupAuditFindingCount(info.Findings, backup.AuditStatusFail),
						NonFailureFindings:  len(info.Findings) - backupAuditFindingCount(info.Findings, backup.AuditStatusFail),
					},
					Artifacts: backupAuditArtifacts{
						DBBackup:     info.DBBackup,
						FilesBackup:  info.FilesBackup,
						ManifestJSON: info.ManifestJSON,
						ManifestTXT:  info.ManifestTXT,
					},
					Items: backupAuditFindings(info.Findings),
				}, nil
			})
		},
	}

	cmd.Flags().StringVar(&backupRoot, "backup-root", "", "backup root containing db, files and manifests directories")
	cmd.Flags().BoolVar(&skipDB, "skip-db", false, "skip db backup audit")
	cmd.Flags().BoolVar(&skipFiles, "skip-files", false, "skip files backup audit")
	cmd.Flags().BoolVar(&noVerifyChecksum, "no-verify-checksum", false, "skip checksum verification")
	cmd.Flags().IntVar(&dbMaxAgeHours, "max-db-age-hours", 48, "maximum db backup age in hours")
	cmd.Flags().IntVar(&filesMaxAgeHours, "max-files-age-hours", 48, "maximum files backup age in hours")

	return cmd
}

type backupAuditInput struct {
	backupRoot       string
	skipDB           bool
	skipFiles        bool
	verifyChecksum   bool
	dbMaxAgeHours    int
	filesMaxAgeHours int
}

type backupAuditArtifacts struct {
	DBBackup     backup.AuditComponent `json:"db_backup"`
	FilesBackup  backup.AuditComponent `json:"files_backup"`
	ManifestJSON backup.AuditComponent `json:"manifest_json"`
	ManifestTXT  backup.AuditComponent `json:"manifest_txt"`
}

func validateBackupAuditInput(in *backupAuditInput) error {
	in.backupRoot = strings.TrimSpace(in.backupRoot)
	if err := requireNonBlankFlag("--backup-root", in.backupRoot); err != nil {
		return err
	}
	if in.skipDB && in.skipFiles {
		return usageError(fmt.Errorf("nothing to audit: both --skip-db and --skip-files are set"))
	}
	if in.dbMaxAgeHours < 0 {
		return usageError(fmt.Errorf("--max-db-age-hours must be non-negative"))
	}
	if in.filesMaxAgeHours < 0 {
		return usageError(fmt.Errorf("--max-files-age-hours must be non-negative"))
	}

	return nil
}

func backupAuditFindings(findings []backup.AuditFinding) []any {
	out := make([]any, 0, len(findings))
	for _, finding := range findings {
		out = append(out, finding)
	}

	return out
}

func backupAuditFindingCount(findings []backup.AuditFinding, level string) int {
	count := 0
	for _, finding := range findings {
		if finding.Level == level {
			count++
		}
	}

	return count
}

func renderBackupAuditText(w io.Writer, res result.Result) error {
	details, ok := res.Details.(result.BackupAuditDetails)
	if !ok {
		return result.Render(w, res, false)
	}
	artifacts, ok := res.Artifacts.(backupAuditArtifacts)
	if !ok {
		return fmt.Errorf("unexpected backup audit artifacts type %T", res.Artifacts)
	}

	okCount, warnCount, failCount := backupAuditStatusCounts(artifacts)
	selectedSet := "no data"
	if details.SelectedPrefix != "" || details.SelectedStamp != "" {
		selectedSet = details.SelectedPrefix
		if details.SelectedStamp != "" {
			selectedSet += " | " + details.SelectedStamp
		}
	}

	if _, err := fmt.Fprintln(w, "EspoCRM backup audit"); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "Backup directory:         %s\n", details.BackupRoot); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "Checksum verification:   %s\n", enabledText(details.VerifyChecksum)); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "Selected set:            %s\n", selectedSet); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(w, "\nFreshness thresholds:"); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "  Database backup:       %dh\n", details.DBMaxAgeHours); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "  Files backup:          %dh\n", details.FilesMaxAgeHours); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "  Manifests:             %dh\n", details.ManifestMaxAgeHours); err != nil {
		return err
	}

	if _, err := fmt.Fprintln(w, "\nSummary:"); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "  Successes:       %d\n", okCount); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "  Warnings:        %d\n", warnCount); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "  Failures:        %d\n", failCount); err != nil {
		return err
	}

	if err := renderBackupAuditComponent(w, "\nLatest database backup", artifacts.DBBackup); err != nil {
		return err
	}
	if err := renderBackupAuditComponent(w, "\nLatest files backup", artifacts.FilesBackup); err != nil {
		return err
	}
	if err := renderBackupAuditComponent(w, "\nLatest JSON manifest", artifacts.ManifestJSON); err != nil {
		return err
	}
	return renderBackupAuditComponent(w, "\nLatest text manifest", artifacts.ManifestTXT)
}

func renderBackupAuditComponent(w io.Writer, title string, component backup.AuditComponent) error {
	if _, err := fmt.Fprintf(w, "%s:\n", title); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "  Status:          %s\n", auditStatusText(component.Status)); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "  File:            %s\n", textOrNA(component.File)); err != nil {
		return err
	}
	if component.Sidecar != "" || component.Status != backup.AuditStatusSkipped {
		if _, err := fmt.Fprintf(w, "  Checksum file:   %s\n", textOrNA(component.Sidecar)); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintf(w, "  Age, h:          %s\n", intPtrText(component.AgeHours)); err != nil {
		return err
	}
	_, err := fmt.Fprintf(w, "  Message:         %s\n", textOrNA(component.Message))
	return err
}

func backupAuditStatusCounts(artifacts backupAuditArtifacts) (int, int, int) {
	okCount := 0
	warnCount := 0
	failCount := 0
	for _, status := range []string{
		artifacts.DBBackup.Status,
		artifacts.FilesBackup.Status,
		artifacts.ManifestJSON.Status,
		artifacts.ManifestTXT.Status,
	} {
		switch status {
		case backup.AuditStatusOK:
			okCount++
		case backup.AuditStatusWarn:
			warnCount++
		case backup.AuditStatusFail:
			failCount++
		}
	}

	return okCount, warnCount, failCount
}

func auditStatusText(status string) string {
	switch status {
	case backup.AuditStatusOK:
		return "success"
	case backup.AuditStatusWarn:
		return "warning"
	case backup.AuditStatusFail:
		return "failure"
	case backup.AuditStatusSkipped:
		return "skipped"
	default:
		return status
	}
}

func textOrNA(value string) string {
	if value == "" {
		return "n/a"
	}
	return value
}
