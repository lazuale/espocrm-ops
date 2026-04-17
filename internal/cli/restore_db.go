package cli

import (
	"fmt"
	"strings"

	"github.com/lazuale/espocrm-ops/internal/contract/exitcode"
	"github.com/lazuale/espocrm-ops/internal/contract/result"
	"github.com/lazuale/espocrm-ops/internal/usecase/restore"
	"github.com/spf13/cobra"
)

func newRestoreDBCmd() *cobra.Command {
	var manifestPath string
	var dbBackup string
	var dbContainer string
	var dbName string
	var dbUser string
	var dbPassword string
	var dbPasswordFile string
	var dbRootPassword string
	var dbRootPasswordFile string
	var dryRun bool

	cmd := &cobra.Command{
		Use:   "restore-db",
		Short: "Restore database from a manifest-backed backup set",
		Args:  noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			in := restoreDBInput{
				manifestPath:       manifestPath,
				dbBackup:           dbBackup,
				dbContainer:        dbContainer,
				dbName:             dbName,
				dbUser:             dbUser,
				dbPassword:         dbPassword,
				dbPasswordFile:     dbPasswordFile,
				dbRootPassword:     dbRootPassword,
				dbRootPasswordFile: dbRootPasswordFile,
				dryRun:             dryRun,
			}
			resolveRestoreDBEnvSecrets(&in)
			if err := validateRestoreDBInput(&in); err != nil {
				return err
			}

			return RunCommand(cmd, CommandSpec{
				Name:      "restore-db",
				ErrorCode: "restore_db_failed",
				ExitCode:  exitcode.RestoreError,
			}, func() (result.Result, error) {
				res := result.Result{
					DryRun: dryRun,
					Artifacts: result.RestoreDBArtifacts{
						Manifest:    in.manifestPath,
						DBBackup:    in.dbBackup,
						DBContainer: in.dbContainer,
						DBName:      in.dbName,
					},
					Details: result.RestoreDBDetails{
						DryRun: in.dryRun,
						DBUser: in.dbUser,
					},
				}

				req := restore.RestoreDBRequest{
					ManifestPath:       in.manifestPath,
					DBBackup:           in.dbBackup,
					DBContainer:        in.dbContainer,
					DBName:             in.dbName,
					DBUser:             in.dbUser,
					DBPassword:         in.dbPassword,
					DBPasswordFile:     in.dbPasswordFile,
					DBRootPassword:     in.dbRootPassword,
					DBRootPasswordFile: in.dbRootPasswordFile,
					DryRun:             in.dryRun,
				}

				plan, err := restore.RestoreDB(req)
				if err != nil {
					return res, err
				}

				res.Details = result.RestoreDBDetails{
					DryRun: in.dryRun,
					DBUser: in.dbUser,
					Plan:   restorePlanDetails(plan.Plan),
				}

				msg := "db restore completed"
				if in.dryRun {
					msg = "db restore dry-run passed"
				}

				var warnings []string
				if !in.dryRun {
					warnings = append(warnings, "database restore is destructive for the target database")
				}

				res.Message = msg
				res.Warnings = warnings

				return res, nil
			})
		},
	}

	cmd.Flags().StringVar(&manifestPath, "manifest", "", "path to backup manifest.json")
	cmd.Flags().StringVar(&dbBackup, "db-backup", "", "path to db backup sql.gz")
	cmd.Flags().StringVar(&dbContainer, "db-container", envDefault("ESPOPS_DB_CONTAINER", ""), "database container name or id")
	cmd.Flags().StringVar(&dbName, "db-name", envDefault("ESPOPS_DB_NAME", envDefault("DB_NAME", "")), "database name")
	cmd.Flags().StringVar(&dbUser, "db-user", envDefault("ESPOPS_DB_USER", envDefault("DB_USER", "")), "database user")
	cmd.Flags().StringVar(&dbPassword, "db-password", "", "database password (prefer --db-password-file or ESPOPS_DB_PASSWORD)")
	cmd.Flags().StringVar(&dbPasswordFile, "db-password-file", envDefault("ESPOPS_DB_PASSWORD_FILE", ""), "path to file with database password")
	cmd.Flags().StringVar(&dbRootPassword, "db-root-password", "", "database root password (prefer --db-root-password-file or ESPOPS_DB_ROOT_PASSWORD)")
	cmd.Flags().StringVar(&dbRootPasswordFile, "db-root-password-file", envDefault("ESPOPS_DB_ROOT_PASSWORD_FILE", ""), "path to file with database root password")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "run preflight and lock checks without restoring")
	_ = cmd.Flags().MarkHidden("db-password")
	_ = cmd.Flags().MarkHidden("db-root-password")

	return cmd
}

type restoreDBInput struct {
	manifestPath       string
	dbBackup           string
	dbContainer        string
	dbName             string
	dbUser             string
	dbPassword         string
	dbPasswordFile     string
	dbRootPassword     string
	dbRootPasswordFile string
	dryRun             bool
}

func validateRestoreDBInput(in *restoreDBInput) error {
	hasManifest := in.manifestPath != ""
	hasDBBackup := in.dbBackup != ""
	switch {
	case hasManifest && hasDBBackup:
		return usageError(fmt.Errorf("use either --manifest or --db-backup, not both"))
	case !hasManifest && !hasDBBackup:
		return usageError(fmt.Errorf("--manifest or --db-backup is required"))
	}
	if err := requireNonBlankFlag("--db-container", in.dbContainer); err != nil {
		return err
	}
	if err := requireNonBlankFlag("--db-name", in.dbName); err != nil {
		return err
	}
	if err := requireNonBlankFlag("--db-user", in.dbUser); err != nil {
		return err
	}

	hasInlinePassword := strings.TrimSpace(in.dbPassword) != ""
	hasPasswordFile := strings.TrimSpace(in.dbPasswordFile) != ""
	if hasInlinePassword && hasPasswordFile {
		return usageError(fmt.Errorf("use either --db-password or --db-password-file, not both"))
	}
	if !hasInlinePassword && !hasPasswordFile {
		return usageError(fmt.Errorf("database password source is required"))
	}
	hasInlineRootPassword := strings.TrimSpace(in.dbRootPassword) != ""
	hasRootPasswordFile := strings.TrimSpace(in.dbRootPasswordFile) != ""
	if hasInlineRootPassword && hasRootPasswordFile {
		return usageError(fmt.Errorf("use either --db-root-password or --db-root-password-file, not both"))
	}
	if !in.dryRun && !hasInlineRootPassword && !hasRootPasswordFile {
		return usageError(fmt.Errorf("database root password source is required"))
	}

	return nil
}

func resolveRestoreDBEnvSecrets(in *restoreDBInput) {
	if strings.TrimSpace(in.dbPassword) == "" && strings.TrimSpace(in.dbPasswordFile) == "" {
		in.dbPassword = envDefault("ESPOPS_DB_PASSWORD", envDefault("DB_PASSWORD", ""))
	}
	if strings.TrimSpace(in.dbRootPassword) == "" && strings.TrimSpace(in.dbRootPasswordFile) == "" {
		in.dbRootPassword = envDefault("ESPOPS_DB_ROOT_PASSWORD", envDefault("DB_ROOT_PASSWORD", ""))
	}
}
