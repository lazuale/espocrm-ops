package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"

	journalusecase "github.com/lazuale/espocrm-ops/internal/usecase/journal"
)

func renderOperationReportText(w io.Writer, report journalusecase.OperationReport) error {
	if _, err := fmt.Fprintf(w, "EspoCRM %s report\n", report.Command); err != nil {
		return err
	}

	fields := []struct {
		label string
		value string
	}{
		{label: "Operation ID", value: report.OperationID},
		{label: "Mode", value: reportMode(report)},
		{label: "Outcome", value: reportOutcome(report)},
		{label: "Started at", value: report.StartedAt},
		{label: "Finished at", value: report.FinishedAt},
		{label: "Scope", value: report.Scope},
		{label: "Project dir", value: reportArtifact(report, "project_dir")},
		{label: "Compose file", value: reportArtifact(report, "compose_file")},
		{label: "Env file", value: reportArtifact(report, "env_file")},
		{label: "Compose name", value: reportArtifact(report, "compose_project")},
		{label: "Backup root", value: reportArtifact(report, "backup_root")},
		{label: "Site URL", value: reportArtifact(report, "site_url")},
	}
	for _, field := range fields {
		if strings.TrimSpace(field.value) == "" {
			continue
		}
		if _, err := fmt.Fprintf(w, "%-13s %s\n", field.label+":", field.value); err != nil {
			return err
		}
	}

	if report.Target != nil {
		targetFields := []struct {
			label string
			value string
		}{
			{label: "Selection", value: report.Target.SelectionMode},
			{label: "Target prefix", value: report.Target.Prefix},
			{label: "Target stamp", value: report.Target.Stamp},
			{label: "Manifest JSON", value: report.Target.ManifestJSON},
			{label: "DB backup", value: report.Target.DBBackup},
			{label: "Files backup", value: report.Target.FilesBackup},
		}
		for _, field := range targetFields {
			if strings.TrimSpace(field.value) == "" {
				continue
			}
			if _, err := fmt.Fprintf(w, "%-13s %s\n", field.label+":", field.value); err != nil {
				return err
			}
		}
	}

	if strings.TrimSpace(report.Message) != "" || report.DurationMS > 0 || report.Counts.Steps != 0 {
		if _, err := fmt.Fprintln(w, "\nSummary:"); err != nil {
			return err
		}
		if strings.TrimSpace(report.Message) != "" {
			if _, err := fmt.Fprintf(w, "  %-12s %s\n", "Message:", report.Message); err != nil {
				return err
			}
		}
		if report.DurationMS > 0 {
			if _, err := fmt.Fprintf(w, "  %-12s %d\n", "Duration ms:", report.DurationMS); err != nil {
				return err
			}
		}
		if _, err := fmt.Fprintf(w, "  %-12s %d\n", "Steps:", report.Counts.Steps); err != nil {
			return err
		}
		if report.Counts.WouldRun != 0 || report.DryRun {
			if _, err := fmt.Fprintf(w, "  %-12s %d\n", "Would run:", report.Counts.WouldRun); err != nil {
				return err
			}
		}
		if _, err := fmt.Fprintf(w, "  %-12s %d\n", "Completed:", report.Counts.Completed); err != nil {
			return err
		}
		if _, err := fmt.Fprintf(w, "  %-12s %d\n", "Skipped:", report.Counts.Skipped); err != nil {
			return err
		}
		if _, err := fmt.Fprintf(w, "  %-12s %d\n", "Blocked:", report.Counts.Blocked); err != nil {
			return err
		}
		if _, err := fmt.Fprintf(w, "  %-12s %d\n", "Failed:", report.Counts.Failed); err != nil {
			return err
		}
		if report.Counts.Unknown != 0 {
			if _, err := fmt.Fprintf(w, "  %-12s %d\n", "Unknown:", report.Counts.Unknown); err != nil {
				return err
			}
		}
	}

	if report.Failure != nil {
		if _, err := fmt.Fprintln(w, "\nFailure:"); err != nil {
			return err
		}
		failureFields := []struct {
			label string
			value string
		}{
			{label: "Code", value: report.Failure.Code},
			{label: "Message", value: report.Failure.Message},
			{label: "Step", value: report.Failure.StepCode},
			{label: "Summary", value: report.Failure.StepSummary},
			{label: "Action", value: report.Failure.Action},
		}
		for _, field := range failureFields {
			if strings.TrimSpace(field.value) == "" {
				continue
			}
			if _, err := fmt.Fprintf(w, "  %-10s %s\n", field.label+":", field.value); err != nil {
				return err
			}
		}
	}

	if err := renderOperationRecoverySection(w, report.Recovery); err != nil {
		return err
	}

	if len(report.Warnings) != 0 {
		if _, err := fmt.Fprintln(w, "\nWarnings:"); err != nil {
			return err
		}
		for _, warning := range report.Warnings {
			if _, err := fmt.Fprintf(w, "- %s\n", warning); err != nil {
				return err
			}
		}
	}

	if len(report.Steps) != 0 {
		if _, err := fmt.Fprintln(w, "\nSteps:"); err != nil {
			return err
		}
		for _, step := range report.Steps {
			if _, err := fmt.Fprintf(w, "[%s] %s\n", strings.ToUpper(step.Status), step.Summary); err != nil {
				return err
			}
			if strings.TrimSpace(step.Details) != "" {
				if _, err := fmt.Fprintf(w, "  %s\n", step.Details); err != nil {
					return err
				}
			}
			if strings.TrimSpace(step.Action) != "" {
				if _, err := fmt.Fprintf(w, "  Action: %s\n", step.Action); err != nil {
					return err
				}
			}
		}
		return nil
	}

	if err := renderReportMapSection(w, "Details", report.Details); err != nil {
		return err
	}
	if err := renderReportMapSection(w, "Artifacts", report.Artifacts); err != nil {
		return err
	}

	return nil
}

func renderReportMapSection(w io.Writer, title string, values map[string]any) error {
	if len(values) == 0 {
		return nil
	}

	if _, err := fmt.Fprintf(w, "\n%s:\n", title); err != nil {
		return err
	}

	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	for _, key := range keys {
		rendered, err := renderReportValue(values[key])
		if err != nil {
			return err
		}
		if _, err := fmt.Fprintf(w, "  %-12s %s\n", key+":", rendered); err != nil {
			return err
		}
	}

	return nil
}

func renderReportValue(value any) (string, error) {
	switch v := value.(type) {
	case string:
		return v, nil
	case nil:
		return "", nil
	default:
		raw, err := json.Marshal(v)
		if err != nil {
			return "", err
		}
		return string(raw), nil
	}
}

func reportMode(report journalusecase.OperationReport) string {
	if report.DryRun {
		return "dry-run"
	}
	return "execution"
}

func reportOutcome(report journalusecase.OperationReport) string {
	if report.DryRun {
		if report.OK {
			return "PLANNED"
		}
		return "BLOCKED"
	}
	if report.OK {
		return "COMPLETED"
	}
	return "FAILED"
}

func reportArtifact(report journalusecase.OperationReport, key string) string {
	if report.Artifacts == nil {
		return ""
	}

	value, _ := report.Artifacts[key].(string)
	return value
}
