package cli

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	config "github.com/lazuale/espocrm-ops/internal/config"
	"github.com/lazuale/espocrm-ops/internal/ops"
	runtime "github.com/lazuale/espocrm-ops/internal/runtime"
	"github.com/spf13/cobra"
)

var restoreNow = func() time.Time {
	return time.Now().UTC()
}

type restoreResult struct {
	Manifest         string `json:"manifest,omitempty"`
	SnapshotManifest string `json:"snapshot_manifest,omitempty"`
}

func newRestoreCmd() *cobra.Command {
	var scope string
	var projectDir string
	var manifestPath string

	cmd := &cobra.Command{
		Use:   "restore",
		Short: "Restore from a verified backup manifest",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			path, err := normalizeRestoreManifestPath(manifestPath)
			if err != nil {
				return err
			}

			cfg, err := loadRestoreConfig(scope, projectDir)
			if err != nil {
				return &commandError{
					command:  "restore",
					kind:     ops.ErrorKindUsage,
					exitCode: exitUsage,
					message:  "restore failed",
					err:      err,
				}
			}

			result, restoreErr := ops.Restore(cmd.Context(), cfg, path, runtime.DockerCompose{}, restoreNow())
			if restoreErr != nil {
				return restoreCommandError(result, restoreErr)
			}

			return writeJSON(cmd.OutOrStdout(), envelope{
				Command:  "restore",
				OK:       true,
				Message:  "restore completed",
				Error:    nil,
				Warnings: combineWarnings(cfg.Warnings, result.Warnings),
				Result: restoreResult{
					Manifest:         result.Manifest,
					SnapshotManifest: result.SnapshotManifest,
				},
			})
		},
	}

	cmd.Flags().StringVar(&scope, "scope", "", "restore contour")
	cmd.Flags().StringVar(&projectDir, "project-dir", ".", "project directory containing the compose file and env files")
	cmd.Flags().StringVar(&manifestPath, "manifest", "", "path to source backup manifest.json")
	return cmd
}

func normalizeRestoreManifestPath(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", &commandError{
			command:  "restore",
			kind:     ops.ErrorKindUsage,
			exitCode: exitUsage,
			message:  "restore failed",
			err:      fmt.Errorf("--manifest is required"),
		}
	}

	abs, err := filepath.Abs(filepath.Clean(value))
	if err != nil {
		return "", &commandError{
			command:  "restore",
			kind:     ops.ErrorKindUsage,
			exitCode: exitUsage,
			message:  "restore failed",
			err:      fmt.Errorf("resolve --manifest: %w", err),
		}
	}
	return abs, nil
}

func restoreCommandError(result ops.RestoreResult, err error) error {
	verifyErr, ok := err.(*ops.VerifyError)
	if !ok {
		return &commandError{
			command:  "restore",
			kind:     ops.ErrorKindIO,
			exitCode: exitIO,
			message:  "restore failed",
			err:      err,
			result: restoreResult{
				Manifest:         result.Manifest,
				SnapshotManifest: result.SnapshotManifest,
			},
		}
	}

	return &commandError{
		command:  "restore",
		kind:     verifyErr.Kind,
		exitCode: backupExitCode(verifyErr.Kind),
		message:  "restore failed",
		err:      verifyErr,
		result: restoreResult{
			Manifest:         result.Manifest,
			SnapshotManifest: result.SnapshotManifest,
		},
	}
}

func loadRestoreConfig(scope, projectDir string) (config.BackupConfig, error) {
	if scope == "" {
		return config.BackupConfig{}, fmt.Errorf("--scope is required")
	}
	return config.LoadRestore(config.BackupRequest{
		Scope:      scope,
		ProjectDir: projectDir,
	})
}
