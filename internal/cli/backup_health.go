package cli

import (
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/lazuale/espocrm-ops/internal/contract/apperr"
	"github.com/lazuale/espocrm-ops/internal/contract/exitcode"
	"github.com/lazuale/espocrm-ops/internal/contract/result"
	"github.com/lazuale/espocrm-ops/internal/usecase/backup"
	"github.com/spf13/cobra"
)

func newBackupHealthCmd() *cobra.Command {
	var backupRoot string
	var noVerifyChecksum bool
	var maxAgeHours int

	cmd := &cobra.Command{
		Use:   "backup-health",
		Short: "Show canonical backup freshness and policy posture",
		Args:  noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			in := backupHealthInput{
				backupRoot:     backupRoot,
				verifyChecksum: !noVerifyChecksum,
				maxAgeHours:    maxAgeHours,
			}
			if err := validateBackupHealthInput(&in); err != nil {
				return err
			}

			info, err := backup.Health(backup.HealthRequest{
				BackupRoot:     in.backupRoot,
				JournalDir:     appForCommand(cmd).options.JournalDir,
				VerifyChecksum: in.verifyChecksum,
				MaxAgeHours:    in.maxAgeHours,
				Now:            appForCommand(cmd).runtime.Now(),
			})
			if err != nil {
				return err
			}

			res := backupHealthResult(info)
			if info.Verdict != backup.HealthVerdictBlocked {
				return renderCommandResult(cmd, CommandSpec{
					Name:       "backup-health",
					RenderText: renderBackupHealthText,
				}, res)
			}

			errCode := CodeError{
				Code:    exitcode.ValidationError,
				Err:     apperr.Wrap(apperr.KindValidation, "backup_health_blocked", errors.New("backup posture is blocked")),
				ErrCode: "backup_health_blocked",
			}

			if appForCommand(cmd).JSONEnabled() {
				return ResultCodeError{
					CodeError: errCode,
					Result:    res,
				}
			}

			if err := renderBackupHealthText(cmd.OutOrStdout(), res); err != nil {
				return err
			}
			if err := renderWarnings(cmd.OutOrStdout(), res.Warnings); err != nil {
				return err
			}
			return silentCodeError{CodeError: errCode}
		},
	}

	cmd.Flags().StringVar(&backupRoot, "backup-root", "", "backup root containing db, files and manifests directories")
	cmd.Flags().BoolVar(&noVerifyChecksum, "no-verify-checksum", false, "skip checksum verification and report unverified backup posture")
	cmd.Flags().IntVar(&maxAgeHours, "max-age-hours", 48, "maximum allowed age in hours for the latest restore-ready backup set")

	return cmd
}

type backupHealthInput struct {
	backupRoot     string
	verifyChecksum bool
	maxAgeHours    int
}

type backupHealthArtifacts struct {
	LatestSet      *backup.CatalogItem `json:"latest_set,omitempty"`
	LatestReadySet *backup.CatalogItem `json:"latest_ready_set,omitempty"`
}

func validateBackupHealthInput(in *backupHealthInput) error {
	in.backupRoot = strings.TrimSpace(in.backupRoot)
	if err := requireNonBlankFlag("--backup-root", in.backupRoot); err != nil {
		return err
	}
	if in.maxAgeHours < 0 {
		return usageError(fmt.Errorf("--max-age-hours must be non-negative"))
	}

	return nil
}

func backupHealthResult(info backup.HealthInfo) result.Result {
	res := result.Result{
		Command:  "backup-health",
		OK:       info.Verdict != backup.HealthVerdictBlocked,
		Message:  "backup posture " + info.Verdict,
		Warnings: backupJournalWarnings(info.JournalRead),
		Details: result.BackupHealthDetails{
			JournalReadDetails:    backupJournalReadDetails(info.JournalRead),
			BackupRoot:            info.BackupRoot,
			Verdict:               info.Verdict,
			VerifyChecksum:        info.VerifyChecksum,
			MaxAgeHours:           info.MaxAgeHours,
			TotalSets:             info.TotalSets,
			ReadySets:             info.ReadySets,
			RestoreReady:          info.RestoreReady,
			FreshnessSatisfied:    info.FreshnessSatisfied,
			VerificationSatisfied: info.VerificationSatisfied,
			WarningAlerts:         info.WarningCount(),
			BreachAlerts:          info.BreachCount(),
			LatestSetID:           catalogItemID(info.LatestSet),
			LatestSetReadiness:    catalogItemReadiness(info.LatestSet),
			LatestReadySetID:      catalogItemID(info.LatestReadySet),
			LatestReadyReadiness:  catalogItemReadiness(info.LatestReadySet),
			LatestReadyAgeHours:   info.LatestReadyAgeHours,
			NextAction:            info.NextAction,
		},
		Artifacts: backupHealthArtifacts{
			LatestSet:      info.LatestSet,
			LatestReadySet: info.LatestReadySet,
		},
		Items: backupHealthAlertItems(info.Alerts),
	}

	return res
}

