package cli

import (
	"bytes"
	"fmt"

	migrateusecase "github.com/lazuale/espocrm-ops/internal/app/migrate"
	operationapp "github.com/lazuale/espocrm-ops/internal/app/operation"
	resultbridge "github.com/lazuale/espocrm-ops/internal/cli/resultbridge"
	"github.com/lazuale/espocrm-ops/internal/contract/exitcode"
	"github.com/lazuale/espocrm-ops/internal/contract/result"
	appadapter "github.com/lazuale/espocrm-ops/internal/platform/appadapter"
	backupstoreadapter "github.com/lazuale/espocrm-ops/internal/platform/backupstoreadapter"
	envadapter "github.com/lazuale/espocrm-ops/internal/platform/envadapter"
	runtimeadapter "github.com/lazuale/espocrm-ops/internal/platform/runtimeadapter"
	"github.com/spf13/cobra"
)

func executeMigrateLegacyReferenceCLI(opts []testAppOption, journalDir string, args ...string) execOutcome {
	app, legacy := newLegacyMigrateReferenceHarness(opts...)
	root := newLegacyMigrateReferenceRoot(app, legacy)
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	root.SetOut(stdout)
	root.SetErr(stderr)

	rootArgs := []string{
		"--journal-dir", journalDir,
		"--json",
		"migrate",
	}
	root.SetArgs(append(rootArgs, args...))

	exitCode := ExecuteRoot(root)
	return execOutcome{
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
		ExitCode: exitCode,
	}
}

// Legacy migrate больше не должен оставаться в production App graph.
// Reference harness держит его только как test-only oracle.
func newLegacyMigrateReferenceHarness(opts ...testAppOption) (*App, migrateusecase.Service) {
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
	legacy := migrateusecase.NewService(migrateusecase.Dependencies{
		Operations: operationService,
		Env:        envadapter.EnvLoader{},
		Runtime:    runtimeadapter.Runtime{},
		Files:      appadapter.Files{},
		Locks:      locks,
		Store:      backupstoreadapter.BackupStore{},
	})

	return app, legacy
}

func newLegacyMigrateReferenceRoot(app *App, legacy migrateusecase.Service) *cobra.Command {
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
	root.AddCommand(bindApp(newLegacyMigrateReferenceCmd(app, legacy), app))
	return root
}

func newLegacyMigrateReferenceCmd(app *App, legacy migrateusecase.Service) *cobra.Command {
	var in migrateInput

	cmd := &cobra.Command{
		Use:   "migrate",
		Short: "Run the legacy migrate oracle flow",
		Args:  noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateLegacyMigrateReferenceInput(cmd, &in); err != nil {
				return err
			}

			return RunCommandWithResult(cmd, CommandSpec{
				Name:       "migrate",
				ErrorCode:  "migrate_failed",
				ExitCode:   exitcode.InternalError,
				RenderText: resultbridge.RenderMigrateText,
			}, func() (result.Result, error) {
				info, err := legacy.Execute(migrateusecase.ExecuteRequest{
					SourceScope: in.fromScope,
					TargetScope: in.toScope,
					ProjectDir:  in.projectDir,
					ComposeFile: in.composeFile,
					DBBackup:    in.dbBackup,
					FilesBackup: in.filesBackup,
					SkipDB:      in.skipDB,
					SkipFiles:   in.skipFiles,
					NoStart:     in.noStart,
					LogWriter:   cmd.ErrOrStderr(),
				})
				return legacyMigrateCLIResult(info), err
			})
		},
	}

	cmd.Flags().StringVar(&in.fromScope, "from", "", "source contour")
	cmd.Flags().StringVar(&in.toScope, "to", "", "target contour")
	cmd.Flags().StringVar(&in.projectDir, "project-dir", ".", "project directory containing the compose file and env files")
	cmd.Flags().StringVar(&in.composeFile, "compose-file", "", "compose file path (defaults to project-dir/compose.yaml)")
	cmd.Flags().StringVar(&in.dbBackup, "db-backup", "", "explicit source database backup path")
	cmd.Flags().StringVar(&in.filesBackup, "files-backup", "", "explicit source files backup path")
	cmd.Flags().BoolVar(&in.skipDB, "skip-db", false, "skip migrating the database backup")
	cmd.Flags().BoolVar(&in.skipFiles, "skip-files", false, "skip migrating the files backup")
	cmd.Flags().BoolVar(&in.noStart, "no-start", false, "leave the target application services stopped after migration")
	cmd.Flags().BoolVar(&in.force, "force", false, "confirm that the destructive migration should run")
	cmd.Flags().StringVar(&in.confirmProd, "confirm-prod", "", "confirm destructive prod migration by passing the literal value `prod`")

	return cmd
}

