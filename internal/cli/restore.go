package cli

import (
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/lazuale/espocrm-ops/internal/contract/exitcode"
	"github.com/lazuale/espocrm-ops/internal/contract/result"
	operationusecase "github.com/lazuale/espocrm-ops/internal/usecase/operation"
	restoreusecase "github.com/lazuale/espocrm-ops/internal/usecase/restore"
	"github.com/spf13/cobra"
)

func newRestoreCmd() *cobra.Command {
	var scope string
	var projectDir string
	var composeFile string
	var envFile string
	var manifestPath string
	var dbBackup string
	var filesBackup string
	var skipDB bool
	var skipFiles bool
	var noSnapshot bool
	var snapshotBeforeRestore bool
	var noStop bool
	var noStart bool
	var dryRun bool
	var force bool
	var confirmProd string

	cmd := &cobra.Command{
		Use:   "restore",
		Short: "Run the canonical restore flow",
		Args:  noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			in := restoreInput{
				scope:                 scope,
				projectDir:            projectDir,
				composeFile:           composeFile,
				envFile:               envFile,
				manifestPath:          manifestPath,
				dbBackup:              dbBackup,
				filesBackup:           filesBackup,
				skipDB:                skipDB,
				skipFiles:             skipFiles,
				noSnapshot:            noSnapshot,
				snapshotBeforeRestore: snapshotBeforeRestore,
				noStop:                noStop,
				noStart:               noStart,
				dryRun:                dryRun,
				force:                 force,
				confirmProd:           confirmProd,
			}
			if err := validateRestoreInput(cmd, &in); err != nil {
				return err
			}
			return runRestore(cmd, in)
		},
	}

	cmd.Flags().StringVar(&scope, "scope", "", "restore contour")
	cmd.Flags().StringVar(&projectDir, "project-dir", ".", "project directory containing the compose file and env files")
	cmd.Flags().StringVar(&composeFile, "compose-file", "", "compose file path (defaults to project-dir/compose.yaml)")
	cmd.Flags().StringVar(&envFile, "env-file", "", "override env file path")
	cmd.Flags().StringVar(&manifestPath, "manifest", "", "path to manifest json anchoring the restore source")
	cmd.Flags().StringVar(&dbBackup, "db-backup", "", "path to the database backup to restore")
	cmd.Flags().StringVar(&filesBackup, "files-backup", "", "path to the files backup to restore")
	cmd.Flags().BoolVar(&skipDB, "skip-db", false, "skip the database restore step")
	cmd.Flags().BoolVar(&skipFiles, "skip-files", false, "skip the files restore step")
	cmd.Flags().BoolVar(&noSnapshot, "no-snapshot", false, "skip the pre-restore emergency recovery point")
	cmd.Flags().BoolVar(&snapshotBeforeRestore, "snapshot-before-restore", false, "keep the pre-restore recovery point enabled explicitly")
	cmd.Flags().BoolVar(&noStop, "no-stop", false, "do not stop application services before restore")
	cmd.Flags().BoolVar(&noStart, "no-start", false, "leave application services stopped after restore")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "show what restore would do without making changes")
	cmd.Flags().BoolVar(&force, "force", false, "confirm that the destructive restore should run")
	cmd.Flags().StringVar(&confirmProd, "confirm-prod", "", "confirm destructive prod restore by passing the literal value `prod`")

	return cmd
}

type restoreInput struct {
	scope                 string
	projectDir            string
	composeFile           string
	envFile               string
	manifestPath          string
	dbBackup              string
	filesBackup           string
	skipDB                bool
	skipFiles             bool
	noSnapshot            bool
	snapshotBeforeRestore bool
	noStop                bool
	noStart               bool
	dryRun                bool
	force                 bool
	confirmProd           string
}

