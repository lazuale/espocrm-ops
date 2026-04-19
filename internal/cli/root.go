package cli

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

type appContextKey struct{}

func NewRootCmd() *cobra.Command {
	return NewApp(Dependencies{}).NewRootCmd()
}

func NewRootCmdWithDeps(deps Dependencies) *cobra.Command {
	return NewApp(deps).NewRootCmd()
}

func (a *App) NewRootCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:           "espops",
		Short:         "EspoCRM ops core utilities",
		Args:          cobra.ArbitraryArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				return usageError(fmt.Errorf("unknown command %q for %q", args[0], cmd.CommandPath()))
			}

			return cmd.Help()
		},
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			return validateGlobalOptions(a.options)
		},
	}

	bindApp(cmd, a)

	cmd.PersistentFlags().BoolVar(&a.options.JSON, "json", false, "output result as JSON")
	cmd.PersistentFlags().StringVar(&a.options.JournalDir, "journal-dir", a.options.JournalDir, "directory for operation journal entries")

	cmd.SetFlagErrorFunc(func(_ *cobra.Command, err error) error {
		return usageError(err)
	})

	cmd.AddCommand(
		bindApp(newDoctorCmd(), a),
		bindApp(newUpdateCmd(), a),
		bindApp(newRollbackCmd(), a),
		bindApp(newRestoreCmd(), a),
		bindApp(newMigrateBackupCmd(), a),
		bindApp(newUpdatePlanCmd(), a),
		bindApp(newRollbackPlanCmd(), a),
		bindApp(newBackupCmd(), a),
		bindApp(newBackupExecuteCmd(), a),
		bindApp(newBackupAuditCmd(), a),
		bindApp(newBackupCatalogCmd(), a),
		bindApp(newShowBackupCmd(), a),
		bindApp(newRunOperationCmd(), a),
		bindApp(newVerifyBackupCmd(), a),
		bindApp(newRestoreFilesCmd(), a),
		bindApp(newRestoreDBCmd(), a),
		bindApp(newUpdateBackupCmd(), a),
		bindApp(newUpdateRuntimeCmd(), a),
		bindApp(newHistoryCmd(), a),
		bindApp(newLastOperationCmd(), a),
		bindApp(newShowOperationCmd(), a),
		bindApp(newExportOperationCmd(), a),
		bindApp(newJournalPruneCmd(), a),
	)

	return cmd
}

func (a *App) IsJSONEnabled() bool {
	if a == nil {
		return false
	}

	return a.options.JSON
}

func bindApp(cmd *cobra.Command, app *App) *cobra.Command {
	if app == nil {
		app = NewApp(Dependencies{})
	}

	base := context.Background()
	if current := cmd.Context(); current != nil {
		base = current
	}

	cmd.SetContext(context.WithValue(base, appContextKey{}, app))
	return cmd
}

func appForCommand(cmd *cobra.Command) *App {
	if cmd != nil {
		if ctx := cmd.Context(); ctx != nil {
			if app, ok := ctx.Value(appContextKey{}).(*App); ok && app != nil {
				return app
			}
		}
	}

	return NewApp(Dependencies{})
}

func validateGlobalOptions(opts GlobalOptions) error {
	if strings.TrimSpace(opts.JournalDir) == "" {
		return requiredFlagError("--journal-dir")
	}

	cleanJournalDir := filepath.Clean(opts.JournalDir)
	if cleanJournalDir == "." {
		return usageError(fmt.Errorf("--journal-dir must not be the current directory"))
	}
	if cleanJournalDir == string(filepath.Separator) {
		return usageError(fmt.Errorf("--journal-dir must not be the filesystem root"))
	}

	return nil
}
