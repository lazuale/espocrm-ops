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

func PruneDetailsFromStats(stats ReadStats, checked, deleted, removedDirs, keepDays, keep int, dryRun bool) result.PruneDetails {
	return result.PruneDetails{
		JournalReadDetails: ReadDetailsFromStats(stats),
		Checked:            checked,
		Deleted:            deleted,
		RemovedDirs:        removedDirs,
		KeepDays:           keepDays,
		Keep:               keep,
		DryRun:             dryRun,
	}
}
