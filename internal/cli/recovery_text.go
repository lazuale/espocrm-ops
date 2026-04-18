package cli

import (
	"fmt"
	"io"
	"strings"

	"github.com/lazuale/espocrm-ops/internal/contract/result"
	journalusecase "github.com/lazuale/espocrm-ops/internal/usecase/journal"
)

func renderRecoveryAttemptSection(w io.Writer, recovery *result.RecoveryDetails) error {
	if recovery == nil {
		return nil
	}

	if _, err := fmt.Fprintln(w, "\nRecovery:"); err != nil {
		return err
	}

	lines := []struct {
		label string
		value string
	}{
		{label: "Source op", value: recovery.SourceOperationID},
		{label: "Requested", value: recovery.RequestedMode},
		{label: "Applied", value: recovery.AppliedMode},
		{label: "Decision", value: recovery.Decision},
		{label: "Resume step", value: recovery.ResumeStep},
	}
	for _, line := range lines {
		if strings.TrimSpace(line.value) == "" {
			continue
		}
		if _, err := fmt.Fprintf(w, "  %-12s %s\n", line.label+":", line.value); err != nil {
			return err
		}
	}

	return nil
}

func renderOperationRecoverySection(w io.Writer, recovery *journalusecase.OperationRecovery) error {
	if recovery == nil {
		return nil
	}

	if _, err := fmt.Fprintln(w, "\nRecovery:"); err != nil {
		return err
	}

	lines := []struct {
		label string
		value string
	}{
		{label: "Decision", value: recovery.Decision},
		{label: "Recommended", value: recovery.RecommendedMode},
		{label: "Retryable", value: fmt.Sprintf("%t", recovery.Retryable)},
		{label: "Resumable", value: fmt.Sprintf("%t", recovery.Resumable)},
		{label: "Resume step", value: recovery.ResumeStep},
		{label: "Summary", value: recovery.Summary},
		{label: "Details", value: recovery.Details},
		{label: "Action", value: recovery.Action},
	}
	for _, line := range lines {
		if strings.TrimSpace(line.value) == "" {
			continue
		}
		if _, err := fmt.Fprintf(w, "  %-12s %s\n", line.label+":", line.value); err != nil {
			return err
		}
	}

	return nil
}
