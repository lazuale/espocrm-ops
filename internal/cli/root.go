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
		Short:         "EspoCRM backup and recovery utilities",
		Args:          cobra.ArbitraryArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		CompletionOptions: cobra.CompletionOptions{
			DisableDefaultCmd: true,
		},
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

	backupCmd := bindApp(newBackupCmd(), a)
	backupCmd.AddCommand(bindApp(newBackupVerifyCmd(), a))

	cmd.AddCommand(
		bindApp(newDoctorCmd(), a),
		backupCmd,
		bindApp(newRestoreCmd(), a),
		bindApp(newMigrateCmd(), a),
	)

	return cmd
}

func (a *App) JSONEnabled() bool {
	if a == nil {
		return false
	}

	return a.options.JSON
}

func bindApp(cmd *cobra.Command, app *App) *cobra.Command {
	if app == nil {
		panic("cli: bindApp requires non-nil app")
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

	panic("cli: command app is not bound")
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
