package architecture

import (
	"github.com/lazuale/espocrm-ops/internal/cli"
	"github.com/lazuale/espocrm-ops/internal/platform/journalstore"
	operationusecase "github.com/lazuale/espocrm-ops/internal/usecase/operation"
	"github.com/spf13/cobra"
)

type App struct {
	cli *cli.App
}

func NewApp() *App {
	return &App{
		cli: cli.NewApp(cli.Dependencies{
			Runtime: operationusecase.DefaultRuntime{},
			JournalWriterFactory: func(dir string) cli.JournalWriter {
				return journalstore.FSWriter{Dir: dir}
			},
		}),
	}
}

func (a *App) RootCmd() *cobra.Command {
	if a == nil {
		return NewApp().RootCmd()
	}

	return a.cli.NewRootCmd()
}

func (a *App) ExecuteRoot(root *cobra.Command) int {
	return cli.ExecuteRoot(root)
}
