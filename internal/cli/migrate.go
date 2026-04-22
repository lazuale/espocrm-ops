package cli

import (
	"fmt"

	migrateusecase "github.com/lazuale/espocrm-ops/internal/app/migrate"
	resultbridge "github.com/lazuale/espocrm-ops/internal/cli/resultbridge"
	"github.com/lazuale/espocrm-ops/internal/contract/exitcode"
	"github.com/lazuale/espocrm-ops/internal/contract/result"
	"github.com/spf13/cobra"
)

func newMigrateCmd() *cobra.Command {
	var fromScope string
	var toScope string
	var projectDir string
	var composeFile string
	var dbBackup string
	var filesBackup string
	var skipDB bool
	var skipFiles bool
	var noStart bool
	var force bool
	var confirmProd string

	cmd := &cobra.Command{
		Use:   "migrate",
		Short: "Migrate a backup between contours",
		Args:  noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			in := migrateInput{
				fromScope:   fromScope,
				toScope:     toScope,
				projectDir:  projectDir,
				composeFile: composeFile,
				dbBackup:    dbBackup,
				filesBackup: filesBackup,
				skipDB:      skipDB,
				skipFiles:   skipFiles,
				noStart:     noStart,
				force:       force,
				confirmProd: confirmProd,
			}
			if err := validateMigrateInput(cmd, &in); err != nil {
				return err
			}

			return runMigrateExecute(cmd, in)
		},
	}

	cmd.Flags().StringVar(&fromScope, "from", "", "source contour")
	cmd.Flags().StringVar(&toScope, "to", "", "target contour")
	cmd.Flags().StringVar(&projectDir, "project-dir", ".", "project directory containing the compose file and env files")
	cmd.Flags().StringVar(&composeFile, "compose-file", "", "compose file path (defaults to project-dir/compose.yaml)")
	cmd.Flags().StringVar(&dbBackup, "db-backup", "", "explicit source database backup path")
	cmd.Flags().StringVar(&filesBackup, "files-backup", "", "explicit source files backup path")
	cmd.Flags().BoolVar(&skipDB, "skip-db", false, "skip migrating the database backup")
	cmd.Flags().BoolVar(&skipFiles, "skip-files", false, "skip migrating the files backup")
	cmd.Flags().BoolVar(&noStart, "no-start", false, "leave the target application services stopped after migration")
	cmd.Flags().BoolVar(&force, "force", false, "confirm that the destructive migration should run")
	cmd.Flags().StringVar(&confirmProd, "confirm-prod", "", "confirm destructive prod migration by passing the literal value `prod`")

	return cmd
}

type migrateInput struct {
	fromScope   string
	toScope     string
	projectDir  string
	composeFile string
	dbBackup    string
	filesBackup string
	skipDB      bool
	skipFiles   bool
	noStart     bool
	force       bool
	confirmProd string
}

func validateMigrateInput(cmd *cobra.Command, in *migrateInput) error {
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

func runMigrateExecute(cmd *cobra.Command, in migrateInput) error {
	app := appForCommand(cmd)
	return RunCommandWithResult(cmd, CommandSpec{
		Name:       "migrate",
		ErrorCode:  "migrate_failed",
		ExitCode:   exitcode.InternalError,
		RenderText: resultbridge.RenderMigrateText,
	}, func() (result.Result, error) {
		info, err := app.migrate.Execute(migrateusecase.ExecuteRequest{
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

		return resultbridge.MigrateResult(info), err
	})
}
