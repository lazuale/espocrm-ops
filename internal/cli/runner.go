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

type commandRunMode int

const (
	commandRunModeWithoutJournal commandRunMode = iota
	commandRunModeWithJournal
)

type exitCodedError interface {
	error
	ExitCode() int
}

func RunCommand(cmd *cobra.Command, spec CommandSpec, fn func() (result.Result, error)) error {
	return runCommandWithMode(cmd, spec, fn, commandRunModeWithJournal)
}

func RunResultCommand(cmd *cobra.Command, spec CommandSpec, fn func() (result.Result, error)) error {
	return runCommandWithMode(cmd, spec, fn, commandRunModeWithoutJournal)
}

func runCommandWithMode(cmd *cobra.Command, spec CommandSpec, fn func() (result.Result, error), mode commandRunMode) error {
	app := appForCommand(cmd)
	var exec operationusecase.Execution
	if mode == commandRunModeWithJournal {
		exec = operationusecase.Begin(app.runtime, app.journalWriterFactory(app.options.JournalDir), spec.Name)
	}

	res, err := fn()

	if err != nil {
		return commandFailure(cmd, app, spec, mode, exec, res, err)
	}

	if mode == commandRunModeWithJournal {
		var finishErr error
		res, finishErr = exec.FinishSuccess(res)
		if finishErr != nil {
			return finishErr
		}
	}

	return renderCommandResult(cmd, spec, res)
}

func commandFailure(cmd *cobra.Command, app *App, spec CommandSpec, mode commandRunMode, exec operationusecase.Execution, res result.Result, err error) error {
	errCode := errorCodeForError(err, spec.ErrorCode)
	warnings := []string{}

	if mode == commandRunModeWithJournal {
		if journalErr := exec.FinishFailure(res, err, errCode); journalErr != nil {
			message := fmt.Sprintf("failed to write journal entry: %v", journalErr)
			if app.JSONEnabled() {
				warnings = append(warnings, message)
			} else {
				fmt.Fprintf(cmd.ErrOrStderr(), "WARNING: %s\n", message)
			}
		}
	}

	return CodeError{
		Code:     codeForError(err, spec.ExitCode),
		Err:      err,
		ErrCode:  errCode,
		Warnings: warnings,
	}
}

func renderCommandResult(cmd *cobra.Command, spec CommandSpec, res result.Result) error {
	res.Command = spec.Name
	if !appForCommand(cmd).JSONEnabled() && spec.RenderText != nil {
		if err := spec.RenderText(cmd.OutOrStdout(), res); err != nil {
			return err
		}
		return renderWarnings(cmd.OutOrStdout(), res.Warnings)
	}

	return result.Render(cmd.OutOrStdout(), res, appForCommand(cmd).JSONEnabled())
}
