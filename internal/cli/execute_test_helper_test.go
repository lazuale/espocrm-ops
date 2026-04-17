package cli

import (
	"bytes"

	"github.com/lazuale/espocrm-ops/internal/contract/exitcode"
	"github.com/lazuale/espocrm-ops/internal/contract/result"
)

type execOutcome struct {
	Stdout   string
	Stderr   string
	ExitCode int
}

func executeCLI(args ...string) execOutcome {
	return executeCLIWithOptions(nil, args...)
}

func executeCLIWithOptions(opts []testAppOption, args ...string) execOutcome {
	root := newTestRootCmd(opts...)
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	root.SetOut(stdout)
	root.SetErr(stderr)
	root.SetArgs(args)

	err := root.Execute()
	if err == nil {
		return execOutcome{
			Stdout:   stdout.String(),
			Stderr:   stderr.String(),
			ExitCode: exitcode.OK,
		}
	}

	fallbackExitCode := exitcode.InternalError
	fallbackErrorCode := "internal_error"
	if IsUsageError(err) {
		fallbackExitCode = exitcode.UsageError
		fallbackErrorCode = "usage_error"
	}
	errorResult, exitCode := ErrorResult(root.Name(), err, fallbackExitCode, fallbackErrorCode)

	jsonEnabled, _ := root.Flags().GetBool("json")
	if jsonEnabled {
		_ = result.Render(stdout, errorResult, true)
	} else {
		stderr.WriteString("ERROR: " + err.Error() + "\n")
	}

	return execOutcome{
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
		ExitCode: exitCode,
	}
}