func renderBackupHealthText(w io.Writer, res result.Result) error {
	details, ok := res.Details.(result.BackupHealthDetails)
	if !ok {
		return result.Render(w, res, false)
	}
	artifacts, ok := res.Artifacts.(backupHealthArtifacts)
	if !ok {
		return fmt.Errorf("unexpected backup-health artifacts type %T", res.Artifacts)
	}

	if _, err := fmt.Fprintln(w, "EspoCRM backup health"); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "Backup directory:         %s\n", details.BackupRoot); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "Verdict:                  %s\n", details.Verdict); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "Checksum verification:    %s\n", enabledText(details.VerifyChecksum)); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "Freshness threshold:      %dh\n", details.MaxAgeHours); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "Journal files scanned:    %d\n", details.TotalFilesSeen); err != nil {
		return err
	}

	if _, err := fmt.Fprintln(w, "\nSummary:"); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "  Total sets:             %d\n", details.TotalSets); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "  Ready sets:             %d\n", details.ReadySets); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "  Restore ready:          %t\n", details.RestoreReady); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "  Freshness satisfied:    %t\n", details.FreshnessSatisfied); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "  Verification satisfied: %t\n", details.VerificationSatisfied); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "  Warnings:               %d\n", details.WarningAlerts); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "  Breaches:               %d\n", details.BreachAlerts); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "  Latest observed set:    %s\n", valueOrNA(details.LatestSetID)); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "  Latest ready set:       %s\n", valueOrNA(details.LatestReadySetID)); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "  Latest ready age:       %s\n", backupHealthAgeText(details.LatestReadyAgeHours)); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "  Next action:            %s\n", valueOrNA(details.NextAction)); err != nil {
		return err
	}

	if err := renderBackupHealthSetSection(w, "\nLatest observed backup", artifacts.LatestSet); err != nil {
		return err
	}
	if err := renderBackupHealthSetSection(w, "\nLatest restore-ready backup", artifacts.LatestReadySet); err != nil {
		return err
	}

	if err := renderStepItemsBlock(w, res.Items, backupHealthAlertItem, stepRenderOptions{
		Title:      "Alerts",
		StatusText: upperStatusText,
	}); err != nil {
		return err
	}

	return nil
}

func renderBackupHealthSetSection(w io.Writer, title string, item *backup.CatalogItem) error {
	if _, err := fmt.Fprintf(w, "%s:\n", title); err != nil {
		return err
	}
	if item == nil {
		_, err := fmt.Fprintln(w, "  none")
		return err
	}

	if _, err := fmt.Fprintf(w, "  ID:                %s\n", item.ID); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "  Readiness:         %s\n", item.RestoreReadiness); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "  Created at:        %s\n", valueOrNA(item.CreatedAt)); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "  Age:               %s\n", backupHealthAgeText(backupHealthItemAge(item))); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "  Source:            %s\n", valueOrNA(item.Origin.Label)); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "  DB/files:          %s\n", backupHealthPairingText(*item)); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "  Checksum posture:  db=%s files=%s\n", checksumText(item.DB.ChecksumStatus), checksumText(item.Files.ChecksumStatus)); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "  Manifest posture:  json=%s txt=%s\n", manifestStatusText(item.ManifestJSON.Status), manifestStatusText(item.ManifestTXT.Status)); err != nil {
		return err
	}

	return nil
}

func backupHealthAlertItems(alerts []backup.HealthAlert) []any {
	out := make([]any, 0, len(alerts))
	for _, alert := range alerts {
		out = append(out, alert)
	}

	return out
}

func backupHealthAlertItem(raw any) (result.SectionItem, error) {
	alert, ok := raw.(backup.HealthAlert)
	if !ok {
		return result.SectionItem{}, fmt.Errorf("unexpected backup-health alert type %T", raw)
	}

	return result.SectionItem{
		Code:    alert.Code,
		Status:  alert.Level,
		Summary: alert.Summary,
		Details: alert.Details,
		Action:  alert.Action,
	}, nil
}

func catalogItemID(item *backup.CatalogItem) string {
	if item == nil {
		return ""
	}

	return item.ID
}

func catalogItemReadiness(item *backup.CatalogItem) string {
	if item == nil {
		return ""
	}

	return item.RestoreReadiness
}

func backupHealthAgeText(age *int) string {
	if age == nil {
		return "n/a"
	}

	return fmt.Sprintf("%dh", *age)
}

func backupHealthItemAge(item *backup.CatalogItem) *int {
	if item == nil {
		return nil
	}
	if item.ManifestJSON.AgeHours != nil {
		return item.ManifestJSON.AgeHours
	}
	if item.ManifestTXT.AgeHours != nil {
		return item.ManifestTXT.AgeHours
	}
	if item.DB.AgeHours != nil {
		return item.DB.AgeHours
	}
	if item.Files.AgeHours != nil {
		return item.Files.AgeHours
	}

	return nil
}

func backupHealthPairingText(item backup.CatalogItem) string {
	switch {
	case item.DB.File == "" && item.Files.File == "":
		return "database missing, files missing"
	case item.DB.File == "":
		return "database missing, files present"
	case item.Files.File == "":
		return "database present, files missing"
	default:
		return "database present, files present"
	}
}
