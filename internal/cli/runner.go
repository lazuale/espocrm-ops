package cli

import (
	"fmt"
	"io"

	"github.com/lazuale/espocrm-ops/internal/contract/result"
	operationusecase "github.com/lazuale/espocrm-ops/internal/app/operation"
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
				if _, writeErr := fmt.Fprintf(cmd.ErrOrStderr(), "WARNING: %s\n", message); writeErr != nil {
					warnings = append(warnings, fmt.Sprintf("failed to render warning %q: %v", message, writeErr))
				}
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

func finishJournaledCommandSuccess(cmd *cobra.Command, spec CommandSpec, exec operationusecase.Execution, res result.Result) error {
	finished, err := exec.FinishSuccess(res)
	if err != nil {
		return err
	}

	return renderCommandResult(cmd, spec, finished)
}

func finishJournaledCommandFailure(cmd *cobra.Command, spec CommandSpec, exec operationusecase.Execution, res result.Result, err error) error {
	res.Command = spec.Name
	errCode := errorCodeForError(err, spec.ErrorCode)

	if journalErr := exec.FinishFailure(res, err, errCode); journalErr != nil {
		message := fmt.Sprintf("failed to write journal entry: %v", journalErr)
		if appForCommand(cmd).JSONEnabled() {
			res.Warnings = append(res.Warnings, message)
		} else if _, writeErr := fmt.Fprintf(cmd.ErrOrStderr(), "WARNING: %s\n", message); writeErr != nil {
			res.Warnings = append(res.Warnings, fmt.Sprintf("failed to render warning %q: %v", message, writeErr))
		}
	}

	codeErr := CodeError{
		Code:     codeForError(err, spec.ExitCode),
		Err:      err,
		ErrCode:  errCode,
		Warnings: warningMessages(err),
	}

	if appForCommand(cmd).JSONEnabled() {
		return ResultCodeError{
			CodeError: codeErr,
			Result:    res,
		}
	}

	if spec.RenderText != nil {
		if err := spec.RenderText(cmd.OutOrStdout(), res); err != nil {
			return err
		}
	} else if err := result.Render(cmd.OutOrStdout(), res, false); err != nil {
		return err
	}

	if err := renderWarnings(cmd.OutOrStdout(), res.Warnings); err != nil {
		return err
	}

	return silentCodeError{CodeError: codeErr}
}

func renderWarnings(w io.Writer, warnings []string) error {
	for _, warning := range warnings {
		if _, err := fmt.Fprintf(w, "WARNING: %s\n", warning); err != nil {
			return err
		}
	}

	return nil
}
