package backup

import (
	"fmt"
	"strings"
	"time"
)

const (
	HealthVerdictHealthy  = "healthy"
	HealthVerdictDegraded = "degraded"
	HealthVerdictBlocked  = "blocked"

	HealthAlertWarning = "warning"
	HealthAlertBreach  = "breach"
)

type HealthRequest struct {
	BackupRoot     string
	JournalDir     string
	VerifyChecksum bool
	MaxAgeHours    int
	Now            time.Time
}

type HealthInfo struct {
	BackupRoot            string           `json:"backup_root"`
	Verdict               string           `json:"verdict"`
	VerifyChecksum        bool             `json:"verify_checksum"`
	MaxAgeHours           int              `json:"max_age_hours"`
	TotalSets             int              `json:"total_sets"`
	ReadySets             int              `json:"ready_sets"`
	RestoreReady          bool             `json:"restore_ready"`
	FreshnessSatisfied    bool             `json:"freshness_satisfied"`
	VerificationSatisfied bool             `json:"verification_satisfied"`
	LatestSet             *CatalogItem     `json:"latest_set,omitempty"`
	LatestReadySet        *CatalogItem     `json:"latest_ready_set,omitempty"`
	LatestReadyAgeHours   *int             `json:"latest_ready_age_hours,omitempty"`
	Alerts                []HealthAlert    `json:"alerts,omitempty"`
	JournalRead           JournalReadStats `json:"journal_read"`
	NextAction            string           `json:"next_action,omitempty"`
	EvaluatedAt           string           `json:"evaluated_at,omitempty"`
}

type HealthAlert struct {
	Code    string `json:"code"`
	Level   string `json:"level"`
	Summary string `json:"summary"`
	Details string `json:"details,omitempty"`
	Action  string `json:"action,omitempty"`
}

func Health(req HealthRequest) (HealthInfo, error) {
	backupRoot := strings.TrimSpace(req.BackupRoot)
	if backupRoot == "" {
		return HealthInfo{}, fmt.Errorf("backup root is required")
	}
	if req.MaxAgeHours < 0 {
		return HealthInfo{}, fmt.Errorf("max age hours must be non-negative")
	}

	now := req.Now
	if now.IsZero() {
		now = time.Now()
	}

	catalog, err := Catalog(CatalogRequest{
		BackupRoot:     backupRoot,
		JournalDir:     strings.TrimSpace(req.JournalDir),
		VerifyChecksum: req.VerifyChecksum,
		Now:            now,
	})
	if err != nil {
		return HealthInfo{}, err
	}

	info := HealthInfo{
		BackupRoot:     backupRoot,
		VerifyChecksum: req.VerifyChecksum,
		MaxAgeHours:    req.MaxAgeHours,
		TotalSets:      catalog.TotalSets,
		JournalRead:    catalog.JournalRead,
		EvaluatedAt:    now.UTC().Format(time.RFC3339),
	}

	if len(catalog.Items) != 0 {
		latest := catalog.Items[0]
		info.LatestSet = &latest
	}

	for _, item := range catalog.Items {
		if !item.IsReady {
			continue
		}
		info.ReadySets++
		if info.LatestReadySet != nil {
			continue
		}

		latestReady := item
		info.LatestReadySet = &latestReady
		if age, ok := catalogItemAgeHours(item); ok {
			info.LatestReadyAgeHours = intPtr(age)
		}
	}

	info.RestoreReady = info.LatestReadySet != nil
	info.FreshnessSatisfied = info.RestoreReady && info.LatestReadyAgeHours != nil && *info.LatestReadyAgeHours <= req.MaxAgeHours
	info.VerificationSatisfied = info.LatestReadySet != nil && req.VerifyChecksum && info.LatestReadySet.RestoreReadiness == CatalogReadinessReadyVerified
	info.Alerts = buildHealthAlerts(info)
	info.Verdict = healthVerdict(info.Alerts)

	if len(info.Alerts) == 0 {
		info.NextAction = "Continue routine backup monitoring."
	} else {
		info.NextAction = info.Alerts[0].Action
	}

	return info, nil
}

func (i HealthInfo) WarningCount() int {
	return healthAlertCount(i.Alerts, HealthAlertWarning)
}

func (i HealthInfo) BreachCount() int {
	return healthAlertCount(i.Alerts, HealthAlertBreach)
}

