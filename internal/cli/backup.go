package cli

import (
	"fmt"
	"io"
	"strings"

	backupusecase "github.com/lazuale/espocrm-ops/internal/app/backup"
	"github.com/lazuale/espocrm-ops/internal/contract/exitcode"
	"github.com/lazuale/espocrm-ops/internal/contract/result"
	"github.com/spf13/cobra"
)

func newBackupCmd() *cobra.Command {
	var scope string
	var projectDir string
	var composeFile string
	var envFile string
	var skipDB bool
	var skipFiles bool
	var noStop bool

	cmd := &cobra.Command{
		Use:   "backup",
		Short: "Create a coherent backup set",
		Args:  noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			in := backupInput{
				scope:       scope,
				projectDir:  projectDir,
				composeFile: composeFile,
				envFile:     envFile,
				skipDB:      skipDB,
				skipFiles:   skipFiles,
				noStop:      noStop,
			}
			if err := validateBackupInput(cmd, &in); err != nil {
				return err
			}

			return runBackup(cmd, in)
		},
	}

	cmd.Flags().StringVar(&scope, "scope", "", "backup contour")
	cmd.Flags().StringVar(&projectDir, "project-dir", ".", "project directory containing the compose file and env files")
	cmd.Flags().StringVar(&composeFile, "compose-file", "", "compose file path (defaults to project-dir/compose.yaml)")
	cmd.Flags().StringVar(&envFile, "env-file", "", "override env file path")
	cmd.Flags().BoolVar(&skipDB, "skip-db", false, "skip database backup")
	cmd.Flags().BoolVar(&skipFiles, "skip-files", false, "skip files backup")
	cmd.Flags().BoolVar(&noStop, "no-stop", false, "do not stop application services before backup")

	return cmd
}

type backupInput struct {
	scope       string
	projectDir  string
	composeFile string
	envFile     string
	skipDB      bool
	skipFiles   bool
	noStop      bool
}

func validateBackupInput(cmd *cobra.Command, in *backupInput) error {
	if err := normalizeContourFlag("--scope", &in.scope); err != nil {
		return err
	}
	if err := normalizeProjectContext(cmd, &in.projectDir, &in.composeFile, &in.envFile); err != nil {
		return err
	}

	if in.skipDB && in.skipFiles {
		return usageError(fmt.Errorf("nothing to back up: --skip-db and --skip-files cannot both be set"))
	}

	return nil
}

func runBackup(cmd *cobra.Command, in backupInput) error {
	return RunCommand(cmd, CommandSpec{
		Name:       "backup",
		ErrorCode:  "backup_failed",
		ExitCode:   exitcode.InternalError,
		RenderText: renderBackupText,
	}, func() (res result.Result, err error) {
		info, err := appForCommand(cmd).backup.Execute(backupusecase.Request{
			Scope:           in.scope,
			ProjectDir:      in.projectDir,
			ComposeFile:     in.composeFile,
			EnvFileOverride: in.envFile,
			SkipDB:          in.skipDB,
			SkipFiles:       in.skipFiles,
			NoStop:          in.noStop,
			Now:             appForCommand(cmd).runtime.Now,
		})
		if err != nil {
			res = backupResult(info)
			return res, err
		}

		return backupResult(info), nil
	})
}

func backupResult(info backupusecase.ExecuteInfo) result.Result {
	completed, skipped, blocked, failed := info.Counts()
	message := "backup completed"
	if !info.Ready() {
		message = "backup failed"
	}

	items := make([]any, 0, len(info.Steps))
	for _, step := range info.Steps {
		items = append(items, result.BackupItem{
			SectionItem: newSectionItem(step.Code, step.Status, step.Summary, step.Details, step.Action),
		})
	}

	return result.Result{
		Command:  "backup",
		OK:       info.Ready(),
		Message:  message,
		Warnings: append([]string(nil), info.Warnings...),
		Details: result.BackupDetails{
			Scope:                  info.Scope,
			Ready:                  info.Ready(),
			CreatedAt:              info.CreatedAt,
			Steps:                  len(info.Steps),
			Completed:              completed,
			Skipped:                skipped,
			Blocked:                blocked,
			Failed:                 failed,
			Warnings:               len(info.Warnings),
			SkipDB:                 info.SkipDB,
			SkipFiles:              info.SkipFiles,
			NoStop:                 info.NoStop,
			ConsistentSnapshot:     info.ConsistentSnapshot,
			AppServicesWereRunning: info.AppServicesWereRunning,
			RetentionDays:          info.RetentionDays,
		},
		Artifacts: result.BackupArtifacts{
			ProjectDir:    info.ProjectDir,
			ComposeFile:   info.ComposeFile,
			EnvFile:       info.EnvFile,
			BackupRoot:    info.BackupRoot,
			ManifestTXT:   info.ManifestTXTPath,
			ManifestJSON:  info.ManifestJSONPath,
			DBBackup:      info.DBBackupPath,
			FilesBackup:   info.FilesBackupPath,
			DBChecksum:    info.DBSidecarPath,
			FilesChecksum: info.FilesSidecarPath,
		},
		Items: items,
	}
}

