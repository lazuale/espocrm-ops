package cli

import (
	contractresult "github.com/lazuale/espocrm-ops/internal/contract/result"
	restoreusecase "github.com/lazuale/espocrm-ops/internal/usecase/restore"
)

func restorePlanDetails(plan restoreusecase.RestorePlan) contractresult.RestorePlanDetails {
	return contractresult.RestorePlanDetails{
		SourceKind:  plan.SourceKind,
		SourcePath:  plan.SourcePath,
		Destructive: plan.Destructive,
		Changes:     append([]string(nil), plan.Changes...),
		NonChanges:  append([]string(nil), plan.NonChanges...),
		Checks:      restorePlanChecks(plan.Checks),
		NextStep:    plan.NextStep,
	}
}

func restorePlanChecks(checks []restoreusecase.RestorePlanCheck) []contractresult.RestorePlanCheck {
	items := make([]contractresult.RestorePlanCheck, 0, len(checks))
	for _, check := range checks {
		items = append(items, contractresult.RestorePlanCheck{
			Name:    check.Name,
			Status:  check.Status,
			Details: check.Details,
		})
	}

	return items
}
