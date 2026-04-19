package cli

import (
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/lazuale/espocrm-ops/internal/contract/apperr"
	"github.com/lazuale/espocrm-ops/internal/contract/exitcode"
	"github.com/lazuale/espocrm-ops/internal/contract/result"
	operationgateusecase "github.com/lazuale/espocrm-ops/internal/usecase/operationgate"
	"github.com/spf13/cobra"
)

func newOperationGateCmd() *cobra.Command {
	var action string
	var scope string
	var fromScope string
	var toScope string
	var projectDir string
	var composeFile string
	var envFile string
	var noVerifyChecksum bool
	var maxAgeHours int

	var skipDoctor bool
	var skipBackup bool
	var skipPull bool
	var skipHTTPProbe bool

	var dbBackup string
	var filesBackup string
	var manifestPath string
	var skipDB bool
	var skipFiles bool
	var noSnapshot bool
	var noStop bool
	var noStart bool
	var appPort int
	var wsPort int
	var keepArtifacts bool

	cmd := &cobra.Command{
		Use:   "operation-gate",
		Short: "Show the canonical Go-owned readiness decision for a risky operator action",
		Args:  noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			in := operationGateInput{
				action:         action,
				scope:          scope,
				fromScope:      fromScope,
				toScope:        toScope,
				projectDir:     projectDir,
				composeFile:    composeFile,
				envFile:        envFile,
				verifyChecksum: !noVerifyChecksum,
				maxAgeHours:    maxAgeHours,
				skipDoctor:     skipDoctor,
				skipBackup:     skipBackup,
				skipPull:       skipPull,
				skipHTTPProbe:  skipHTTPProbe,
				dbBackup:       dbBackup,
				filesBackup:    filesBackup,
				manifestPath:   manifestPath,
				skipDB:         skipDB,
				skipFiles:      skipFiles,
				noSnapshot:     noSnapshot,
				noStop:         noStop,
				noStart:        noStart,
				appPort:        appPort,
				wsPort:         wsPort,
				keepArtifacts:  keepArtifacts,
			}
			if err := validateOperationGateInput(cmd, &in); err != nil {
				return err
			}

			info, err := operationgateusecase.Summarize(operationgateusecase.Request{
				Action:          in.action,
				Scope:           in.scope,
				SourceScope:     in.fromScope,
				TargetScope:     in.toScope,
				ProjectDir:      in.projectDir,
				ComposeFile:     in.composeFile,
				EnvFileOverride: in.envFile,
				JournalDir:      appForCommand(cmd).options.JournalDir,
				VerifyChecksum:  in.verifyChecksum,
				MaxAgeHours:     in.maxAgeHours,
				Now:             appForCommand(cmd).runtime.Now(),
				SkipDoctor:      in.skipDoctor,
				SkipBackup:      in.skipBackup,
				SkipPull:        in.skipPull,
				SkipHTTPProbe:   in.skipHTTPProbe,
				DBBackup:        in.dbBackup,
				FilesBackup:     in.filesBackup,
				ManifestPath:    in.manifestPath,
				SkipDB:          in.skipDB,
				SkipFiles:       in.skipFiles,
				NoSnapshot:      in.noSnapshot,
				NoStop:          in.noStop,
				NoStart:         in.noStart,
				DrillAppPort:    in.appPort,
				DrillWSPort:     in.wsPort,
				KeepArtifacts:   in.keepArtifacts,
			})

			res := operationGateResult(info)
			if err == nil && info.Decision != operationgateusecase.DecisionBlocked {
				return renderCommandResult(cmd, CommandSpec{
					Name:       "operation-gate",
					RenderText: renderOperationGateText,
				}, res)
			}

			errCode := operationGateCodeError(err)
			if info.Decision == operationgateusecase.DecisionBlocked && len(info.FailedSections) == 0 {
				errCode = CodeError{
					Code:    exitcode.ValidationError,
					Err:     apperr.Wrap(apperr.KindValidation, "operation_gate_blocked", errors.New("operation gate reported blocking conditions")),
					ErrCode: "operation_gate_blocked",
				}
			}

			if appForCommand(cmd).JSONEnabled() {
				return ResultCodeError{
					CodeError: errCode,
					Result:    res,
				}
			}

			if renderErr := renderOperationGateText(cmd.OutOrStdout(), res); renderErr != nil {
				return renderErr
			}
			if renderErr := renderWarnings(cmd.OutOrStdout(), res.Warnings); renderErr != nil {
				return renderErr
			}
			return silentCodeError{CodeError: errCode}
		},
	}

	cmd.Flags().StringVar(&action, "action", "", "action to evaluate: update, rollback, restore, restore-drill, or migrate-backup")
	cmd.Flags().StringVar(&scope, "scope", "", "single-contour action scope")
	cmd.Flags().StringVar(&fromScope, "from", "", "source contour for migrate-backup")
	cmd.Flags().StringVar(&toScope, "to", "", "target contour for migrate-backup")
	cmd.Flags().StringVar(&projectDir, "project-dir", ".", "project directory containing the compose file and env files")
	cmd.Flags().StringVar(&composeFile, "compose-file", "", "compose file path (defaults to project-dir/compose.yaml)")
	cmd.Flags().StringVar(&envFile, "env-file", "", "override env file path for single-contour actions")
	cmd.Flags().BoolVar(&noVerifyChecksum, "no-verify-checksum", false, "skip checksum verification while evaluating backup posture")
	cmd.Flags().IntVar(&maxAgeHours, "max-age-hours", 48, "maximum allowed age in hours for the latest restore-ready backup set")

	cmd.Flags().BoolVar(&skipDoctor, "skip-doctor", false, "evaluate update with --skip-doctor")
	cmd.Flags().BoolVar(&skipBackup, "skip-backup", false, "evaluate update with --skip-backup")
	cmd.Flags().BoolVar(&skipPull, "skip-pull", false, "evaluate update with --skip-pull")
	cmd.Flags().BoolVar(&skipHTTPProbe, "skip-http-probe", false, "evaluate actions with --skip-http-probe where supported")

	cmd.Flags().StringVar(&dbBackup, "db-backup", "", "explicit database backup path for rollback, restore, restore-drill, or migrate-backup")
	cmd.Flags().StringVar(&filesBackup, "files-backup", "", "explicit files backup path for rollback, restore, restore-drill, or migrate-backup")
	cmd.Flags().StringVar(&manifestPath, "manifest", "", "manifest path for restore")
	cmd.Flags().BoolVar(&skipDB, "skip-db", false, "evaluate restore or migrate-backup with --skip-db")
	cmd.Flags().BoolVar(&skipFiles, "skip-files", false, "evaluate restore or migrate-backup with --skip-files")
	cmd.Flags().BoolVar(&noSnapshot, "no-snapshot", false, "evaluate rollback or restore with --no-snapshot")
	cmd.Flags().BoolVar(&noStop, "no-stop", false, "evaluate restore with --no-stop")
	cmd.Flags().BoolVar(&noStart, "no-start", false, "evaluate rollback, restore, or migrate-backup with --no-start")
	cmd.Flags().IntVar(&appPort, "app-port", 0, "evaluate restore-drill with an explicit app port")
	cmd.Flags().IntVar(&wsPort, "ws-port", 0, "evaluate restore-drill with an explicit websocket port")
	cmd.Flags().BoolVar(&keepArtifacts, "keep-artifacts", false, "evaluate restore-drill with --keep-artifacts")

	return cmd
}

