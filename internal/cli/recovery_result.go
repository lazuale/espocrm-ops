package cli

import (
	"github.com/lazuale/espocrm-ops/internal/contract/result"
	operationusecase "github.com/lazuale/espocrm-ops/internal/usecase/operation"
)

func recoveryResultDetails(info operationusecase.RecoveryInfo) *result.RecoveryDetails {
	if !info.Active() {
		return nil
	}

	return &result.RecoveryDetails{
		SourceOperationID: info.SourceOperationID,
		RequestedMode:     info.RequestedMode,
		AppliedMode:       info.AppliedMode,
		Decision:          info.Decision,
		ResumeStep:        info.ResumeStep,
	}
}