func validateLegacyMigrateReferenceInput(cmd *cobra.Command, in *migrateInput) error {
	if err := normalizeChoiceFlag("--from", &in.fromScope, "--from must be dev or prod", "dev", "prod"); err != nil {
		return err
	}
	if err := normalizeChoiceFlag("--to", &in.toScope, "--to must be dev or prod", "dev", "prod"); err != nil {
		return err
	}
	if in.fromScope == in.toScope {
		return usageError(fmt.Errorf("source and target contours must differ"))
	}
	if err := normalizeProjectContext(cmd, &in.projectDir, &in.composeFile, nil); err != nil {
		return err
	}
	if err := normalizeConfirmProdFlag(cmd, &in.confirmProd); err != nil {
		return err
	}
	if in.skipDB && in.skipFiles {
		return usageError(fmt.Errorf("nothing to migrate: --skip-db and --skip-files cannot both be set"))
	}
	if in.skipDB && in.dbBackup != "" {
		return usageError(fmt.Errorf("--db-backup cannot be used with --skip-db"))
	}
	if in.skipFiles && in.filesBackup != "" {
		return usageError(fmt.Errorf("--files-backup cannot be used with --skip-files"))
	}
	if err := normalizeOptionalAbsolutePathFlag(cmd, "db-backup", &in.dbBackup); err != nil {
		return err
	}
	if err := normalizeOptionalAbsolutePathFlag(cmd, "files-backup", &in.filesBackup); err != nil {
		return err
	}
	if !in.force {
		return usageError(fmt.Errorf("migrate requires an explicit --force flag"))
	}
	if in.toScope == "prod" && in.confirmProd != "prod" {
		return usageError(fmt.Errorf("prod migration also requires --confirm-prod prod"))
	}
	return nil
}

func legacyMigrateCLIResult(info migrateusecase.ExecuteInfo) result.Result {
	completed, skipped, blocked, failed := info.Counts()
	message := "backup migration completed"
	if !info.Ready() {
		message = "backup migration failed"
	}

	return result.Result{
		Command:  "migrate",
		OK:       info.Ready(),
		Message:  message,
		Warnings: append([]string(nil), info.Warnings...),
		Details: result.MigrateDetails{
			SourceScope:          info.SourceScope,
			TargetScope:          info.TargetScope,
			Ready:                info.Ready(),
			SelectionMode:        info.SelectionMode,
			Steps:                len(info.Steps),
			Completed:            completed,
			Skipped:              skipped,
			Blocked:              blocked,
			Failed:               failed,
			Warnings:             len(info.Warnings),
			SkipDB:               info.SkipDB,
			SkipFiles:            info.SkipFiles,
			NoStart:              info.NoStart,
			StartedDBTemporarily: info.StartedDBTemporarily,
		},
		Artifacts: result.MigrateArtifacts{
			ProjectDir:       info.ProjectDir,
			ComposeFile:      info.ComposeFile,
			SourceEnvFile:    info.SourceEnvFile,
			TargetEnvFile:    info.TargetEnvFile,
			SourceBackupRoot: info.SourceBackupRoot,
			TargetBackupRoot: info.TargetBackupRoot,
			ManifestTXT:      info.ManifestTXTPath,
			ManifestJSON:     info.ManifestJSONPath,
			DBBackup:         info.DBBackupPath,
			FilesBackup:      info.FilesBackupPath,
		},
		Items: legacyMigrateExecutionItems(info.Steps),
	}
}

func legacyMigrateExecutionItems(steps []migrateusecase.ExecuteStep) []result.ItemPayload {
	items := make([]result.ItemPayload, 0, len(steps))
	for _, step := range steps {
		items = append(items, result.MigrateItem{
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
