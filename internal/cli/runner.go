package cli

import (
	"fmt"
	"io"

	journalbridge "github.com/lazuale/espocrm-ops/internal/cli/journalbridge"
	operationtrace "github.com/lazuale/espocrm-ops/internal/app/operationtrace"
	"github.com/lazuale/espocrm-ops/internal/contract/result"
	"github.com/spf13/cobra"
)

type CommandSpec struct {
	Name       string
	ErrorCode  string
	ExitCode   int
	RenderText func(io.Writer, result.Result) error
}

type commandRunOptions struct {
	journal     bool
	resultError bool
}

type exitCodedError interface {
	error
	ExitCode() int
}

func RunCommand(cmd *cobra.Command, spec CommandSpec, fn func() (result.Result, error)) error {
	return runCommand(cmd, spec, fn, commandRunOptions{journal: true})
}

func RunCommandWithResult(cmd *cobra.Command, spec CommandSpec, fn func() (result.Result, error)) error {
	return runCommand(cmd, spec, fn, commandRunOptions{
		journal:     true,
		resultError: true,
	})
}

func RunDiagnosticCommand(cmd *cobra.Command, spec CommandSpec, fn func() (result.Result, error)) error {
	return runCommand(cmd, spec, fn, commandRunOptions{resultError: true})
}

func runCommand(cmd *cobra.Command, spec CommandSpec, fn func() (result.Result, error), opts commandRunOptions) error {
	app := appForCommand(cmd)
	var exec operationtrace.Execution
	if opts.journal {
		exec = operationtrace.Begin(app.runtime, app.journalWriterFactory(app.options.JournalDir), spec.Name)
	}

	res, err := fn()

	if err != nil {
		return commandFailure(cmd, app, spec, opts, exec, res, err)
	}

	if opts.journal {
		completion, finishErr := exec.FinishSuccess(journalbridge.RecordFromResult(&res))
		if finishErr != nil {
			return finishErr
		}
		journalbridge.ApplyExecutionCompletion(&res, completion)
	}

	return renderCommandResult(cmd, spec, res)
}

func commandFailure(cmd *cobra.Command, app *App, spec CommandSpec, opts commandRunOptions, exec operationtrace.Execution, res result.Result, err error) error {
	res.Command = spec.Name
	errCode := errorCodeForError(err, spec.ErrorCode)

	warnings := []string{}
	if opts.journal {
		warnings = finishJournalFailure(cmd, app, exec, &res, err, errCode, opts.resultError)
	}

	if opts.resultError {
		return renderCommandFailureResult(cmd, spec, res, err, errCode)
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

func finishJournalFailure(cmd *cobra.Command, app *App, exec operationtrace.Execution, res *result.Result, err error, errCode string, resultError bool) []string {
	if journalErr := exec.FinishFailure(journalbridge.RecordFromResult(res), err, errCode); journalErr != nil {
		message := fmt.Sprintf("failed to write journal entry: %v", journalErr)
		if resultError {
			appendCommandWarning(cmd, app, &res.Warnings, message)
			return nil
		}
		warnings := []string{}
		appendCommandWarning(cmd, app, &warnings, message)
		return warnings
	}

	return nil
}

func renderCommandFailureResult(cmd *cobra.Command, spec CommandSpec, res result.Result, err error, errCode string) error {
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

	if err := renderCommandResult(cmd, spec, res); err != nil {
		return err
	}

	return silentCodeError{CodeError: codeErr}
}

func appendCommandWarning(cmd *cobra.Command, app *App, warnings *[]string, message string) {
	if app.JSONEnabled() {
		*warnings = append(*warnings, message)
		return
	}
	if _, writeErr := fmt.Fprintf(cmd.ErrOrStderr(), "WARNING: %s\n", message); writeErr != nil {
		*warnings = append(*warnings, fmt.Sprintf("failed to render warning %q: %v", message, writeErr))
	}
}

func renderWarnings(w io.Writer, warnings []string) error {
	for _, warning := range warnings {
		if _, err := fmt.Fprintf(w, "WARNING: %s\n", warning); err != nil {
			return err
		}
	}

	return nil
}
