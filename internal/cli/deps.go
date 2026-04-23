package cli

import (
	v2app "github.com/lazuale/espocrm-ops/internal/app"
	doctorapp "github.com/lazuale/espocrm-ops/internal/app/doctor"
	operationapp "github.com/lazuale/espocrm-ops/internal/app/operation"
	operationtrace "github.com/lazuale/espocrm-ops/internal/app/operationtrace"
	lockport "github.com/lazuale/espocrm-ops/internal/app/ports/lockport"
	appadapter "github.com/lazuale/espocrm-ops/internal/platform/appadapter"
	envadapter "github.com/lazuale/espocrm-ops/internal/platform/envadapter"
	runtimeadapter "github.com/lazuale/espocrm-ops/internal/platform/runtimeadapter"
	v2runtime "github.com/lazuale/espocrm-ops/internal/runtime"
	v2store "github.com/lazuale/espocrm-ops/internal/store"
)

type JournalWriter interface {
	operationtrace.Writer
}

type JournalWriterFactory func(dir string) JournalWriter

type Dependencies struct {
	Runtime              operationtrace.Runtime
	JournalWriterFactory JournalWriterFactory
	Locks                lockport.Locks
}

type App struct {
	runtime              operationtrace.Runtime
	journalWriterFactory JournalWriterFactory
	backup               v2app.BackupCommandService
	backupVerify         v2app.BackupVerifyService
	doctor               doctorapp.Service
	restore              v2app.RestoreCommandService
	migrate              v2app.MigrateCommandService
	options              GlobalOptions
}

func NewApp(deps Dependencies) *App {
	runtime := deps.Runtime
	if runtime == nil {
		runtime = operationtrace.DefaultRuntime{}
	}

	journalWriterFactory := deps.JournalWriterFactory
	if journalWriterFactory == nil {
		journalWriterFactory = func(dir string) JournalWriter {
			return operationtrace.DisabledWriter{}
		}
	}

	locks := deps.Locks
	if locks == nil {
		locks = appadapter.Locks{}
	}

	operationService := operationapp.NewService(operationapp.Dependencies{
		Env:   envadapter.EnvLoader{},
		Files: appadapter.Files{},
		Locks: locks,
	})
	backupService := v2app.NewBackupCommandService(v2app.BackupCommandDependencies{
		Operations: operationService,
		Core: v2app.NewBackupService(v2app.BackupDependencies{
			Runtime: v2runtime.Docker{},
			Store:   v2store.FileStore{},
		}),
	})
	backupVerifyService := v2app.NewBackupVerifyService(v2app.BackupVerifyDependencies{
		Store: v2store.FileStore{},
	})
	restoreCommandService := v2app.NewRestoreCommandService(v2app.RestoreCommandDependencies{
		Operations: operationService,
		Env:        envadapter.EnvLoader{},
		Core: v2app.NewRestoreService(v2app.RestoreDependencies{
			Runtime: v2runtime.Docker{},
			Store:   v2store.FileStore{},
			Snapshot: v2app.NewBackupService(v2app.BackupDependencies{
				Runtime: v2runtime.Docker{},
				Store:   v2store.FileStore{},
			}),
		}),
	})
	migrateService := v2app.NewMigrateCommandService(v2app.MigrateCommandDependencies{
		Operations: operationService,
		Env:        envadapter.EnvLoader{},
		Core: v2app.NewMigrateService(v2app.MigrateDependencies{
			Runtime: v2runtime.Docker{},
			Store:   v2store.FileStore{},
			Snapshot: v2app.NewBackupService(v2app.BackupDependencies{
				Runtime: v2runtime.Docker{},
				Store:   v2store.FileStore{},
			}),
		}),
	})
	doctorService := doctorapp.NewService(doctorapp.Dependencies{
		Env:     envadapter.EnvLoader{},
		Files:   appadapter.Files{},
		Locks:   locks,
		Runtime: runtimeadapter.Runtime{},
	})

	return &App{
		runtime:              runtime,
		journalWriterFactory: journalWriterFactory,
		backup:               backupService,
		backupVerify:         backupVerifyService,
		doctor:               doctorService,
		restore:              restoreCommandService,
		migrate:              migrateService,
		options:              defaultGlobalOptions(),
	}
}
