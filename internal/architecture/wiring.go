package architecture

import (
	"github.com/lazuale/espocrm-ops/internal/cli"
	"github.com/lazuale/espocrm-ops/internal/contract/result"
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

func (a *App) IsJSONEnabled() bool {
	if a == nil {
		return false
	}

	return a.cli.IsJSONEnabled()
}

func (a *App) IsUsageError(err error) bool {
	return cli.IsUsageError(err)
}

func (a *App) ErrorResult(command string, err error, fallbackExitCode int, fallbackErrorCode string) (result.Result, int) {
	return cli.ErrorResult(command, err, fallbackExitCode, fallbackErrorCode)
}
