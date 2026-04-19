package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/lazuale/espocrm-ops/internal/contract/exitcode"
	"github.com/lazuale/espocrm-ops/internal/contract/result"
	operationusecase "github.com/lazuale/espocrm-ops/internal/usecase/operation"
	restoreusecase "github.com/lazuale/espocrm-ops/internal/usecase/restore"
	"github.com/spf13/cobra"
)

func newRestoreDrillCmd() *cobra.Command {
	var scope string
	var projectDir string
	var composeFile string
	var envFile string
	var dbBackup string
	var filesBackup string
	var timeoutSeconds int
	var appPort int
	var wsPort int
	var skipHTTPProbe bool
	var keepArtifacts bool

	cmd := &cobra.Command{
		Use:   "restore-drill",
		Short: "Run the canonical restore-drill flow",
		Args:  noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			in := restoreDrillInput{
				scope:         scope,
				projectDir:    projectDir,
				composeFile:   composeFile,
				envFile:       envFile,
				dbBackup:      dbBackup,
				filesBackup:   filesBackup,
				timeout:       timeoutSeconds,
				appPort:       appPort,
				wsPort:        wsPort,
				skipHTTPProbe: skipHTTPProbe,
				keepArtifacts: keepArtifacts,
			}
			if err := validateRestoreDrillInput(cmd, &in); err != nil {
				return err
			}

			return runRestoreDrill(cmd, in)
		},
	}

	cmd.Flags().StringVar(&scope, "scope", "", "restore-drill contour")
	cmd.Flags().StringVar(&projectDir, "project-dir", ".", "project directory containing the compose file and env files")
	cmd.Flags().StringVar(&composeFile, "compose-file", "", "compose file path (defaults to project-dir/compose.yaml)")
	cmd.Flags().StringVar(&envFile, "env-file", "", "override env file path")
	cmd.Flags().StringVar(&dbBackup, "db-backup", "", "explicit database backup path for the restore drill")
	cmd.Flags().StringVar(&filesBackup, "files-backup", "", "explicit files backup path for the restore drill")
	cmd.Flags().IntVar(&timeoutSeconds, "timeout", 600, "shared readiness timeout in seconds")
	cmd.Flags().IntVar(&appPort, "app-port", 0, "override the temporary restore-drill application port")
	cmd.Flags().IntVar(&wsPort, "ws-port", 0, "override the temporary restore-drill websocket port")
	cmd.Flags().BoolVar(&skipHTTPProbe, "skip-http-probe", false, "skip the final HTTP probe in the restore drill")
	cmd.Flags().BoolVar(&keepArtifacts, "keep-artifacts", false, "preserve the temporary restore-drill contour after completion")

	return cmd
}

type restoreDrillInput struct {
	scope         string
	projectDir    string
	composeFile   string
	envFile       string
	dbBackup      string
	filesBackup   string
	timeout       int
	appPort       int
	wsPort        int
	skipHTTPProbe bool
	keepArtifacts bool
}

func validateRestoreDrillInput(cmd *cobra.Command, in *restoreDrillInput) error {
	in.scope = strings.TrimSpace(in.scope)
	in.projectDir = strings.TrimSpace(in.projectDir)
	in.composeFile = strings.TrimSpace(in.composeFile)
	in.envFile = strings.TrimSpace(in.envFile)
	in.dbBackup = strings.TrimSpace(in.dbBackup)
	in.filesBackup = strings.TrimSpace(in.filesBackup)

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

	if in.timeout <= 0 {
		return usageError(fmt.Errorf("--timeout must be a positive integer number of seconds"))
	}
	if cmd.Flags().Changed("app-port") {
		if in.appPort < 1 || in.appPort > 65535 {
			return usageError(fmt.Errorf("invalid --app-port for restore-drill: %d", in.appPort))
		}
	}
	if cmd.Flags().Changed("ws-port") {
		if in.wsPort < 1 || in.wsPort > 65535 {
			return usageError(fmt.Errorf("invalid --ws-port for restore-drill: %d", in.wsPort))
		}
	}
	if in.appPort != 0 && in.wsPort != 0 && in.appPort == in.wsPort {
		return usageError(fmt.Errorf("APP and WS ports for restore-drill must differ"))
	}

	return nil
}

