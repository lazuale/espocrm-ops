package cli

import (
	backupapp "github.com/lazuale/espocrm-ops/internal/app/backup"
	doctorapp "github.com/lazuale/espocrm-ops/internal/app/doctor"
	migrateapp "github.com/lazuale/espocrm-ops/internal/app/migrate"
	operationusecase "github.com/lazuale/espocrm-ops/internal/app/operation"
	restoreapp "github.com/lazuale/espocrm-ops/internal/app/restore"
	appadapter "github.com/lazuale/espocrm-ops/internal/platform/appadapter"
)

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
	backup               backupapp.Service
	doctor               doctorapp.Service
	restore              restoreapp.Service
	migrate              migrateapp.Service
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

	operationService := operationusecase.NewService(operationusecase.Dependencies{
		Env:   appadapter.EnvLoader{},
		Files: appadapter.Files{},
		Locks: appadapter.Locks{},
	})
	backupService := backupapp.NewService(backupapp.Dependencies{
		Operations: operationService,
		Env:        appadapter.EnvLoader{},
		Runtime:    appadapter.Runtime{},
		Files:      appadapter.Files{},
		Store:      appadapter.BackupStore{},
	})
	restoreService := restoreapp.NewService(restoreapp.Dependencies{
		Operations: operationService,
		Backup:     backupService,
		Env:        appadapter.EnvLoader{},
		Runtime:    appadapter.Runtime{},
		Files:      appadapter.Files{},
		Locks:      appadapter.Locks{},
		Store:      appadapter.BackupStore{},
	})
	migrateService := migrateapp.NewService(migrateapp.Dependencies{
		Operations: operationService,
		Restore:    restoreService,
		Env:        appadapter.EnvLoader{},
		Runtime:    appadapter.Runtime{},
		Store:      appadapter.BackupStore{},
	})
	doctorService := doctorapp.NewService(doctorapp.Dependencies{
		Env:     appadapter.EnvLoader{},
		Files:   appadapter.Files{},
		Locks:   appadapter.Locks{},
		Runtime: appadapter.Runtime{},
	})

	return &App{
		runtime:              runtime,
		journalWriterFactory: journalWriterFactory,
		backup:               backupService,
		doctor:               doctorService,
		restore:              restoreService,
		migrate:              migrateService,
		options:              defaultGlobalOptions(),
	}
}
