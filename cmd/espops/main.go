package main

import (
	"os"

	operationtrace "github.com/lazuale/espocrm-ops/internal/app/operationtrace"
	"github.com/lazuale/espocrm-ops/internal/cli"
	"github.com/lazuale/espocrm-ops/internal/platform/journalstore"
	v3cli "github.com/lazuale/espocrm-ops/internal/v3/cli"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "v3" {
		os.Exit(v3cli.Execute(os.Args[2:], os.Stdout, os.Stderr))
	}

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
