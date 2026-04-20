package main

import (
	"os"

	"github.com/lazuale/espocrm-ops/internal/cli"
	"github.com/lazuale/espocrm-ops/internal/platform/journalstore"
	operationusecase "github.com/lazuale/espocrm-ops/internal/usecase/operation"
)

func main() {
	app := cli.NewApp(cli.Dependencies{
		Runtime: operationusecase.DefaultRuntime{},
		JournalWriterFactory: func(dir string) cli.JournalWriter {
			return journalstore.FSWriter{Dir: dir}
		},
	})
	root := app.NewRootCmd()
	root.SetOut(os.Stdout)
	root.SetErr(os.Stderr)
	os.Exit(cli.ExecuteRoot(root))
}
