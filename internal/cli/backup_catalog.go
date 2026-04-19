package cli

import (
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/lazuale/espocrm-ops/internal/contract/exitcode"
	"github.com/lazuale/espocrm-ops/internal/contract/result"
	"github.com/lazuale/espocrm-ops/internal/usecase/backup"
	journalusecase "github.com/lazuale/espocrm-ops/internal/usecase/journal"
	"github.com/spf13/cobra"
)

func newBackupCatalogCmd() *cobra.Command {
	var backupRoot string
	var verifyChecksum bool
	var readyOnly bool
	var latestOnly bool
	var limit int

	cmd := &cobra.Command{
		Use:   "backup-catalog",
		Short: "List canonical backup inventory",
		Args:  noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			app := appForCommand(cmd)
			in := backupCatalogInput{
				backupRoot:     backupRoot,
				verifyChecksum: verifyChecksum,
				readyOnly:      readyOnly,
				latestOnly:     latestOnly,
				limit:          limit,
			}
			if err := validateBackupCatalogInput(&in); err != nil {
				return err
			}

			return RunCommand(cmd, CommandSpec{
				Name:       "backup-catalog",
				ErrorCode:  "backup_catalog_failed",
				ExitCode:   exitcode.ValidationError,
				RenderText: renderBackupCatalogText,
			}, func() (result.Result, error) {
				info, err := backup.Catalog(backup.CatalogRequest{
					BackupRoot:     in.backupRoot,
					JournalDir:     app.options.JournalDir,
					VerifyChecksum: in.verifyChecksum,
					ReadyOnly:      in.readyOnly,
					Limit:          in.resolvedLimit(),
					Now:            app.runtime.Now(),
				})
				if err != nil {
					return result.Result{}, err
				}

				return result.Result{
					Message:  "backup inventory loaded",
					Warnings: backupJournalWarnings(info.JournalRead),
					Details: result.BackupCatalogDetails{
						JournalReadDetails: backupJournalReadDetails(info.JournalRead),
						BackupRoot:         info.BackupRoot,
						VerifyChecksum:     info.VerifyChecksum,
						ReadyOnly:          info.ReadyOnly,
						Limit:              info.Limit,
						TotalSets:          info.TotalSets,
						ShownSets:          info.ShownSets,
					},
					Items: backupCatalogItems(info.Items),
				}, nil
			})
		},
	}

	cmd.Flags().StringVar(&backupRoot, "backup-root", "", "backup root containing db, files and manifests directories")
	cmd.Flags().BoolVar(&verifyChecksum, "verify-checksum", false, "verify checksum sidecars")
	cmd.Flags().BoolVar(&readyOnly, "ready-only", false, "include only restore-ready backup sets")
	cmd.Flags().BoolVar(&latestOnly, "latest-only", false, "include only the newest selected backup set")
	cmd.Flags().IntVar(&limit, "limit", 0, "maximum number of backup sets to include (0 means all)")

	return cmd
}

type backupCatalogInput struct {
	backupRoot     string
	verifyChecksum bool
	readyOnly      bool
	latestOnly     bool
	limit          int
}

func validateBackupCatalogInput(in *backupCatalogInput) error {
	in.backupRoot = strings.TrimSpace(in.backupRoot)
	if err := requireNonBlankFlag("--backup-root", in.backupRoot); err != nil {
		return err
	}
	if in.limit < 0 {
		return usageError(fmt.Errorf("--limit must be non-negative"))
	}

	return nil
}

func (in backupCatalogInput) resolvedLimit() int {
	if in.latestOnly {
		return 1
	}

	return in.limit
}

func backupCatalogItems(items []backup.CatalogItem) []any {
	out := make([]any, 0, len(items))
	for _, item := range items {
		out = append(out, item)
	}

	return out
}

func renderBackupCatalogText(w io.Writer, res result.Result) error {
	details, ok := res.Details.(result.BackupCatalogDetails)
	if !ok {
		return result.Render(w, res, false)
	}

	limit := "all"
	if details.Limit > 0 {
		limit = strconv.Itoa(details.Limit)
	}

	if _, err := fmt.Fprintln(w, "EspoCRM backup inventory"); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "Backup directory:         %s\n", details.BackupRoot); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "Checksum verification:   %s\n", enabledText(details.VerifyChecksum)); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "Ready-only filter:       %s\n", enabledText(details.ReadyOnly)); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "Limit:                   %s\n", limit); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "Journal files scanned:   %d\n", details.TotalFilesSeen); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "Total sets:              %d\n", details.TotalSets); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "Shown sets:              %d\n", details.ShownSets); err != nil {
		return err
	}

	if details.ShownSets == 0 {
		_, err := fmt.Fprintln(w, "\nNo backup sets matched the current selection filters")
		return err
	}

	for i, raw := range res.Items {
		item, ok := raw.(backup.CatalogItem)
		if !ok {
			return fmt.Errorf("unexpected backup catalog item type %T", raw)
		}
		if _, err := fmt.Fprintf(w, "\n[%d] %s | source=%s | readiness=%s\n", i+1, item.ID, item.Origin.Label, readinessText(item.RestoreReadiness)); err != nil {
			return err
		}
		if err := renderBackupIdentityText(w, item); err != nil {
			return err
		}
		if err := renderBackupArtifactsText(w, item); err != nil {
			return err
		}
	}

	return nil
}

func backupJournalReadDetails(stats backup.JournalReadStats) result.JournalReadDetails {
	return result.JournalReadDetails{
		TotalFilesSeen: stats.TotalFilesSeen,
		LoadedEntries:  stats.LoadedEntries,
		SkippedCorrupt: stats.SkippedCorrupt,
	}
}