func validateRestoreInput(cmd *cobra.Command, in *restoreInput) error {
	in.scope = strings.TrimSpace(in.scope)
	in.projectDir = strings.TrimSpace(in.projectDir)
	in.composeFile = strings.TrimSpace(in.composeFile)
	in.envFile = strings.TrimSpace(in.envFile)
	in.manifestPath = strings.TrimSpace(in.manifestPath)
	in.dbBackup = strings.TrimSpace(in.dbBackup)
	in.filesBackup = strings.TrimSpace(in.filesBackup)
	in.confirmProd = strings.TrimSpace(in.confirmProd)

	switch in.scope {
	case "dev", "prod":
	default:
		return usageError(fmt.Errorf("--scope must be dev or prod"))
	}

	if err := requireNonBlankFlag("--project-dir", in.projectDir); err != nil {
		return err
	}

	projectAbs, err := filepath.Abs(filepath.Clean(in.projectDir))
	if err != nil {
		return usageError(fmt.Errorf("resolve --project-dir: %w", err))
	}
	in.projectDir = projectAbs

	if err := normalizeOptionalStringFlag(cmd, "compose-file", &in.composeFile); err != nil {
		return err
	}
	if in.composeFile == "" {
		in.composeFile = filepath.Join(in.projectDir, "compose.yaml")
	} else if !filepath.IsAbs(in.composeFile) {
		in.composeFile = filepath.Join(in.projectDir, in.composeFile)
	}
	in.composeFile = filepath.Clean(in.composeFile)

	if err := normalizeOptionalStringFlag(cmd, "env-file", &in.envFile); err != nil {
		return err
	}
	if in.envFile != "" && !filepath.IsAbs(in.envFile) {
		in.envFile = filepath.Join(in.projectDir, in.envFile)
	}
	if in.envFile != "" {
		in.envFile = filepath.Clean(in.envFile)
	}

	if err := normalizeOptionalStringFlag(cmd, "manifest", &in.manifestPath); err != nil {
		return err
	}
	if in.manifestPath != "" {
		manifestAbs, err := filepath.Abs(filepath.Clean(in.manifestPath))
		if err != nil {
			return usageError(fmt.Errorf("resolve --manifest: %w", err))
		}
		in.manifestPath = manifestAbs
	}

	if in.dbBackup != "" {
		dbAbs, err := filepath.Abs(filepath.Clean(in.dbBackup))
		if err != nil {
			return usageError(fmt.Errorf("resolve --db-backup: %w", err))
		}
		in.dbBackup = dbAbs
	}
	if in.filesBackup != "" {
		filesAbs, err := filepath.Abs(filepath.Clean(in.filesBackup))
		if err != nil {
			return usageError(fmt.Errorf("resolve --files-backup: %w", err))
		}
		in.filesBackup = filesAbs
	}

	if in.skipDB && in.skipFiles {
		return usageError(fmt.Errorf("nothing to restore: --skip-db and --skip-files cannot both be set"))
	}
	if in.manifestPath != "" && (in.dbBackup != "" || in.filesBackup != "") {
		return usageError(fmt.Errorf("use either --manifest or direct backup flags, not both"))
	}

	switch {
	case in.manifestPath != "":
	case in.skipDB:
		if in.filesBackup == "" {
			return usageError(fmt.Errorf("--files-backup is required when restore keeps only the files step"))
		}
		if in.dbBackup != "" {
			return usageError(fmt.Errorf("--db-backup cannot be used with --skip-db"))
		}
	case in.skipFiles:
		if in.dbBackup == "" {
			return usageError(fmt.Errorf("--db-backup is required when restore keeps only the database step"))
		}
		if in.filesBackup != "" {
			return usageError(fmt.Errorf("--files-backup cannot be used with --skip-files"))
		}
	default:
		if in.dbBackup == "" || in.filesBackup == "" {
			return usageError(fmt.Errorf("pass both --db-backup and --files-backup together, or use --manifest"))
		}
	}

	if in.noSnapshot && in.snapshotBeforeRestore {
		return usageError(fmt.Errorf("--snapshot-before-restore cannot be combined with --no-snapshot"))
	}
	if in.confirmProd != "" && in.confirmProd != "prod" {
		return usageError(fmt.Errorf("--confirm-prod accepts only the value prod"))
	}
	if !in.dryRun {
		if !in.force {
			return usageError(fmt.Errorf("restore requires an explicit --force flag"))
		}
		if in.scope == "prod" && in.confirmProd != "prod" {
			return usageError(fmt.Errorf("prod restore also requires --confirm-prod prod"))
		}
	}

	return nil
}

func runRestore(cmd *cobra.Command, in restoreInput) error {
	spec := CommandSpec{
		Name:       "restore",
		ErrorCode:  "restore_failed",
		ExitCode:   exitcode.InternalError,
		RenderText: renderRestoreText,
	}
	exec := operationusecase.Begin(
		appForCommand(cmd).runtime,
		appForCommand(cmd).journalWriterFactory(appForCommand(cmd).options.JournalDir),
		spec.Name,
	)

	info, err := restoreusecase.Execute(restoreusecase.ExecuteRequest{
		Scope:           in.scope,
		ProjectDir:      in.projectDir,
		ComposeFile:     in.composeFile,
		EnvFileOverride: in.envFile,
		EnvContourHint:  envFileContourHint(),
		ManifestPath:    in.manifestPath,
		DBBackup:        in.dbBackup,
		FilesBackup:     in.filesBackup,
		SkipDB:          in.skipDB,
		SkipFiles:       in.skipFiles,
		NoSnapshot:      in.noSnapshot,
		NoStop:          in.noStop,
		NoStart:         in.noStart,
		DryRun:          in.dryRun,
		LogWriter:       cmd.ErrOrStderr(),
	})

	res := restoreResult(info)
	res.Command = spec.Name

	if err != nil {
		return finishJournaledCommandFailure(cmd, spec, exec, res, err)
	}
	return finishJournaledCommandSuccess(cmd, spec, exec, res)
}

