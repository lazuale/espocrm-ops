package backupflow

import (
	"errors"
	"strings"
	"time"

	domainfailure "github.com/lazuale/espocrm-ops/internal/domain/failure"
)

func dedupeStrings(values []string) []string {
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

func executeNow(now func() time.Time) time.Time {
	if now == nil {
		return time.Now().UTC()
	}

	return now().UTC()
}

func normalizeFailure(err error, defaultCode string) error {
	if err == nil {
		return nil
	}

	var failure domainfailure.Failure
	if errors.As(err, &failure) && failure.Kind != "" {
		if strings.TrimSpace(failure.Code) == "" || failure.Code == "operation_execute_failed" {
			failure.Code = defaultCode
		}
		return failure
	}

	return err
}
