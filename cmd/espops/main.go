package main

import (
	"os"

	operationtrace "github.com/lazuale/espocrm-ops/internal/app/operationtrace"
	"github.com/lazuale/espocrm-ops/internal/cli"
	"github.com/lazuale/espocrm-ops/internal/platform/journalstore"
)

func main() {
	app := cli.NewApp(cli.Dependencies{
		Runtime: operationtrace.DefaultRuntime{},
		JournalWriterFactory: func(dir string) cli.JournalWriter {
			return journalstore.FSWriter{Dir: dir}
		},
	})
	root := app.NewRootCmd()
	root.SetOut(os.Stdout)
	root.SetErr(os.Stderr)
	os.Exit(cli.ExecuteRoot(root))
}