func restoreResult(info restoreusecase.ExecuteInfo) result.Result {
	wouldRun, completed, skipped, blocked, failed := info.Counts()
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
		Details: result.RestoreExecutionDetails{
			Scope:                  info.Scope,
			Ready:                  info.Ready(),
			SelectionMode:          info.SelectionMode,
			SourceKind:             info.SourceKind,
			Steps:                  len(info.Steps),
			WouldRun:               wouldRun,
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
		Artifacts: result.RestoreExecutionArtifacts{
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
		Items: restoreExecutionItems(info.Steps),
	}
}

func renderRestoreText(w io.Writer, res result.Result) error {
	details, ok := res.Details.(result.RestoreExecutionDetails)
	if !ok {
		return result.Render(w, res, false)
	}
	artifacts, ok := res.Artifacts.(result.RestoreExecutionArtifacts)
	if !ok {
		return fmt.Errorf("unexpected restore artifacts type %T", res.Artifacts)
	}

	if _, err := fmt.Fprintln(w, "EspoCRM restore"); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "Contour: %s\n", details.Scope); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "Env file: %s\n", artifacts.EnvFile); err != nil {
		return err
	}
	if strings.TrimSpace(details.SelectionMode) != "" {
		if _, err := fmt.Fprintf(w, "Selection: %s\n", details.SelectionMode); err != nil {
			return err
		}
	}
	if strings.TrimSpace(artifacts.ManifestJSON) != "" {
		if _, err := fmt.Fprintf(w, "Manifest JSON: %s\n", artifacts.ManifestJSON); err != nil {
			return err
		}
	}
	if strings.TrimSpace(artifacts.DBBackup) != "" {
		if _, err := fmt.Fprintf(w, "DB backup: %s\n", artifacts.DBBackup); err != nil {
			return err
		}
	}
	if strings.TrimSpace(artifacts.FilesBackup) != "" {
		if _, err := fmt.Fprintf(w, "Files backup: %s\n", artifacts.FilesBackup); err != nil {
			return err
		}
	}

	if _, err := fmt.Fprintln(w, "\nSummary:"); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "  Ready:        %t\n", details.Ready); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "  Steps:        %d\n", details.Steps); err != nil {
		return err
	}
	if details.WouldRun != 0 || res.DryRun {
		if _, err := fmt.Fprintf(w, "  Would run:    %d\n", details.WouldRun); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintf(w, "  Completed:    %d\n", details.Completed); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "  Skipped:      %d\n", details.Skipped); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "  Blocked:      %d\n", details.Blocked); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "  Failed:       %d\n", details.Failed); err != nil {
		return err
	}

	if strings.TrimSpace(artifacts.SnapshotManifestJSON) != "" {
		if _, err := fmt.Fprintln(w, "\nEmergency Recovery Point:"); err != nil {
			return err
		}
		if strings.TrimSpace(artifacts.SnapshotManifestJSON) != "" {
			if _, err := fmt.Fprintf(w, "  Manifest JSON: %s\n", artifacts.SnapshotManifestJSON); err != nil {
				return err
			}
		}
		if strings.TrimSpace(artifacts.SnapshotDBBackup) != "" {
			if _, err := fmt.Fprintf(w, "  DB backup:     %s\n", artifacts.SnapshotDBBackup); err != nil {
				return err
			}
		}
		if strings.TrimSpace(artifacts.SnapshotFilesBackup) != "" {
			if _, err := fmt.Fprintf(w, "  Files backup:  %s\n", artifacts.SnapshotFilesBackup); err != nil {
				return err
			}
		}
	}

	if err := renderStepItemsBlock(w, res.Items, restoreExecutionItem, stepRenderOptions{
		Title:            "Steps",
		IgnoreUnexpected: true,
		StatusText:       upperStatusText,
	}); err != nil {
		return err
	}

	return nil
}
