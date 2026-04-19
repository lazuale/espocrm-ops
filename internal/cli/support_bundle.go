package cli

import (
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/lazuale/espocrm-ops/internal/contract/exitcode"
	"github.com/lazuale/espocrm-ops/internal/contract/result"
	operationusecase "github.com/lazuale/espocrm-ops/internal/usecase/operation"
	supportbundleusecase "github.com/lazuale/espocrm-ops/internal/usecase/supportbundle"
	"github.com/spf13/cobra"
)

func newSupportBundleCmd() *cobra.Command {
	var scope string
	var projectDir string
	var composeFile string
	var envFile string
	var outputPath string
	var tailLines int

	cmd := &cobra.Command{
		Use:   "support-bundle",
		Short: "Collect a canonical support bundle for one contour",
		Args:  noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			in := supportBundleInput{
				scope:       scope,
				projectDir:  projectDir,
				composeFile: composeFile,
				envFile:     envFile,
				outputPath:  outputPath,
				tailLines:   tailLines,
			}
			if err := validateSupportBundleInput(cmd, &in); err != nil {
				return err
			}

			return runSupportBundle(cmd, in)
		},
	}

	cmd.Flags().StringVar(&scope, "scope", "", "support-bundle contour")
	cmd.Flags().StringVar(&projectDir, "project-dir", ".", "project directory containing the compose file and env files")
	cmd.Flags().StringVar(&composeFile, "compose-file", "", "compose file path (defaults to project-dir/compose.yaml)")
	cmd.Flags().StringVar(&envFile, "env-file", "", "override env file path")
	cmd.Flags().StringVar(&outputPath, "output", "", "path for the support bundle archive (defaults to backup-root/support)")
	cmd.Flags().IntVar(&tailLines, "tail", 300, "number of log lines to collect from docker compose logs")

	return cmd
}

type supportBundleInput struct {
	scope       string
	projectDir  string
	composeFile string
	envFile     string
	outputPath  string
	tailLines   int
}

func validateSupportBundleInput(cmd *cobra.Command, in *supportBundleInput) error {
	in.scope = strings.TrimSpace(in.scope)
	in.projectDir = strings.TrimSpace(in.projectDir)
	in.composeFile = strings.TrimSpace(in.composeFile)
	in.envFile = strings.TrimSpace(in.envFile)
	in.outputPath = strings.TrimSpace(in.outputPath)

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

	if err := normalizeOptionalStringFlag(cmd, "output", &in.outputPath); err != nil {
		return err
	}
	if in.outputPath != "" {
		if filepath.Clean(in.outputPath) == string(filepath.Separator) {
			return usageError(fmt.Errorf("--output must not be the filesystem root"))
		}
		outputAbs, err := filepath.Abs(filepath.Clean(in.outputPath))
		if err != nil {
			return usageError(fmt.Errorf("resolve --output: %w", err))
		}
		in.outputPath = filepath.Clean(outputAbs)
	}

	if in.tailLines <= 0 {
		return usageError(fmt.Errorf("--tail must be a positive integer"))
	}

	return nil
}

func runSupportBundle(cmd *cobra.Command, in supportBundleInput) error {
	spec := CommandSpec{
		Name:       "support-bundle",
		ErrorCode:  "support_bundle_failed",
		ExitCode:   exitcode.InternalError,
		RenderText: renderSupportBundleText,
	}
	exec := operationusecase.Begin(
		appForCommand(cmd).runtime,
		appForCommand(cmd).journalWriterFactory(appForCommand(cmd).options.JournalDir),
		spec.Name,
	)

	info, err := supportbundleusecase.Generate(supportbundleusecase.Request{
		Scope:           in.scope,
		ProjectDir:      in.projectDir,
		ComposeFile:     in.composeFile,
		EnvFileOverride: in.envFile,
		EnvContourHint:  envFileContourHint(),
		JournalDir:      appForCommand(cmd).options.JournalDir,
		OutputPath:      in.outputPath,
		TailLines:       in.tailLines,
		Now:             appForCommand(cmd).runtime.Now(),
		LogWriter:       cmd.ErrOrStderr(),
	})

	res := supportBundleResult(info)
	if err != nil {
		return finishJournaledCommandFailure(cmd, spec, exec, res, err)
	}

	return finishJournaledCommandSuccess(cmd, spec, exec, res)
}