func runRestoreDrill(cmd *cobra.Command, in restoreDrillInput) error {
	spec := CommandSpec{
		Name:       "restore-drill",
		ErrorCode:  "restore_drill_failed",
		ExitCode:   exitcode.InternalError,
		RenderText: renderRestoreDrillText,
	}
	exec := operationusecase.Begin(
		appForCommand(cmd).runtime,
		appForCommand(cmd).journalWriterFactory(appForCommand(cmd).options.JournalDir),
		spec.Name,
	)

	info, err := restoreusecase.ExecuteDrill(restoreusecase.DrillRequest{
		Scope:           in.scope,
		ProjectDir:      in.projectDir,
		ComposeFile:     in.composeFile,
		EnvFileOverride: in.envFile,
		EnvContourHint:  envFileContourHint(),
		DBBackup:        in.dbBackup,
		FilesBackup:     in.filesBackup,
		TimeoutSeconds:  in.timeout,
		DrillAppPort:    in.appPort,
		DrillWSPort:     in.wsPort,
		SkipHTTPProbe:   in.skipHTTPProbe,
		KeepArtifacts:   in.keepArtifacts,
		LogWriter:       cmd.ErrOrStderr(),
	})

	res := restoreDrillResult(info)
	res.Command = spec.Name
	if warnings := writeRestoreDrillReports(spec, info, res, err); len(warnings) != 0 {
		res.Warnings = append(res.Warnings, warnings...)
	}

	if err != nil {
		return finishJournaledCommandFailure(cmd, spec, exec, res, err)
	}

	return finishJournaledCommandSuccess(cmd, spec, exec, res)
}

func restoreDrillResult(info restoreusecase.DrillInfo) result.Result {
	completed, failed, notRun := info.Counts()
	message := "restore drill completed"
	if !info.Ready() {
		message = "restore drill failed"
	}

	items := make([]any, 0, len(info.Steps))
	for _, step := range info.Steps {
		items = append(items, result.RestoreDrillItem{
			SectionItem: result.SectionItem{
				Code:    step.Code,
				Status:  step.Status,
				Summary: step.Summary,
				Details: step.Details,
				Action:  step.Action,
			},
		})
	}

	return result.Result{
		Command:  "restore-drill",
		OK:       info.Ready(),
		Message:  message,
		Warnings: append([]string(nil), info.Warnings...),
		Details: result.RestoreDrillDetails{
			Scope:                  info.Scope,
			Ready:                  info.Ready(),
			RequestedSelectionMode: info.RequestedSelectionMode,
			SelectionMode:          info.SelectionMode,
			Steps:                  len(info.Steps),
			Completed:              completed,
			Failed:                 failed,
			NotRun:                 notRun,
			Warnings:               len(info.Warnings),
			TimeoutSeconds:         info.TimeoutSeconds,
			SkipHTTPProbe:          info.SkipHTTPProbe,
			KeepArtifacts:          info.KeepArtifacts,
			DrillAppPort:           info.DrillAppPort,
			DrillWSPort:            info.DrillWSPort,
			ServicesReady:          append([]string(nil), info.ServicesReady...),
		},
		Artifacts: result.RestoreDrillArtifacts{
			ProjectDir:           info.ProjectDir,
			ComposeFile:          info.ComposeFile,
			SourceEnvFile:        info.SourceEnvFile,
			SourceComposeProject: info.SourceComposeProject,
			SourceBackupRoot:     info.SourceBackupRoot,
			SelectedPrefix:       info.SelectedPrefix,
			SelectedStamp:        info.SelectedStamp,
			ManifestTXT:          info.ManifestTXTPath,
			ManifestJSON:         info.ManifestJSONPath,
			DBBackup:             info.DBBackupPath,
			FilesBackup:          info.FilesBackupPath,
			DrillEnvFile:         info.DrillEnvFile,
			DrillComposeProject:  info.DrillComposeProject,
			DrillBackupRoot:      info.DrillBackupRoot,
			DrillDBStorage:       info.DrillDBStorage,
			DrillESPOStorage:     info.DrillESPOStorage,
			SiteURL:              info.SiteURL,
			WSPublicURL:          info.WSPublicURL,
			ReportTXT:            info.ReportTXTPath,
			ReportJSON:           info.ReportJSONPath,
		},
		Items: items,
	}
}

