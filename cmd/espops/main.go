package main

import (
	"fmt"
	"os"

	"github.com/lazuale/espocrm-ops/internal/architecture"
	"github.com/lazuale/espocrm-ops/internal/contract/exitcode"
	"github.com/lazuale/espocrm-ops/internal/contract/result"
)

func main() {
	app := architecture.NewApp()
	root := app.RootCmd()
	if err := root.Execute(); err != nil {
		fallbackExitCode := exitcode.InternalError
		fallbackErrorCode := "internal_error"
		if app.IsUsageError(err) {
			fallbackExitCode = exitcode.UsageError
			fallbackErrorCode = "usage_error"
		}

		if app.IsJSONEnabled() {
			errorResult, exitCode := app.ErrorResult(root.Name(), err, fallbackExitCode, fallbackErrorCode)
			_ = result.Render(os.Stdout, errorResult, true)
			os.Exit(exitCode)
		}

		fmt.Fprintln(os.Stderr, "ERROR:", err)
		_, exitCode := app.ErrorResult(root.Name(), err, fallbackExitCode, fallbackErrorCode)
		os.Exit(exitCode)
	}
}
