package cli

import (
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/lazuale/espocrm-ops/internal/contract/exitcode"
	"github.com/lazuale/espocrm-ops/internal/contract/result"
	"github.com/lazuale/espocrm-ops/internal/usecase/backup"
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
		Short: "Build a backup-set catalog",
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
					VerifyChecksum: in.verifyChecksum,
					ReadyOnly:      in.readyOnly,
					Limit:          in.resolvedLimit(),
					Now:            app.runtime.Now(),
				})
				if err != nil {
					return result.Result{}, err
				}

				return result.Result{
					Message: "backup catalog built",
					Details: result.BackupCatalogDetails{
						BackupRoot:     info.BackupRoot,
						VerifyChecksum: info.VerifyChecksum,
						ReadyOnly:      info.ReadyOnly,
						Limit:          info.Limit,
						TotalSets:      info.TotalSets,
						ShownSets:      info.ShownSets,
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

	if _, err := fmt.Fprintln(w, "EspoCRM backup-set catalog"); err != nil {
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
	if _, err := fmt.Fprintf(w, "Total sets:        %d\n", details.TotalSets); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "Shown sets:        %d\n", details.ShownSets); err != nil {
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
		if _, err := fmt.Fprintf(w, "\n[%d] %s | prefix=%s | readiness=%s\n", i+1, item.Stamp, item.Prefix, readinessText(item.RestoreReadiness)); err != nil {
			return err
		}
		if err := renderBackupCatalogArtifact(w, "Database", item.DB); err != nil {
			return err
		}
		if err := renderBackupCatalogArtifact(w, "Files", item.Files); err != nil {
			return err
		}
		if err := renderBackupCatalogManifest(w, "Text manifest", item.ManifestTXT); err != nil {
			return err
		}
		if err := renderBackupCatalogManifest(w, "JSON manifest", item.ManifestJSON); err != nil {
			return err
		}
	}

	return nil
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
	_, err := fmt.Fprintf(w, "    age_hours=%s\n", intPtrText(manifest.AgeHours))
	return err
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

func valueOrMissing(value string) string {
	if value == "" {
		return "missing"
	}
	return value
}