func renderRestoreDrillText(w io.Writer, res result.Result) error {
	details, ok := res.Details.(result.RestoreDrillDetails)
	if !ok {
		return result.Render(w, res, false)
	}
	artifacts, ok := res.Artifacts.(result.RestoreDrillArtifacts)
	if !ok {
		return fmt.Errorf("unexpected restore-drill artifacts type %T", res.Artifacts)
	}

	if _, err := fmt.Fprintln(w, "EspoCRM restore drill"); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "Source contour: %s\n", details.Scope); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "Source env file: %s\n", artifacts.SourceEnvFile); err != nil {
		return err
	}
	if strings.TrimSpace(artifacts.DrillEnvFile) != "" {
		if _, err := fmt.Fprintf(w, "Drill env file: %s\n", artifacts.DrillEnvFile); err != nil {
			return err
		}
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
	if strings.TrimSpace(artifacts.SiteURL) != "" {
		if _, err := fmt.Fprintf(w, "Drill URL: %s\n", artifacts.SiteURL); err != nil {
			return err
		}
	}

	if _, err := fmt.Fprintln(w, "\nSummary:"); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "  Ready:           %t\n", details.Ready); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "  Steps:           %d\n", details.Steps); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "  Completed:       %d\n", details.Completed); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "  Failed:          %d\n", details.Failed); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "  Not run:         %d\n", details.NotRun); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "  Drill APP port:  %d\n", details.DrillAppPort); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "  Drill WS port:   %d\n", details.DrillWSPort); err != nil {
		return err
	}

	if len(details.ServicesReady) != 0 {
		if _, err := fmt.Fprintln(w, "\nReadiness:"); err != nil {
			return err
		}
		if _, err := fmt.Fprintf(w, "  Services ready: %s\n", strings.Join(details.ServicesReady, ", ")); err != nil {
			return err
		}
	}

	if strings.TrimSpace(artifacts.ReportTXT) != "" || strings.TrimSpace(artifacts.ReportJSON) != "" {
		if _, err := fmt.Fprintln(w, "\nReports:"); err != nil {
			return err
		}
		if strings.TrimSpace(artifacts.ReportTXT) != "" {
			if _, err := fmt.Fprintf(w, "  Text report: %s\n", artifacts.ReportTXT); err != nil {
				return err
			}
		}
		if strings.TrimSpace(artifacts.ReportJSON) != "" {
			if _, err := fmt.Fprintf(w, "  JSON report: %s\n", artifacts.ReportJSON); err != nil {
				return err
			}
		}
	}

	if len(res.Items) != 0 {
		if _, err := fmt.Fprintln(w, "\nSteps:"); err != nil {
			return err
		}
		for _, rawItem := range res.Items {
			item, ok := rawItem.(result.RestoreDrillItem)
			if !ok {
				continue
			}
			if _, err := fmt.Fprintf(w, "[%s] %s\n", strings.ToUpper(item.Status), item.Summary); err != nil {
				return err
			}
			if strings.TrimSpace(item.Details) != "" {
				if _, err := fmt.Fprintf(w, "  %s\n", item.Details); err != nil {
					return err
				}
			}
			if strings.TrimSpace(item.Action) != "" {
				if _, err := fmt.Fprintf(w, "  Action: %s\n", item.Action); err != nil {
					return err
				}
			}
		}
	}

	return nil
}

func writeRestoreDrillReports(spec CommandSpec, info restoreusecase.DrillInfo, res result.Result, err error) []string {
	if strings.TrimSpace(info.ReportTXTPath) == "" && strings.TrimSpace(info.ReportJSONPath) == "" {
		return nil
	}

	reportRes := restoreDrillReportResult(spec, res, err)
	warnings := []string{}

	if strings.TrimSpace(info.ReportTXTPath) != "" {
		if writeErr := writeRestoreDrillTextReport(spec, info.ReportTXTPath, reportRes); writeErr != nil {
			warnings = append(warnings, fmt.Sprintf("failed to write restore-drill text report %s: %v", info.ReportTXTPath, writeErr))
		}
	}
	if strings.TrimSpace(info.ReportJSONPath) != "" {
		if writeErr := writeRestoreDrillJSONReport(info.ReportJSONPath, reportRes); writeErr != nil {
			warnings = append(warnings, fmt.Sprintf("failed to write restore-drill JSON report %s: %v", info.ReportJSONPath, writeErr))
		}
	}

	return warnings
}

func restoreDrillReportResult(spec CommandSpec, res result.Result, err error) result.Result {
	if err == nil {
		return res
	}

	errorRes, _ := ErrorResult(spec.Name, err, spec.ExitCode, spec.ErrorCode)
	errorRes.Message = res.Message
	errorRes.Warnings = append(errorRes.Warnings, res.Warnings...)
	errorRes.Details = res.Details
	errorRes.Artifacts = res.Artifacts
	errorRes.Items = res.Items
	errorRes.DryRun = res.DryRun
	return errorRes
}

func writeRestoreDrillTextReport(spec CommandSpec, path string, res result.Result) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer func() {
		_ = f.Close()
	}()

	if spec.RenderText != nil {
		if err := spec.RenderText(f, res); err != nil {
			return err
		}
	} else if err := result.Render(f, res, false); err != nil {
		return err
	}

	return renderWarnings(f, res.Warnings)
}

func writeRestoreDrillJSONReport(path string, res result.Result) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer func() {
		_ = f.Close()
	}()

	return result.Render(f, res, true)
}
