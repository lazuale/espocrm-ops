package cli

import (
	"fmt"
	"time"

	v3config "github.com/lazuale/espocrm-ops/internal/v3/config"
	"github.com/lazuale/espocrm-ops/internal/v3/ops"
	v3runtime "github.com/lazuale/espocrm-ops/internal/v3/runtime"
	"github.com/spf13/cobra"
)

var migrateNow = func() time.Time {
	return time.Now().UTC()
}

type migrateResult struct {
	Manifest         string `json:"manifest,omitempty"`
	SnapshotManifest string `json:"snapshot_manifest,omitempty"`
}

func newMigrateCmd() *cobra.Command {
	var fromScope string
	var toScope string
	var projectDir string
	var manifestPath string

	cmd := &cobra.Command{
		Use:   "migrate",
		Short: "Migrate from a verified backup manifest into another scope",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			path, err := normalizeRestoreManifestPath(manifestPath)
			if err != nil {
				return migrateUsageError(err)
			}

			targetCfg, err := loadMigrateTargetConfig(toScope, projectDir)
			if err != nil {
				return migrateUsageError(err)
			}

			result, migrateErr := ops.Migrate(cmd.Context(), fromScope, targetCfg, path, v3runtime.DockerCompose{}, migrateNow())
			if migrateErr != nil {
				return migrateCommandError(result, migrateErr)
			}

			return writeJSON(cmd.OutOrStdout(), envelope{
				Command:  "migrate",
				OK:       true,
				Message:  "migrate completed",
				Error:    nil,
				Warnings: []string{},
				Result: migrateResult{
					Manifest:         result.Manifest,
					SnapshotManifest: result.SnapshotManifest,
				},
			})
		},
	}

	cmd.Flags().StringVar(&fromScope, "from-scope", "", "source backup scope")
	cmd.Flags().StringVar(&toScope, "to-scope", "", "target restore scope")
	cmd.Flags().StringVar(&projectDir, "project-dir", ".", "project directory containing the compose file and env files")
	cmd.Flags().StringVar(&manifestPath, "manifest", "", "path to source backup manifest.json")
	return cmd
}

func migrateUsageError(err error) error {
	return &commandError{
		command:  "migrate",
		kind:     ops.ErrorKindUsage,
		exitCode: exitUsage,
		message:  "migrate failed",
		err:      err,
	}
}

func loadMigrateTargetConfig(scope, projectDir string) (v3config.BackupConfig, error) {
	if scope == "" {
		return v3config.BackupConfig{}, fmt.Errorf("--to-scope is required")
	}
	return v3config.LoadBackup(v3config.BackupRequest{
		Scope:      scope,
		ProjectDir: projectDir,
	})
}

func migrateCommandError(result ops.MigrateResult, err error) error {
	verifyErr, ok := err.(*ops.VerifyError)
	if !ok {
		return &commandError{
			command:  "migrate",
			kind:     ops.ErrorKindIO,
			exitCode: exitIO,
			message:  "migrate failed",
			err:      err,
			result: migrateResult{
				Manifest:         result.Manifest,
				SnapshotManifest: result.SnapshotManifest,
			},
		}
	}

	return &commandError{
		command:  "migrate",
		kind:     verifyErr.Kind,
		exitCode: backupExitCode(verifyErr.Kind),
		message:  "migrate failed",
		err:      verifyErr,
		result: migrateResult{
			Manifest:         result.Manifest,
			SnapshotManifest: result.SnapshotManifest,
		},
	}
}
