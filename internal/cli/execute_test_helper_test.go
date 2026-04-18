package cli

import (
	"bytes"
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

	exitCode := ExecuteRoot(root)

	return execOutcome{
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
		ExitCode: exitCode,
	}
}
