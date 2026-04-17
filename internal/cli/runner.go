package cli

import (
	"fmt"
	"io"

	"github.com/lazuale/espocrm-ops/internal/contract/result"
	operationusecase "github.com/lazuale/espocrm-ops/internal/usecase/operation"
	"github.com/spf13/cobra"
)

type CommandSpec struct {
	Name       string
	ErrorCode  string
	ExitCode   int
	RenderText func(io.Writer, result.Result) error
}

type exitCodedError interface {
	error
	ExitCode() int
}

func RunCommand(cmd *cobra.Command, spec CommandSpec, fn func() (result.Result, error)) error {
	app := appForCommand(cmd)
	exec := operationusecase.Begin(app.runtime, app.journalWriterFactory(app.options.JournalDir), spec.Name)

	res, err := fn()

	if err != nil {
		exitCode := codeForError(err, spec.ExitCode)
		errCode := errorCodeForError(err, spec.ErrorCode)
		warnings := []string{}
		if journalErr := exec.FinishFailure(res, err, errCode); journalErr != nil {
			message := fmt.Sprintf("failed to write journal entry: %v", journalErr)
			if app.IsJSONEnabled() {
				warnings = append(warnings, message)
			} else {
				fmt.Fprintf(cmd.ErrOrStderr(), "WARNING: %s\n", message)
			}
		}

		return CodeError{
			Code:     exitCode,
			Err:      err,
			ErrCode:  errCode,
			Warnings: warnings,
		}
	}

	res, finishErr := exec.FinishSuccess(res)
	if finishErr != nil {
		return finishErr
	}

	if !app.IsJSONEnabled() && spec.RenderText != nil {
		return spec.RenderText(cmd.OutOrStdout(), res)
	}

	return result.Render(cmd.OutOrStdout(), res, app.IsJSONEnabled())
}
