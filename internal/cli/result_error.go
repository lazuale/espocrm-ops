package cli

import "github.com/lazuale/espocrm-ops/internal/contract/result"

type resultCarrier interface {
	CommandResult() result.Result
}

type ResultCodeError struct {
	CodeError
	Result result.Result
}

func (e ResultCodeError) CommandResult() result.Result {
	return e.Result
}
