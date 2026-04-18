package cli

import (
	"fmt"
	"io"

	journalusecase "github.com/lazuale/espocrm-ops/internal/usecase/journal"
)

func formatEntryLine(entry journalusecase.Entry) string {
	return fmt.Sprintf(
		"%s  %s  %s  dry_run=%t  id=%s",
		entry.StartedAt,
		entry.Command,
		formatEntryStatus(entry),
		entry.DryRun,
		entry.OperationID,
	)
}

func formatEntryStatus(entry journalusecase.Entry) string {
	if entry.OK {
		return "OK"
	}
	return "FAIL"
}

func renderWarnings(w io.Writer, warnings []string) error {
	for _, warning := range warnings {
		if _, err := fmt.Fprintf(w, "WARNING: %s\n", warning); err != nil {
			return err
		}
	}

	return nil
}
