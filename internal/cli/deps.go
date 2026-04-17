package cli

import operationusecase "github.com/lazuale/espocrm-ops/internal/usecase/operation"

type JournalWriter interface {
	operationusecase.Writer
}

type JournalWriterFactory func(dir string) JournalWriter

type Dependencies struct {
	Runtime              operationusecase.Runtime
	JournalWriterFactory JournalWriterFactory
}

type App struct {
	runtime              operationusecase.Runtime
	journalWriterFactory JournalWriterFactory
	options              GlobalOptions
}

func NewApp(deps Dependencies) *App {
	runtime := deps.Runtime
	if runtime == nil {
		runtime = operationusecase.DefaultRuntime{}
	}

	journalWriterFactory := deps.JournalWriterFactory
	if journalWriterFactory == nil {
		journalWriterFactory = func(dir string) JournalWriter {
			return operationusecase.DisabledWriter{}
		}
	}

	return &App{
		runtime:              runtime,
		journalWriterFactory: journalWriterFactory,
		options:              defaultGlobalOptions(),
	}
}
