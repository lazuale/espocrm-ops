package cli

import (
	"fmt"
	"io"

	"github.com/lazuale/espocrm-ops/internal/contract/result"
)

type operatorSummaryLine struct {
	Label string
	Value any
}

type operatorSummaryRenderOptions struct {
	IncludeWarnings bool
	ExtraLines      []operatorSummaryLine
}

func renderOperatorSummaryBlock(w io.Writer, summary result.SectionSummary, opts operatorSummaryRenderOptions) error {
	lines := []operatorSummaryLine{
		{Label: "Included", Value: summary.Included},
		{Label: "Omitted", Value: summary.Omitted},
		{Label: "Failed", Value: summary.Failed},
	}
	if opts.IncludeWarnings {
		lines = append(lines, operatorSummaryLine{Label: "Warnings", Value: summary.Warnings})
	}
	lines = append(lines, opts.ExtraLines...)
	lines = append(lines,
		operatorSummaryLine{Label: "Included sections", Value: sectionListText(summary.IncludedSections)},
		operatorSummaryLine{Label: "Omitted sections", Value: sectionListText(summary.OmittedSections)},
		operatorSummaryLine{Label: "Failed sections", Value: sectionListText(summary.FailedSections)},
	)

	if _, err := fmt.Fprintln(w, "\nSummary:"); err != nil {
		return err
	}
	for _, line := range lines {
		if _, err := fmt.Fprintf(w, "  %s: %v\n", line.Label, line.Value); err != nil {
			return err
		}
	}

	return nil
}