func backupJournalWarnings(stats backup.JournalReadStats) []string {
	return journalusecase.WarningsFromReadStats(journalusecase.ReadStats{
		TotalFilesSeen: stats.TotalFilesSeen,
		LoadedEntries:  stats.LoadedEntries,
		SkippedCorrupt: stats.SkippedCorrupt,
	})
}

func renderBackupIdentityText(w io.Writer, item backup.CatalogItem) error {
	if _, err := fmt.Fprintf(w, "  Prefix/stamp:          %s | %s\n", item.Prefix, item.Stamp); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "  Scope/contour:         %s | %s\n", valueOrNA(item.Scope), valueOrNA(item.Contour)); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "  Created at:            %s\n", valueOrNA(item.CreatedAt)); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "  Compose project:       %s\n", valueOrNA(item.ComposeProject)); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "  Origin operation:      %s\n", backupOriginOperationText(item.Origin)); err != nil {
		return err
	}

	return nil
}

func renderBackupArtifactsText(w io.Writer, item backup.CatalogItem) error {
	if err := renderBackupCatalogArtifact(w, "Database", item.DB); err != nil {
		return err
	}
	if err := renderBackupCatalogArtifact(w, "Files", item.Files); err != nil {
		return err
	}
	if err := renderBackupCatalogManifest(w, "Text manifest", item.ManifestTXT); err != nil {
		return err
	}
	return renderBackupCatalogManifest(w, "JSON manifest", item.ManifestJSON)
}

func renderBackupCatalogArtifact(w io.Writer, label string, artifact backup.CatalogArtifact) error {
	if artifact.File == "" {
		_, err := fmt.Fprintf(w, "  %s: missing\n", label)
		return err
	}

	if _, err := fmt.Fprintf(w, "  %s: %s\n", label, artifact.File); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "    age_hours=%s size_bytes=%s\n", intPtrText(artifact.AgeHours), int64PtrText(artifact.SizeBytes)); err != nil {
		return err
	}
	_, err := fmt.Fprintf(w, "    checksum_file=%s checksum=%s\n", valueOrMissing(artifact.Sidecar), checksumText(artifact.ChecksumStatus))
	return err
}

func renderBackupCatalogManifest(w io.Writer, label string, manifest backup.CatalogManifest) error {
	if manifest.File == "" {
		_, err := fmt.Fprintf(w, "  %s: missing\n", label)
		return err
	}

	if _, err := fmt.Fprintf(w, "  %s: %s\n", label, manifest.File); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "    age_hours=%s status=%s\n", intPtrText(manifest.AgeHours), manifestStatusText(manifest.Status)); err != nil {
		return err
	}
	if manifest.Version != 0 || manifest.Scope != "" || manifest.Contour != "" || manifest.CreatedAt != "" || manifest.ComposeProject != "" {
		if _, err := fmt.Fprintf(w, "    version=%s scope=%s contour=%s created_at=%s compose_project=%s\n",
			intText(manifest.Version),
			valueOrMissing(manifest.Scope),
			valueOrMissing(manifest.Contour),
			valueOrMissing(manifest.CreatedAt),
			valueOrMissing(manifest.ComposeProject),
		); err != nil {
			return err
		}
	}
	if manifest.Error != "" {
		if _, err := fmt.Fprintf(w, "    error=%s\n", manifest.Error); err != nil {
			return err
		}
	}

	return nil
}

func enabledText(enabled bool) string {
	if enabled {
		return "enabled"
	}
	return "skipped"
}

func readinessText(readiness string) string {
	switch readiness {
	case backup.CatalogReadinessReadyVerified:
		return "ready, checksums verified"
	case backup.CatalogReadinessReadyUnverified:
		return "ready, checksums not verified"
	case backup.CatalogReadinessIncomplete:
		return "incomplete"
	case backup.CatalogReadinessCorrupted:
		return "corrupted"
	default:
		return readiness
	}
}

func checksumText(status string) string {
	switch status {
	case backup.CatalogChecksumVerified:
		return "verified"
	case backup.CatalogChecksumMismatch:
		return "mismatch"
	case backup.CatalogChecksumMissing:
		return "missing"
	case backup.CatalogChecksumSidecarMissing:
		return "checksum file missing"
	case backup.CatalogChecksumSidecarPresent:
		return "not checked"
	default:
		return status
	}
}

func manifestStatusText(status string) string {
	switch status {
	case backup.CatalogManifestValid:
		return "valid"
	case backup.CatalogManifestInvalid:
		return "invalid"
	case backup.CatalogManifestDirectory:
		return "directory"
	case backup.CatalogManifestMissing:
		return "missing"
	default:
		return status
	}
}

func backupOriginOperationText(origin backup.BackupOrigin) string {
	if origin.OperationID == "" && origin.Command == "" {
		return origin.Label
	}

	parts := []string{origin.Label}
	if origin.Command != "" {
		parts = append(parts, "command="+origin.Command)
	}
	if origin.OperationID != "" {
		parts = append(parts, "id="+origin.OperationID)
	}
	if origin.StartedAt != "" {
		parts = append(parts, "started_at="+origin.StartedAt)
	}

	return strings.Join(parts, " | ")
}

func intPtrText(value *int) string {
	if value == nil {
		return "n/a"
	}
	return strconv.Itoa(*value)
}

func int64PtrText(value *int64) string {
	if value == nil {
		return "n/a"
	}
	return strconv.FormatInt(*value, 10)
}

func intText(value int) string {
	if value == 0 {
		return "missing"
	}
	return strconv.Itoa(value)
}

func valueOrMissing(value string) string {
	if value == "" {
		return "missing"
	}
	return value
}

func valueOrNA(value string) string {
	if strings.TrimSpace(value) == "" {
		return "n/a"
	}
	return value
}
