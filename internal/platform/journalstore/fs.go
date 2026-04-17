package journalstore

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	domainjournal "github.com/lazuale/espocrm-ops/internal/domain/journal"
)

type FSWriter struct {
	Dir string
}

func (w FSWriter) Write(entry domainjournal.Entry) error {
	if w.Dir == "" {
		return fmt.Errorf("journal dir is required")
	}

	if err := os.MkdirAll(w.Dir, 0o755); err != nil {
		return fmt.Errorf("create journal dir: %w", err)
	}

	path := filepath.Join(w.Dir, entry.OperationID+".json")

	raw, err := json.MarshalIndent(entry, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal journal entry: %w", err)
	}

	if err := os.WriteFile(path, raw, 0o644); err != nil {
		return fmt.Errorf("write journal entry: %w", err)
	}

	return nil
}
