package journal

import "github.com/lazuale/espocrm-ops/internal/platform/journalstore"

const (
	PruneItemKindOperation = "operation"
	PruneItemKindDir       = "dir"
)

type PruneItem struct {
	Kind      string            `json:"kind"`
	Decision  string            `json:"decision"`
	Path      string            `json:"path,omitempty"`
	Reasons   []string          `json:"reasons,omitempty"`
	Operation *OperationSummary `json:"operation,omitempty"`
}

func pruneItemsFromStore(pruneResult journalstore.PruneResult) []PruneItem {
	items := make([]PruneItem, 0, len(pruneResult.Decisions)+len(pruneResult.RemovedPaths))

	for _, decision := range pruneResult.Decisions {
		operation := summarizeOperation(entryFromDomain(decision.Entry))
		items = append(items, PruneItem{
			Kind:      PruneItemKindOperation,
			Decision:  decision.Decision,
			Path:      decision.Path,
			Reasons:   append([]string(nil), decision.Reasons...),
			Operation: &operation,
		})
	}

	for _, path := range pruneResult.RemovedPaths {
		items = append(items, PruneItem{
			Kind:     PruneItemKindDir,
			Decision: journalstore.PruneDecisionRemove,
			Path:     path,
		})
	}

	return items
}
