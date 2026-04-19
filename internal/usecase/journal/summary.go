package journal

import (
	"fmt"

	"github.com/lazuale/espocrm-ops/internal/contract/result"
)

func WarningsFromReadStats(stats ReadStats) []string {
	if stats.SkippedCorrupt == 0 {
		return nil
	}

	return []string{fmt.Sprintf("skipped %d corrupt journal entrie(s)", stats.SkippedCorrupt)}
}

func ReadDetailsFromStats(stats ReadStats) result.JournalReadDetails {
	return result.JournalReadDetails{
		TotalFilesSeen: stats.TotalFilesSeen,
		LoadedEntries:  stats.LoadedEntries,
		SkippedCorrupt: stats.SkippedCorrupt,
	}
}

func HistoryDetailsFromReadStats(stats ReadStats) result.HistoryDetails {
	return result.HistoryDetails{
		JournalReadDetails: ReadDetailsFromStats(stats),
	}
}

func OperationLookupDetailsFromReadStats(stats ReadStats) result.OperationLookupDetails {
	return result.OperationLookupDetails{
		JournalReadDetails: ReadDetailsFromStats(stats),
	}
}

func OperationExportDetailsFromBundle(id string, bundle OperationBundle) result.OperationExportDetails {
	return result.OperationExportDetails{
		JournalReadDetails:      ReadDetailsFromStats(bundle.JournalRead),
		ID:                      id,
		BundleKind:              bundle.BundleKind,
		BundleVersion:           bundle.BundleVersion,
		ExportedAt:              bundle.ExportedAt,
		IncludedOmittedSections: result.NewIncludedOmittedSections(bundle.IncludedSections, bundle.OmittedSections),
	}
}

func PruneDetailsFromStats(stats ReadStats, checked, retained, protected, deleted, removedDirs, keepDays, keepLast int, latestOperationID string, dryRun bool) result.PruneDetails {
	return result.PruneDetails{
		JournalReadDetails: ReadDetailsFromStats(stats),
		Checked:            checked,
		Retained:           retained,
		Protected:          protected,
		Deleted:            deleted,
		RemovedDirs:        removedDirs,
		KeepDays:           keepDays,
		KeepLast:           keepLast,
		LatestOperationID:  latestOperationID,
		DryRun:             dryRun,
	}
}