func renderBackupText(w io.Writer, res result.Result) error {
	details, ok := res.Details.(result.BackupDetails)
	if !ok {
		return result.Render(w, res, false)
	}

	artifacts, ok := res.Artifacts.(result.BackupArtifacts)
	if !ok {
		return fmt.Errorf("unexpected backup artifacts type %T", res.Artifacts)
	}

	if _, err := fmt.Fprintln(w, "EspoCRM backup"); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "Contour: %s\n", details.Scope); err != nil {
		return err
	}
	if strings.TrimSpace(artifacts.EnvFile) != "" {
		if _, err := fmt.Fprintf(w, "Env file: %s\n", artifacts.EnvFile); err != nil {
			return err
		}
	}
	if strings.TrimSpace(artifacts.BackupRoot) != "" {
		if _, err := fmt.Fprintf(w, "Backup root: %s\n", artifacts.BackupRoot); err != nil {
			return err
		}
	}
	if strings.TrimSpace(details.CreatedAt) != "" {
		if _, err := fmt.Fprintf(w, "Created at: %s\n", details.CreatedAt); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintln(w, "Summary:"); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "  Ready:        %t\n", details.Ready); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "  Steps:        %d\n", details.Steps); err != nil {
		return err
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
	if _, err := fmt.Fprintf(w, "  Warnings:     %d\n", details.Warnings); err != nil {
		return err
	}

	if _, err := fmt.Fprintln(w, "\nArtifacts:"); err != nil {
		return err
	}
	if strings.TrimSpace(artifacts.DBBackup) != "" {
		if _, err := fmt.Fprintf(w, "  DB backup:    %s\n", artifacts.DBBackup); err != nil {
			return err
		}
	}
	if strings.TrimSpace(artifacts.FilesBackup) != "" {
		if _, err := fmt.Fprintf(w, "  Files backup: %s\n", artifacts.FilesBackup); err != nil {
			return err
		}
	}
	if strings.TrimSpace(artifacts.DBChecksum) != "" {
		if _, err := fmt.Fprintf(w, "  DB checksum:  %s\n", artifacts.DBChecksum); err != nil {
			return err
		}
	}
	if strings.TrimSpace(artifacts.FilesChecksum) != "" {
		if _, err := fmt.Fprintf(w, "  Files checksum: %s\n", artifacts.FilesChecksum); err != nil {
			return err
		}
	}
	if strings.TrimSpace(artifacts.ManifestTXT) != "" {
		if _, err := fmt.Fprintf(w, "  Text manifest: %s\n", artifacts.ManifestTXT); err != nil {
			return err
		}
	}
	if strings.TrimSpace(artifacts.ManifestJSON) != "" {
		if _, err := fmt.Fprintf(w, "  JSON manifest: %s\n", artifacts.ManifestJSON); err != nil {
			return err
		}
	}

	return renderStepItemsBlock(w, res.Items, backupExecutionItem, stepRenderOptions{
		Title:      "Steps",
		StatusText: upperStatusText,
	})
}

func backupExecutionItem(raw any) (result.SectionItem, error) {
	item, ok := raw.(result.BackupItem)
	if !ok {
		return result.SectionItem{}, fmt.Errorf("unexpected backup item type %T", raw)
	}

	return item.SectionItem, nil
}
