package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/lazuale/espocrm-ops/internal/contract/exitcode"
	"github.com/lazuale/espocrm-ops/internal/contract/result"
	journalusecase "github.com/lazuale/espocrm-ops/internal/usecase/journal"
	"github.com/spf13/cobra"
)

func newExportOperationCmd() *cobra.Command {
	var id string
	var outputPath string

	cmd := &cobra.Command{
		Use:   "export-operation",
		Short: "Export one operation as a structured incident bundle",
		Args:  noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			app := appForCommand(cmd)
			if err := requireNonBlankFlag("--id", id); err != nil {
				return err
			}

			resolvedOutputPath, err := resolveOperationBundleOutputPath("--output", outputPath)
			if err != nil {
				return err
			}

			return RunResultCommand(cmd, CommandSpec{
				Name:       "export-operation",
				ErrorCode:  "operation_export_failed",
				ExitCode:   exitcode.InternalError,
				RenderText: renderExportOperationText,
			}, func() (result.Result, error) {
				exported, err := journalusecase.Export(journalusecase.ExportInput{
					JournalDir: app.options.JournalDir,
					ID:         id,
				})
				if err != nil {
					return result.Result{}, err
				}

				raw, err := json.MarshalIndent(exported.Bundle, "", "  ")
				if err != nil {
					return result.Result{}, err
				}
				raw = append(raw, '\n')
				if err := os.WriteFile(resolvedOutputPath, raw, 0o644); err != nil {
					return result.Result{}, err
				}

				return result.Result{
					OK:       true,
					Message:  fmt.Sprintf("operation bundle exported to %s", resolvedOutputPath),
					Warnings: journalusecase.WarningsFromReadStats(exported.Bundle.JournalRead),
					Details:  journalusecase.OperationExportDetailsFromBundle(id, exported.Bundle),
					Artifacts: result.OperationExportArtifacts{
						BundlePath: resolvedOutputPath,
					},
					Items: []any{exported.Bundle.Summary},
				}, nil
			})
		},
	}

	cmd.Flags().StringVar(&id, "id", "", "operation id")
	cmd.Flags().StringVar(&outputPath, "output", "", "path to write the exported operation bundle JSON")

	return cmd
}

func resolveOperationBundleOutputPath(flag, raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", requiredFlagError(flag)
	}

	clean := filepath.Clean(trimmed)
	if clean == "." {
		return "", usageError(fmt.Errorf("%s must not be the current directory", flag))
	}
	if clean == string(filepath.Separator) {
		return "", usageError(fmt.Errorf("%s must not be the filesystem root", flag))
	}

	resolved, err := filepath.Abs(clean)
	if err != nil {
		return "", usageError(fmt.Errorf("resolve %s: %w", flag, err))
	}

	return filepath.Clean(resolved), nil
}

func renderExportOperationText(w io.Writer, res result.Result) error {
	details, ok := res.Details.(result.OperationExportDetails)
	if !ok {
		return fmt.Errorf("unexpected export-operation details type %T", res.Details)
	}
	artifacts, ok := res.Artifacts.(result.OperationExportArtifacts)
	if !ok {
		return fmt.Errorf("unexpected export-operation artifacts type %T", res.Artifacts)
	}
	if len(res.Items) != 1 {
		return fmt.Errorf("expected one export-operation item, got %d", len(res.Items))
	}

	summary, ok := res.Items[0].(journalusecase.OperationSummary)
	if !ok {
		return fmt.Errorf("unexpected export-operation item type %T", res.Items[0])
	}

	if _, err := fmt.Fprintf(w, "operation bundle exported: %s\n", artifacts.BundlePath); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "bundle: %s v%d\n", details.BundleKind, details.BundleVersion); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "exported at: %s\n", details.ExportedAt); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "operation: %s\n", formatHistoryLine(summary)); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "included: %s\n", strings.Join(details.IncludedSections, ", ")); err != nil {
		return err
	}

	omitted := "none"
	if len(details.OmittedSections) != 0 {
		omitted = strings.Join(details.OmittedSections, ", ")
	}
	_, err := fmt.Fprintf(w, "omitted: %s\n", omitted)
	return err
}
