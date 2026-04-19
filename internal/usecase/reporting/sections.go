package reporting

import "strings"

type SectionCategory int

const (
	SectionIgnored SectionCategory = iota
	SectionIncluded
	SectionOmitted
	SectionFailed
)

type SectionSummary struct {
	IncludedSections []string
	OmittedSections  []string
	FailedSections   []string
	Warnings         []string
}

type SectionCollector struct {
	included []string
	omitted  []string
	failed   []string
	warnings []string
}

func (c *SectionCollector) Add(category SectionCategory, code string, warnings []string) {
	c.warnings = append(c.warnings, warnings...)

	code = strings.TrimSpace(code)
	if code == "" {
		return
	}

	switch category {
	case SectionIncluded:
		c.included = append(c.included, code)
	case SectionOmitted:
		c.omitted = append(c.omitted, code)
	case SectionFailed:
		c.failed = append(c.failed, code)
	}
}

func (c SectionCollector) Finalize(baseWarnings []string) SectionSummary {
	warnings := append([]string(nil), baseWarnings...)
	warnings = append(warnings, c.warnings...)

	return SectionSummary{
		IncludedSections: cloneStrings(c.included),
		OmittedSections:  cloneStrings(c.omitted),
		FailedSections:   cloneStrings(c.failed),
		Warnings:         DedupeStrings(warnings),
	}
}

func DedupeStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}

	seen := map[string]struct{}{}
	items := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		items = append(items, value)
	}

	if len(items) == 0 {
		return nil
	}
	return items
}

func cloneStrings(values []string) []string {
	if values == nil {
		return nil
	}
	return append([]string(nil), values...)
}