type operationGateInput struct {
	action         string
	scope          string
	fromScope      string
	toScope        string
	projectDir     string
	composeFile    string
	envFile        string
	verifyChecksum bool
	maxAgeHours    int

	skipDoctor    bool
	skipBackup    bool
	skipPull      bool
	skipHTTPProbe bool

	dbBackup      string
	filesBackup   string
	manifestPath  string
	skipDB        bool
	skipFiles     bool
	noSnapshot    bool
	noStop        bool
	noStart       bool
	appPort       int
	wsPort        int
	keepArtifacts bool
}

func validateOperationGateInput(cmd *cobra.Command, in *operationGateInput) error {
	in.action = strings.TrimSpace(in.action)
	in.scope = strings.TrimSpace(in.scope)
	in.fromScope = strings.TrimSpace(in.fromScope)
	in.toScope = strings.TrimSpace(in.toScope)
	in.projectDir = strings.TrimSpace(in.projectDir)
	in.composeFile = strings.TrimSpace(in.composeFile)
	in.envFile = strings.TrimSpace(in.envFile)
	in.dbBackup = strings.TrimSpace(in.dbBackup)
	in.filesBackup = strings.TrimSpace(in.filesBackup)
	in.manifestPath = strings.TrimSpace(in.manifestPath)

	switch in.action {
	case "update", "rollback", "restore", "restore-drill", "migrate-backup":
	default:
		return usageError(fmt.Errorf("--action must be update, rollback, restore, restore-drill, or migrate-backup"))
	}
	if err := requireNonBlankFlag("--project-dir", in.projectDir); err != nil {
		return err
	}
	if in.maxAgeHours < 0 {
		return usageError(fmt.Errorf("--max-age-hours must be non-negative"))
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
	if in.manifestPath != "" {
		manifestAbs, err := filepath.Abs(filepath.Clean(in.manifestPath))
		if err != nil {
			return usageError(fmt.Errorf("resolve --manifest: %w", err))
		}
		in.manifestPath = manifestAbs
	}

	switch in.action {
	case "update":
		if err := requireScopeFlag(in.scope); err != nil {
			return err
		}
		if err := forbidActionFlags(cmd, "update", "from", "to", "db-backup", "files-backup", "manifest", "skip-db", "skip-files", "no-snapshot", "no-stop", "no-start", "app-port", "ws-port", "keep-artifacts"); err != nil {
			return err
		}
	case "rollback":
		if err := requireScopeFlag(in.scope); err != nil {
			return err
		}
		if err := forbidActionFlags(cmd, "rollback", "from", "to", "manifest", "skip-doctor", "skip-backup", "skip-pull", "skip-db", "skip-files", "no-stop", "app-port", "ws-port", "keep-artifacts"); err != nil {
			return err
		}
		if (in.dbBackup == "") != (in.filesBackup == "") {
			return usageError(fmt.Errorf("pass both --db-backup and --files-backup together"))
		}
	case "restore":
		if err := requireScopeFlag(in.scope); err != nil {
			return err
		}
		if err := forbidActionFlags(cmd, "restore", "from", "to", "skip-doctor", "skip-backup", "skip-pull", "skip-http-probe", "app-port", "ws-port", "keep-artifacts"); err != nil {
			return err
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
	case "restore-drill":
		if err := requireScopeFlag(in.scope); err != nil {
			return err
		}
		if err := forbidActionFlags(cmd, "restore-drill", "from", "to", "manifest", "skip-doctor", "skip-backup", "skip-pull", "skip-db", "skip-files", "no-snapshot", "no-stop", "no-start"); err != nil {
			return err
		}
		if cmd.Flags().Changed("app-port") && (in.appPort < 1 || in.appPort > 65535) {
			return usageError(fmt.Errorf("invalid --app-port for restore-drill: %d", in.appPort))
		}
		if cmd.Flags().Changed("ws-port") && (in.wsPort < 1 || in.wsPort > 65535) {
			return usageError(fmt.Errorf("invalid --ws-port for restore-drill: %d", in.wsPort))
		}
		if in.appPort != 0 && in.wsPort != 0 && in.appPort == in.wsPort {
			return usageError(fmt.Errorf("APP and WS ports for restore-drill must differ"))
		}
	case "migrate-backup":
		if err := requireScopeValue("--from", in.fromScope); err != nil {
			return err
		}
		if err := requireScopeValue("--to", in.toScope); err != nil {
			return err
		}
		if in.fromScope == in.toScope {
			return usageError(fmt.Errorf("source and target contours must differ"))
		}
		if err := forbidActionFlags(cmd, "migrate-backup", "scope", "env-file", "manifest", "skip-doctor", "skip-backup", "skip-pull", "skip-http-probe", "no-snapshot", "no-stop", "app-port", "ws-port", "keep-artifacts"); err != nil {
			return err
		}
		if in.skipDB && in.skipFiles {
			return usageError(fmt.Errorf("nothing to migrate: --skip-db and --skip-files cannot both be set"))
		}
		if in.skipDB && in.dbBackup != "" {
			return usageError(fmt.Errorf("--db-backup cannot be used with --skip-db"))
		}
		if in.skipFiles && in.filesBackup != "" {
			return usageError(fmt.Errorf("--files-backup cannot be used with --skip-files"))
		}
	}

	return nil
}

func operationGateResult(info operationgateusecase.Info) result.Result {
	return result.Result{
		Command:  "operation-gate",
		OK:       info.Decision == operationgateusecase.DecisionAllowed || info.Decision == operationgateusecase.DecisionRisky,
		Message:  "operation gate " + info.Action + " " + info.Decision,
		Warnings: append([]string(nil), info.Warnings...),
		Details: result.OperationGateDetails{
			Action:              info.Action,
			Scope:               info.Scope,
			SourceScope:         info.SourceScope,
			TargetScope:         info.TargetScope,
			GeneratedAt:         info.GeneratedAt,
			Decision:            info.Decision,
			NextAction:          info.NextAction,
			HealthVerdict:       info.HealthVerdict,
			SourceHealthVerdict: info.SourceHealthVerdict,
			TargetHealthVerdict: info.TargetHealthVerdict,
			WarningAlerts:       operationGateAlertCount(info.Alerts, operationgateusecase.AlertSeverityWarning),
			BlockingAlerts:      operationGateAlertCount(info.Alerts, operationgateusecase.AlertSeverityBlocking),
			FailureAlerts:       operationGateAlertCount(info.Alerts, operationgateusecase.AlertSeverityFailure),
			Reasons:             append([]string(nil), info.Reasons...),
			SectionResults:      operationGateSections(info.Sections),
			SectionSummary:      result.NewSectionSummary(len(info.Sections), len(info.Warnings), info.IncludedSections, info.OmittedSections, info.FailedSections),
		},
		Artifacts: result.OperationGateArtifacts{
			ProjectDir:       info.ProjectDir,
			ComposeFile:      info.ComposeFile,
			EnvFile:          info.EnvFile,
			BackupRoot:       info.BackupRoot,
			SourceEnvFile:    info.SourceEnvFile,
			SourceBackupRoot: info.SourceBackupRoot,
			TargetEnvFile:    info.TargetEnvFile,
			TargetBackupRoot: info.TargetBackupRoot,
		},
		Items: operationGateAlertItems(info.Alerts),
	}
}

func renderOperationGateText(w io.Writer, res result.Result) error {
	details, ok := res.Details.(result.OperationGateDetails)
	if !ok {
		return result.Render(w, res, false)
	}
	artifacts, ok := res.Artifacts.(result.OperationGateArtifacts)
	if !ok {
		return fmt.Errorf("unexpected operation-gate artifacts type %T", res.Artifacts)
	}

	if _, err := fmt.Fprintln(w, "EspoCRM operation gate"); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "Action: %s\n", details.Action); err != nil {
		return err
	}
	if strings.TrimSpace(details.Scope) != "" {
		if _, err := fmt.Fprintf(w, "Scope: %s\n", details.Scope); err != nil {
			return err
		}
	}
	if strings.TrimSpace(details.SourceScope) != "" || strings.TrimSpace(details.TargetScope) != "" {
		if _, err := fmt.Fprintf(w, "Scopes: %s -> %s\n", valueOrNA(details.SourceScope), valueOrNA(details.TargetScope)); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintf(w, "Generated at: %s\n", details.GeneratedAt); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "Project dir: %s\n", artifacts.ProjectDir); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "Compose file: %s\n", artifacts.ComposeFile); err != nil {
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
	if strings.TrimSpace(artifacts.SourceEnvFile) != "" {
		if _, err := fmt.Fprintf(w, "Source env file: %s\n", artifacts.SourceEnvFile); err != nil {
			return err
		}
	}
	if strings.TrimSpace(artifacts.SourceBackupRoot) != "" {
		if _, err := fmt.Fprintf(w, "Source backup root: %s\n", artifacts.SourceBackupRoot); err != nil {
			return err
		}
	}
	if strings.TrimSpace(artifacts.TargetEnvFile) != "" {
		if _, err := fmt.Fprintf(w, "Target env file: %s\n", artifacts.TargetEnvFile); err != nil {
			return err
		}
	}
	if strings.TrimSpace(artifacts.TargetBackupRoot) != "" {
		if _, err := fmt.Fprintf(w, "Target backup root: %s\n", artifacts.TargetBackupRoot); err != nil {
			return err
		}
	}

	extraLines := []operatorSummaryLine{
		{Label: "Decision", Value: details.Decision},
		{Label: "Health verdict", Value: valueOrNA(details.HealthVerdict)},
		{Label: "Source health", Value: valueOrNA(details.SourceHealthVerdict)},
		{Label: "Target health", Value: valueOrNA(details.TargetHealthVerdict)},
		{Label: "Warning alerts", Value: details.WarningAlerts},
		{Label: "Blocking alerts", Value: details.BlockingAlerts},
		{Label: "Failure alerts", Value: details.FailureAlerts},
		{Label: "Reasons", Value: sectionListText(details.Reasons)},
		{Label: "Next action", Value: valueOrNA(details.NextAction)},
	}
	if err := renderOperatorSummaryBlock(w, details.SectionSummary, operatorSummaryRenderOptions{
		IncludeWarnings: true,
		ExtraLines:      extraLines,
	}); err != nil {
		return err
	}

	if err := renderOperationGateSections(w, details.SectionResults); err != nil {
		return err
	}
	if err := renderOperationGateAlerts(w, res.Items); err != nil {
		return err
	}

	return nil
}

func renderOperationGateSections(w io.Writer, sections []result.OperationGateSection) error {
	if len(sections) == 0 {
		return nil
	}

	if _, err := fmt.Fprintln(w, "\nSections:"); err != nil {
		return err
	}
	for _, section := range sections {
		if _, err := fmt.Fprintf(w, "[%s][%s] %s\n", upperStatusText(section.Decision), upperSpacedStatusText(section.Status), section.Summary); err != nil {
			return err
		}
		if strings.TrimSpace(section.SourceCommand) != "" {
			if _, err := fmt.Fprintf(w, "  Source: %s\n", section.SourceCommand); err != nil {
				return err
			}
		}
		if strings.TrimSpace(section.Details) != "" {
			if _, err := fmt.Fprintf(w, "  %s\n", section.Details); err != nil {
				return err
			}
		}
		if strings.TrimSpace(section.NextAction) != "" {
			if _, err := fmt.Fprintf(w, "  Action: %s\n", section.NextAction); err != nil {
				return err
			}
		}
	}

	return nil
}

func renderOperationGateAlerts(w io.Writer, items []any) error {
	if len(items) == 0 {
		return nil
	}

	if _, err := fmt.Fprintln(w, "\nAlerts:"); err != nil {
		return err
	}
	for _, raw := range items {
		alert, ok := raw.(result.OperationGateItem)
		if !ok {
			return fmt.Errorf("unexpected operation-gate alert type %T", raw)
		}
		if _, err := fmt.Fprintf(w, "[%s][%s] %s\n", upperStatusText(alert.Severity), strings.ToUpper(strings.ReplaceAll(alert.Section, "_", " ")), alert.Summary); err != nil {
			return err
		}
		if strings.TrimSpace(alert.Cause) != "" {
			if _, err := fmt.Fprintf(w, "  Cause: %s\n", alert.Cause); err != nil {
				return err
			}
		}
		if strings.TrimSpace(alert.SourceCommand) != "" {
			if _, err := fmt.Fprintf(w, "  Source: %s\n", alert.SourceCommand); err != nil {
				return err
			}
		}
		if strings.TrimSpace(alert.NextAction) != "" {
			if _, err := fmt.Fprintf(w, "  Action: %s\n", alert.NextAction); err != nil {
				return err
			}
		}
	}

	return nil
}

func operationGateSections(sections []operationgateusecase.Section) []result.OperationGateSection {
	out := make([]result.OperationGateSection, 0, len(sections))
	for _, section := range sections {
		out = append(out, result.OperationGateSection{
			Code:          section.Code,
			Status:        section.Status,
			Decision:      section.Decision,
			SourceCommand: section.SourceCommand,
			Summary:       section.Summary,
			Details:       section.Details,
			CauseCode:     section.CauseCode,
			NextAction:    section.NextAction,
		})
	}
	return out
}

func operationGateAlertItems(alerts []operationgateusecase.Alert) []any {
	out := make([]any, 0, len(alerts))
	for _, alert := range alerts {
		out = append(out, result.OperationGateItem{
			Code:          alert.Code,
			Severity:      alert.Severity,
			Section:       alert.Section,
			SourceCommand: alert.SourceCommand,
			Summary:       alert.Summary,
			Cause:         alert.Cause,
			NextAction:    alert.NextAction,
		})
	}
	return out
}

func operationGateAlertCount(alerts []operationgateusecase.Alert, severity string) int {
	count := 0
	for _, alert := range alerts {
		if alert.Severity == severity {
			count++
		}
	}
	return count
}

func operationGateFailureCode(err error) string {
	code, ok := apperr.CodeOf(err)
	if !ok {
		return "operation_gate_failed"
	}
	return code
}

func operationGateCodeError(err error) CodeError {
	if err == nil {
		err = apperr.Wrap(apperr.KindValidation, "operation_gate_failed", errors.New("operation gate failed"))
	}
	return CodeError{
		Code:    codeForError(err, exitcode.ValidationError),
		Err:     err,
		ErrCode: operationGateFailureCode(err),
	}
}

func requireScopeFlag(value string) error {
	return requireScopeValue("--scope", value)
}

func requireScopeValue(flag, value string) error {
	switch strings.TrimSpace(value) {
	case "dev", "prod":
		return nil
	default:
		return usageError(fmt.Errorf("%s must be dev or prod", flag))
	}
}

func forbidActionFlags(cmd *cobra.Command, action string, names ...string) error {
	for _, name := range names {
		if cmd.Flags().Changed(name) {
			return usageError(fmt.Errorf("--%s is not supported for --action %s", name, action))
		}
	}
	return nil
}
