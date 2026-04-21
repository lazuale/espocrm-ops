package cli

import (
	backupapp "github.com/lazuale/espocrm-ops/internal/app/backup"
	backupverifyapp "github.com/lazuale/espocrm-ops/internal/app/backupverify"
	doctorapp "github.com/lazuale/espocrm-ops/internal/app/doctor"
	migrateapp "github.com/lazuale/espocrm-ops/internal/app/migrate"
	operationusecase "github.com/lazuale/espocrm-ops/internal/app/operation"
	lockport "github.com/lazuale/espocrm-ops/internal/app/ports/lockport"
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
	Locks                lockport.Locks
}

type App struct {
	runtime              operationusecase.Runtime
	journalWriterFactory JournalWriterFactory
	backup               backupapp.Service
	backupVerify         backupverifyapp.Service
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

	locks := deps.Locks
	if locks == nil {
		locks = appadapter.Locks{}
	}

	operationService := operationusecase.NewService(operationusecase.Dependencies{
		Env:   appadapter.EnvLoader{},
		Files: appadapter.Files{},
		Locks: locks,
	})
	backupService := backupapp.NewService(backupapp.Dependencies{
		Operations: operationService,
		Env:        appadapter.EnvLoader{},
		Runtime:    appadapter.Runtime{},
		Files:      appadapter.Files{},
		Store:      appadapter.BackupStore{},
	})
	backupVerifyService := backupverifyapp.NewService(backupverifyapp.Dependencies{
		Store: appadapter.BackupStore{},
	})
	restoreService := restoreapp.NewService(restoreapp.Dependencies{
		Operations: operationService,
		Env:        appadapter.EnvLoader{},
		Runtime:    appadapter.Runtime{},
		Files:      appadapter.Files{},
		Locks:      locks,
		Store:      appadapter.BackupStore{},
	})
	migrateService := migrateapp.NewService(migrateapp.Dependencies{
		Operations: operationService,
		Env:        appadapter.EnvLoader{},
		Runtime:    appadapter.Runtime{},
		Files:      appadapter.Files{},
		Locks:      locks,
		Store:      appadapter.BackupStore{},
	})
	doctorService := doctorapp.NewService(doctorapp.Dependencies{
		Env:     appadapter.EnvLoader{},
		Files:   appadapter.Files{},
		Locks:   locks,
		Runtime: appadapter.Runtime{},
	})

	return &App{
		runtime:              runtime,
		journalWriterFactory: journalWriterFactory,
		backup:               backupService,
		backupVerify:         backupVerifyService,
		doctor:               doctorService,
		restore:              restoreService,
		migrate:              migrateService,
		options:              defaultGlobalOptions(),
	}
}
