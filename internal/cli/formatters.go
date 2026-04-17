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

func renderEntryDetail(w io.Writer, entry journalusecase.Entry) error {
	fields := []struct {
		name  string
		value string
	}{
		{name: "id", value: entry.OperationID},
		{name: "command", value: entry.Command},
		{name: "status", value: formatEntryStatus(entry)},
		{name: "started_at", value: entry.StartedAt},
		{name: "finished_at", value: entry.FinishedAt},
		{name: "dry_run", value: fmt.Sprintf("%t", entry.DryRun)},
	}

	for _, field := range fields {
		if _, err := fmt.Fprintf(w, "%s: %s\n", field.name, field.value); err != nil {
			return err
		}
	}
	if entry.ErrorCode != "" {
		if _, err := fmt.Fprintf(w, "error_code: %s\n", entry.ErrorCode); err != nil {
			return err
		}
	}
	if entry.ErrorMessage != "" {
		if _, err := fmt.Fprintf(w, "error_message: %s\n", entry.ErrorMessage); err != nil {
			return err
		}
	}

	return nil
}

func renderWarnings(w io.Writer, warnings []string) error {
	for _, warning := range warnings {
		if _, err := fmt.Fprintf(w, "WARNING: %s\n", warning); err != nil {
			return err
		}
	}

	return nil
}