func supportBundleResult(info supportbundleusecase.Info) result.Result {
	ok := true
	for _, section := range info.Sections {
		if section.Status == "failed" {
			ok = false
			break
		}
	}

	message := "support bundle created"
	if !ok {
		message = "support bundle failed"
	}

	items := make([]any, 0, len(info.Sections))
	for _, section := range info.Sections {
		items = append(items, result.SupportBundleItem{
			SectionItem: result.SectionItem{
				Code:    section.Code,
				Status:  section.Status,
				Summary: section.Summary,
				Details: section.Details,
				Action:  section.Action,
			},
			Files: append([]string(nil), section.Files...),
		})
	}

	return result.Result{
		Command:  "support-bundle",
		OK:       ok,
		Message:  message,
		Warnings: append([]string(nil), info.Warnings...),
		Details: result.SupportBundleDetails{
			Scope:                  info.Scope,
			BundleKind:             info.BundleKind,
			BundleVersion:          info.BundleVersion,
			GeneratedAt:            info.GeneratedAt,
			TailLines:              info.TailLines,
			RetentionDays:          info.RetentionDays,
			IncludedOmittedSummary: result.NewIncludedOmittedSummary(len(info.IncludedSections)+len(info.OmittedSections), len(info.Warnings), info.IncludedSections, info.OmittedSections),
		},
		Artifacts: result.SupportBundleArtifacts{
			ProjectDir:  info.ProjectDir,
			ComposeFile: info.ComposeFile,
			EnvFile:     info.EnvFile,
			BackupRoot:  info.BackupRoot,
			BundlePath:  info.OutputPath,
		},
		Items: items,
	}
}

func renderSupportBundleText(w io.Writer, res result.Result) error {
	details, ok := res.Details.(result.SupportBundleDetails)
	if !ok {
		return result.Render(w, res, false)
	}
	artifacts, ok := res.Artifacts.(result.SupportBundleArtifacts)
	if !ok {
		return fmt.Errorf("unexpected support-bundle artifacts type %T", res.Artifacts)
	}

	if _, err := fmt.Fprintln(w, "EspoCRM support bundle"); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "Scope: %s\n", details.Scope); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "Env file: %s\n", artifacts.EnvFile); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "Output path: %s\n", artifacts.BundlePath); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "Bundle: %s v%d\n", details.BundleKind, details.BundleVersion); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "Included: %s\n", strings.Join(details.IncludedSections, ", ")); err != nil {
		return err
	}

	omitted := "none"
	if len(details.OmittedSections) != 0 {
		omitted = strings.Join(details.OmittedSections, ", ")
	}
	if _, err := fmt.Fprintf(w, "Omitted: %s\n", omitted); err != nil {
		return err
	}

	if len(res.Items) == 0 {
		return nil
	}

	if _, err := fmt.Fprintln(w, "\nSections:"); err != nil {
		return err
	}
	for _, raw := range res.Items {
		item, ok := raw.(result.SupportBundleItem)
		if !ok {
			return fmt.Errorf("unexpected support-bundle item type %T", raw)
		}
		if _, err := fmt.Fprintf(w, "[%s] %s\n", strings.ToUpper(item.Status), item.Summary); err != nil {
			return err
		}
		if strings.TrimSpace(item.Details) != "" {
			if _, err := fmt.Fprintf(w, "  details: %s\n", item.Details); err != nil {
				return err
			}
		}
		if len(item.Files) != 0 {
			if _, err := fmt.Fprintf(w, "  files: %s\n", strings.Join(item.Files, ", ")); err != nil {
				return err
			}
		}
		if strings.TrimSpace(item.Action) != "" {
			if _, err := fmt.Fprintf(w, "  action: %s\n", item.Action); err != nil {
				return err
			}
		}
	}

	return nil
}
