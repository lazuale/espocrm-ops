package cli

import (
	"context"
	"fmt"
	"strings"
	"time"

	config "github.com/lazuale/espocrm-ops/internal/config"
	"github.com/lazuale/espocrm-ops/internal/ops"
	runtime "github.com/lazuale/espocrm-ops/internal/runtime"
	"github.com/spf13/cobra"
)

var smokeNow = func() time.Time {
	return time.Now().UTC()
}

type smokeStep struct {
	Name string `json:"name"`
	OK   bool   `json:"ok"`
}

type smokeResult struct {
	FromScope               string      `json:"from_scope,omitempty"`
	ToScope                 string      `json:"to_scope,omitempty"`
	Manifest                string      `json:"manifest,omitempty"`
	RestoreSnapshotManifest string      `json:"restore_snapshot_manifest,omitempty"`
	MigrateSnapshotManifest string      `json:"migrate_snapshot_manifest,omitempty"`
	FailedStep              string      `json:"failed_step,omitempty"`
	Steps                   []smokeStep `json:"steps,omitempty"`
}

func newSmokeCmd() *cobra.Command {
	var fromScope string
	var toScope string
	var projectDir string

	cmd := &cobra.Command{
		Use:   "smoke",
		Short: "Run the fixed smoke flow across two scopes",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			result, err := runSmoke(cmd.Context(), fromScope, toScope, projectDir, runtime.DockerCompose{})
			if err != nil {
				return smokeCommandError(result, err)
			}

			return writeJSON(cmd.OutOrStdout(), envelope{
				Command:  "smoke",
				OK:       true,
				Message:  "smoke completed",
				Error:    nil,
				Warnings: []string{},
				Result:   result,
			})
		},
	}

	cmd.Flags().StringVar(&fromScope, "from-scope", "", "source smoke scope")
	cmd.Flags().StringVar(&toScope, "to-scope", "", "target smoke scope")
	cmd.Flags().StringVar(&projectDir, "project-dir", ".", "project directory containing the compose file and env files")
	return cmd
}

func runSmoke(ctx context.Context, fromScope, toScope, projectDir string, rt runtime.DockerCompose) (smokeResult, error) {
	sourceCfg, targetCfg, err := loadSmokeConfigs(fromScope, toScope, projectDir)
	if err != nil {
		return smokeResult{}, err
	}

	result := smokeResult{
		FromScope: sourceCfg.Scope,
		ToScope:   targetCfg.Scope,
		Steps:     make([]smokeStep, 0, 6),
	}
	nextStepTime := smokeStepTimeline(smokeNow())

	if _, err := ops.Doctor(ctx, config.BackupRequest{
		Scope:      sourceCfg.Scope,
		ProjectDir: sourceCfg.ProjectDir,
	}, rt); err != nil {
		return smokeStepFailure(result, "doctor source", err)
	}
	result.Steps = append(result.Steps, smokeStep{Name: "doctor source", OK: true})

	if _, err := ops.Doctor(ctx, config.BackupRequest{
		Scope:      targetCfg.Scope,
		ProjectDir: targetCfg.ProjectDir,
	}, rt); err != nil {
		return smokeStepFailure(result, "doctor target", err)
	}
	result.Steps = append(result.Steps, smokeStep{Name: "doctor target", OK: true})

	backupResult, err := ops.Backup(ctx, sourceCfg, rt, nextStepTime())
	result.Manifest = backupResult.Manifest
	if err != nil {
		return smokeStepFailure(result, "backup", err)
	}
	result.Steps = append(result.Steps, smokeStep{Name: "backup", OK: true})

	verifyResult, err := ops.VerifyBackup(ctx, result.Manifest)
	if err != nil {
		return smokeStepFailure(result, "backup verify", err)
	}
	result.Manifest = verifyResult.Manifest
	result.Steps = append(result.Steps, smokeStep{Name: "backup verify", OK: true})

	restoreResult, err := ops.Restore(ctx, targetCfg, result.Manifest, rt, nextStepTime())
	result.RestoreSnapshotManifest = restoreResult.SnapshotManifest
	if err != nil {
		return smokeStepFailure(result, "restore", err)
	}
	result.Steps = append(result.Steps, smokeStep{Name: "restore", OK: true})

	migrateResult, err := ops.Migrate(ctx, sourceCfg.Scope, targetCfg, result.Manifest, rt, nextStepTime())
	result.MigrateSnapshotManifest = migrateResult.SnapshotManifest
	if err != nil {
		return smokeStepFailure(result, "migrate", err)
	}
	result.Steps = append(result.Steps, smokeStep{Name: "migrate", OK: true})

	return result, nil
}

func loadSmokeConfigs(fromScope, toScope, projectDir string) (config.BackupConfig, config.BackupConfig, error) {
	sourceCfg, err := loadSmokeScopeConfig("--from-scope", fromScope, projectDir)
	if err != nil {
		return config.BackupConfig{}, config.BackupConfig{}, err
	}

	targetCfg, err := loadSmokeScopeConfig("--to-scope", toScope, projectDir)
	if err != nil {
		return config.BackupConfig{}, config.BackupConfig{}, err
	}

	if sourceCfg.Scope == targetCfg.Scope {
		return config.BackupConfig{}, config.BackupConfig{}, &ops.VerifyError{
			Kind:    ops.ErrorKindUsage,
			Message: "--from-scope and --to-scope must differ",
		}
	}

	return sourceCfg, targetCfg, nil
}

func loadSmokeScopeConfig(flagName, scope, projectDir string) (config.BackupConfig, error) {
	scope = strings.TrimSpace(scope)
	switch scope {
	case "":
		return config.BackupConfig{}, &ops.VerifyError{
			Kind:    ops.ErrorKindUsage,
			Message: flagName + " is required",
		}
	case "dev", "prod":
	default:
		return config.BackupConfig{}, &ops.VerifyError{
			Kind:    ops.ErrorKindUsage,
			Message: flagName + " must be dev or prod",
		}
	}

	cfg, err := config.LoadBackup(config.BackupRequest{
		Scope:      scope,
		ProjectDir: projectDir,
	})
	if err != nil {
		return config.BackupConfig{}, &ops.VerifyError{
			Kind:    ops.ErrorKindUsage,
			Message: err.Error(),
			Err:     err,
		}
	}

	return cfg, nil
}

func smokeStepTimeline(base time.Time) func() time.Time {
	base = base.UTC()
	var offset time.Duration

	return func() time.Time {
		current := base.Add(offset)
		offset += time.Second
		return current
	}
}

func smokeStepFailure(result smokeResult, step string, err error) (smokeResult, error) {
	result.FailedStep = step
	result.Steps = append(result.Steps, smokeStep{Name: step, OK: false})

	verifyErr, ok := err.(*ops.VerifyError)
	if !ok {
		return result, fmt.Errorf("%s: %w", step, err)
	}

	return result, &ops.VerifyError{
		Kind:    verifyErr.Kind,
		Message: step,
		Err:     verifyErr,
	}
}

func smokeCommandError(result smokeResult, err error) error {
	verifyErr, ok := err.(*ops.VerifyError)
	if !ok {
		return &commandError{
			command:  "smoke",
			kind:     ops.ErrorKindIO,
			exitCode: exitIO,
			message:  "smoke failed",
			err:      err,
			result:   result,
		}
	}

	return &commandError{
		command:  "smoke",
		kind:     verifyErr.Kind,
		exitCode: backupExitCode(verifyErr.Kind),
		message:  "smoke failed",
		err:      verifyErr,
		result:   result,
	}
}