func buildHealthAlerts(info HealthInfo) []HealthAlert {
	if info.TotalSets == 0 {
		return []HealthAlert{{
			Code:    "no_backup_sets",
			Level:   HealthAlertBreach,
			Summary: "No backup sets were found",
			Details: fmt.Sprintf("No manifest-backed backup sets were found under %s.", info.BackupRoot),
			Action:  "Run a new successful backup and confirm manifests and checksum sidecars are written under the backup root.",
		}}
	}

	if info.LatestReadySet == nil {
		details := "The backup catalog did not find any restore-ready backup set."
		if info.LatestSet != nil {
			details = fmt.Sprintf(
				"Latest observed set %s is %s: %s.",
				info.LatestSet.ID,
				info.LatestSet.RestoreReadiness,
				healthIssueSummary(*info.LatestSet),
			)
		}
		return []HealthAlert{{
			Code:    "no_restore_ready_backup",
			Level:   HealthAlertBreach,
			Summary: "No restore-ready backup set is currently available",
			Details: details,
			Action:  "Create or repair a coherent backup set before relying on restore or automation.",
		}}
	}

	alerts := []HealthAlert{}

	if info.LatestReadyAgeHours == nil {
		alerts = append(alerts, HealthAlert{
			Code:    "freshness_unknown",
			Level:   HealthAlertWarning,
			Summary: "Could not determine the latest restore-ready backup age",
			Details: fmt.Sprintf("Latest restore-ready set %s did not expose enough timestamp metadata to evaluate backup age.", info.LatestReadySet.ID),
			Action:  "Inspect the latest restore-ready backup with show-backup and confirm manifest timestamps are present.",
		})
	} else if *info.LatestReadyAgeHours > info.MaxAgeHours {
		alerts = append(alerts, HealthAlert{
			Code:    "freshness_breached",
			Level:   HealthAlertBreach,
			Summary: "Latest restore-ready backup is older than policy",
			Details: fmt.Sprintf(
				"Latest restore-ready set %s is %dh old, exceeding the %dh freshness policy.",
				info.LatestReadySet.ID,
				*info.LatestReadyAgeHours,
				info.MaxAgeHours,
			),
			Action: "Create a new successful backup before relying on restore or automation.",
		})
	}

	if info.LatestSet != nil && info.LatestSet.ID != info.LatestReadySet.ID {
		alerts = append(alerts, HealthAlert{
			Code:    "latest_set_not_ready",
			Level:   HealthAlertWarning,
			Summary: "Latest observed backup set is not restore-ready",
			Details: fmt.Sprintf(
				"Latest observed set %s is %s: %s. Latest restore-ready set remains %s.",
				info.LatestSet.ID,
				info.LatestSet.RestoreReadiness,
				healthIssueSummary(*info.LatestSet),
				info.LatestReadySet.ID,
			),
			Action: "Inspect the newest backup set with show-backup and rerun or repair the latest backup so the newest set becomes restore-ready.",
		})
	}

	if !info.VerifyChecksum {
		alerts = append(alerts, HealthAlert{
			Code:    "checksum_verification_skipped",
			Level:   HealthAlertWarning,
			Summary: "Checksum verification was skipped",
			Details: fmt.Sprintf("Latest restore-ready set %s was evaluated without verifying checksum sidecars.", info.LatestReadySet.ID),
			Action:  "Rerun backup-health without --no-verify-checksum before treating posture as fully healthy.",
		})
	}

	return alerts
}

func healthVerdict(alerts []HealthAlert) string {
	switch {
	case healthAlertCount(alerts, HealthAlertBreach) != 0:
		return HealthVerdictBlocked
	case len(alerts) != 0:
		return HealthVerdictDegraded
	default:
		return HealthVerdictHealthy
	}
}

func healthAlertCount(alerts []HealthAlert, level string) int {
	count := 0
	for _, alert := range alerts {
		if alert.Level == level {
			count++
		}
	}

	return count
}

func catalogItemAgeHours(item CatalogItem) (int, bool) {
	if item.ManifestJSON.AgeHours != nil {
		return *item.ManifestJSON.AgeHours, true
	}
	if item.ManifestTXT.AgeHours != nil {
		return *item.ManifestTXT.AgeHours, true
	}

	maxAge := 0
	found := false
	for _, age := range []*int{item.DB.AgeHours, item.Files.AgeHours} {
		if age == nil {
			continue
		}
		if !found || *age > maxAge {
			maxAge = *age
		}
		found = true
	}

	return maxAge, found
}

func healthIssueSummary(item CatalogItem) string {
	issues := []string{}

	issues = append(issues, healthArtifactIssues("database backup", item.DB)...)
	issues = append(issues, healthArtifactIssues("files backup", item.Files)...)
	issues = append(issues, healthManifestIssues("json manifest", item.ManifestJSON)...)
	issues = append(issues, healthManifestIssues("text manifest", item.ManifestTXT)...)

	if len(issues) == 0 {
		return "artifacts are paired and manifest/checksum metadata is coherent"
	}

	return strings.Join(issues, "; ")
}

func healthArtifactIssues(label string, artifact CatalogArtifact) []string {
	switch {
	case artifact.File == "":
		return []string{label + " is missing"}
	case artifact.ChecksumStatus == CatalogChecksumMismatch:
		return []string{label + " failed checksum validation"}
	case artifact.ChecksumStatus == CatalogChecksumSidecarMissing:
		return []string{label + " is missing its checksum sidecar"}
	case artifact.ChecksumStatus == CatalogChecksumSidecarPresent:
		return []string{label + " checksum was not verified"}
	default:
		return nil
	}
}

func healthManifestIssues(label string, manifest CatalogManifest) []string {
	switch manifest.Status {
	case CatalogManifestMissing:
		return []string{label + " is missing"}
	case CatalogManifestInvalid:
		return []string{label + " is invalid"}
	case CatalogManifestDirectory:
		return []string{label + " path is a directory"}
	default:
		return nil
	}
}

func intPtr(value int) *int {
	return &value
}
