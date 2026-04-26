package cli

import (
	"encoding/json"
	"errors"
	"io"

	"github.com/spf13/cobra"
)

const (
	exitOK         = 0
	exitUsage      = 2
	exitManifest   = 3
	exitValidation = 4
	exitRuntime    = 5
	exitIO         = 6
	exitInternal   = 10
)

type envelope struct {
	Command string        `json:"command"`
	OK      bool          `json:"ok"`
	Message string        `json:"message"`
	Error   *errorPayload `json:"error"`
	Result  any           `json:"result"`
}

type errorPayload struct {
	Kind    string `json:"kind"`
	Message string `json:"message"`
}

type commandError struct {
	command  string
	kind     string
	exitCode int
	message  string
	result   any
	err      error
}

func (e *commandError) Error() string {
	if e == nil {
		return ""
	}
	if e.err == nil {
		return e.message
	}
	if e.message == "" {
		return e.err.Error()
	}
	return e.message + ": " + e.err.Error()
}

func NewRootCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:           "espops",
		Short:         "EspoCRM backup and restore utilities",
		SilenceUsage:  true,
		SilenceErrors: true,
		CompletionOptions: cobra.CompletionOptions{
			DisableDefaultCmd: true,
		},
	}

	backupCmd := newBackupCmd()
	backupCmd.AddCommand(newBackupVerifyCmd())
	cmd.AddCommand(newDoctorCmd())
	cmd.AddCommand(backupCmd)
	cmd.AddCommand(newRestoreCmd())

	return cmd
}

func Execute(args []string, stdout, stderr io.Writer) int {
	root := NewRootCmd()
	root.SetOut(stdout)
	root.SetErr(stderr)
	root.SetArgs(args)

	if err := root.Execute(); err != nil {
		return renderExecutionError(root.OutOrStdout(), err)
	}
	return exitOK
}

func writeJSON(w io.Writer, value any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(value)
}

func renderExecutionError(w io.Writer, err error) int {
	var cmdErr *commandError
	if errors.As(err, &cmdErr) {
		message := cmdErr.message
		if message == "" {
			message = "command failed"
		}
		_ = writeJSON(w, envelope{
			Command: cmdErr.command,
			OK:      false,
			Message: message,
			Error:   &errorPayload{Kind: cmdErr.kind, Message: cmdErr.messageForOperator()},
			Result:  cmdErr.resultValue(),
		})
		return cmdErr.exitCode
	}

	_ = writeJSON(w, envelope{
		Command: "espops",
		OK:      false,
		Message: "espops command failed",
		Error:   &errorPayload{Kind: "internal", Message: err.Error()},
		Result:  struct{}{},
	})
	return exitInternal
}

func (e *commandError) messageForOperator() string {
	if e == nil {
		return "command failed"
	}
	if e.err == nil {
		if e.message != "" {
			return e.message
		}
		return "command failed"
	}
	return e.err.Error()
}

func (e *commandError) resultValue() any {
	if e == nil || e.result == nil {
		return struct{}{}
	}
	return e.result
}
