package cli

import (
	"time"

	"github.com/lazuale/espocrm-ops/internal/config"
	"github.com/lazuale/espocrm-ops/internal/ops"
	"github.com/lazuale/espocrm-ops/internal/runtime"
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
			path, err := normalizeManifestPath(manifestPath, "restore")
			if err != nil {
				return err
			}
			cfg, err := config.Load(config.Request{Scope: scope, ProjectDir: projectDir})
			if err != nil {
				return commandFailure("restore", ops.ErrorKindUsage, exitUsage, "restore failed", err, nil)
			}

			result, restoreErr := ops.Restore(cmd.Context(), cfg, path, runtime.DockerCompose{}, restoreNow())
			if restoreErr != nil {
				return opsCommandError("restore", "restore failed", restoreResultFromOps(result), restoreErr)
			}

			return writeJSON(cmd.OutOrStdout(), envelope{
				Command: "restore",
				OK:      true,
				Message: "restore completed",
				Error:   nil,
				Result:  restoreResultFromOps(result),
			})
		},
	}

	cmd.Flags().StringVar(&scope, "scope", "", "restore scope")
	cmd.Flags().StringVar(&projectDir, "project-dir", ".", "project directory containing compose.yaml and .env.<scope>")
	cmd.Flags().StringVar(&manifestPath, "manifest", "", "path to source backup manifest.json")
	return cmd
}

func restoreResultFromOps(result ops.RestoreResult) restoreResult {
	return restoreResult{
		Manifest:         result.Manifest,
		SnapshotManifest: result.SnapshotManifest,
	}
}
