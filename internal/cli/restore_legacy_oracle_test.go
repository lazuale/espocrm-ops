package cli

import (
	"bytes"

	operationapp "github.com/lazuale/espocrm-ops/internal/app/operation"
	restoreusecase "github.com/lazuale/espocrm-ops/internal/app/restore"
	resultbridge "github.com/lazuale/espocrm-ops/internal/cli/resultbridge"
	"github.com/lazuale/espocrm-ops/internal/contract/exitcode"
	"github.com/lazuale/espocrm-ops/internal/contract/result"
	appadapter "github.com/lazuale/espocrm-ops/internal/platform/appadapter"
	backupstoreadapter "github.com/lazuale/espocrm-ops/internal/platform/backupstoreadapter"
	envadapter "github.com/lazuale/espocrm-ops/internal/platform/envadapter"
	runtimeadapter "github.com/lazuale/espocrm-ops/internal/platform/runtimeadapter"
	"github.com/spf13/cobra"
)

func executeRestoreLegacyReferenceCLI(opts []testAppOption, journalDir string, request restoreusecase.ExecuteRequest) execOutcome {
	app, legacy := newLegacyRestoreReferenceHarness(opts...)
	root := newLegacyRestoreReferenceRoot(app, legacy, request)
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	root.SetOut(stdout)
	root.SetErr(stderr)

	root.SetArgs([]string{
		"--journal-dir", journalDir,
		"--json",
		"restore",
	})

	exitCode := ExecuteRoot(root)
	return execOutcome{
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
		ExitCode: exitCode,
	}
}

// Legacy restore больше не живёт в обычном App graph, поэтому oracle/reference
// harness собирает его явно только внутри test-only lane.
func newLegacyRestoreReferenceHarness(opts ...testAppOption) (*App, restoreusecase.Service) {
	cfg := defaultTestAppConfig()
	for _, opt := range opts {
		opt(&cfg)
	}

	app := NewApp(Dependencies{
		Runtime:              cfg.runtime,
		JournalWriterFactory: cfg.journalWriterFactory,
		Locks:                cfg.locks,
	})
	app.options = cfg.options

	locks := cfg.locks
	if locks == nil {
		locks = appadapter.Locks{}
	}

	operationService := operationapp.NewService(operationapp.Dependencies{
		Env:   envadapter.EnvLoader{},
		Files: appadapter.Files{},
		Locks: locks,
	})
	legacy := restoreusecase.NewService(restoreusecase.Dependencies{
		Operations: operationService,
		Env:        envadapter.EnvLoader{},
		Runtime:    runtimeadapter.Runtime{},
		Files:      appadapter.Files{},
		Locks:      locks,
		Store:      backupstoreadapter.BackupStore{},
	})

	return app, legacy
}

func newLegacyRestoreReferenceRoot(app *App, legacy restoreusecase.Service, request restoreusecase.ExecuteRequest) *cobra.Command {
	root := &cobra.Command{
		Use:           "espops",
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			return validateGlobalOptions(app.options)
		},
	}
	bindApp(root, app)
	root.PersistentFlags().BoolVar(&app.options.JSON, "json", false, "output result as JSON")
	root.PersistentFlags().StringVar(&app.options.JournalDir, "journal-dir", app.options.JournalDir, "directory for operation journal entries")

	restoreReq := request
	restoreCmd := &cobra.Command{
		Use:   "restore",
		Short: "Run the legacy restore oracle flow",
		RunE: func(cmd *cobra.Command, args []string) error {
			return RunCommandWithResult(cmd, CommandSpec{
				Name:       "restore",
				ErrorCode:  "restore_failed",
				ExitCode:   exitcode.InternalError,
				RenderText: resultbridge.RenderRestoreText,
			}, func() (result.Result, error) {
				req := restoreReq
				req.LogWriter = cmd.ErrOrStderr()
				req.Now = app.runtime.Now
				info, err := legacy.Execute(req)
				return legacyRestoreCLIResult(info), err
			})
		},
	}
	root.AddCommand(bindApp(restoreCmd, app))
	return root
}

func legacyRestoreCLIResult(info restoreusecase.ExecuteInfo) result.Result {
	planned, completed, skipped, blocked, failed := info.Counts()
	message := "restore completed"
	if info.DryRun {
		message = "restore dry-run plan completed"
	}
	if !info.Ready() {
		if info.DryRun {
			message = "restore dry-run plan found blocking conditions"
		} else {
			message = "restore failed"
		}
	}

	return result.Result{
		Command:  "restore",
		OK:       info.Ready(),
		Message:  message,
		DryRun:   info.DryRun,
		Warnings: append([]string(nil), info.Warnings...),
		Details: result.RestoreDetails{
			Scope:                  info.Scope,
			Ready:                  info.Ready(),
			SelectionMode:          info.SelectionMode,
			SourceKind:             info.SourceKind,
			Steps:                  len(info.Steps),
			Planned:                planned,
			Completed:              completed,
			Skipped:                skipped,
			Blocked:                blocked,
			Failed:                 failed,
			Warnings:               len(info.Warnings),
			SnapshotEnabled:        info.SnapshotEnabled,
			SkipDB:                 info.SkipDB,
			SkipFiles:              info.SkipFiles,
			NoStop:                 info.NoStop,
			NoStart:                info.NoStart,
			AppServicesWereRunning: info.AppServicesWereRunning,
			StartedDBTemporarily:   info.StartedDBTemporarily,
		},
		Artifacts: result.RestoreArtifacts{
			ProjectDir:            info.ProjectDir,
			ComposeFile:           info.ComposeFile,
			EnvFile:               info.EnvFile,
			BackupRoot:            info.BackupRoot,
			ManifestTXT:           info.ManifestTXTPath,
			ManifestJSON:          info.ManifestJSONPath,
			DBBackup:              info.DBBackupPath,
			FilesBackup:           info.FilesBackupPath,
			SnapshotManifestTXT:   info.SnapshotManifestTXT,
			SnapshotManifestJSON:  info.SnapshotManifestJSON,
			SnapshotDBBackup:      info.SnapshotDBBackup,
			SnapshotFilesBackup:   info.SnapshotFilesBackup,
			SnapshotDBChecksum:    info.SnapshotDBChecksum,
			SnapshotFilesChecksum: info.SnapshotFilesChecksum,
		},
		Items: legacyRestoreExecutionItems(info.Steps),
	}
}

func legacyRestoreExecutionItems(steps []restoreusecase.ExecuteStep) []result.ItemPayload {
	items := make([]result.ItemPayload, 0, len(steps))
	for _, step := range steps {
		items = append(items, result.RestoreItem{
			SectionItem: result.SectionItem{
				Code:    step.Code,
				Status:  step.Status.String(),
				Summary: step.Summary,
				Details: step.Details,
				Action:  step.Action,
			},
		})
	}
	return items
}
