package cli

import (
	"fmt"
	"io"
	"net/url"
	"strings"

	"github.com/lazuale/espocrm-ops/internal/contract/exitcode"
	"github.com/lazuale/espocrm-ops/internal/contract/result"
	updateusecase "github.com/lazuale/espocrm-ops/internal/usecase/update"
	"github.com/spf13/cobra"
)

func newUpdateRuntimeCmd() *cobra.Command {
	var projectDir string
	var composeFile string
	var envFile string
	var siteURL string
	var timeoutSeconds int
	var skipPull bool
	var skipHTTPProbe bool

	cmd := &cobra.Command{
		Use:    "update-runtime",
		Short:  "Apply docker-compose runtime update steps",
		Args:   noArgs,
		Hidden: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			in := updateRuntimeInput{
				projectDir:     projectDir,
				composeFile:    composeFile,
				envFile:        envFile,
				siteURL:        siteURL,
				timeoutSeconds: timeoutSeconds,
				skipPull:       skipPull,
				skipHTTPProbe:  skipHTTPProbe,
			}
			if err := validateUpdateRuntimeInput(cmd, &in); err != nil {
				return err
			}

			return RunResultCommand(cmd, CommandSpec{
				Name:       "update-runtime",
				ErrorCode:  "update_runtime_failed",
				ExitCode:   exitcode.InternalError,
				RenderText: renderUpdateRuntimeText,
			}, func() (result.Result, error) {
				res := result.Result{
					OK: true,
					Artifacts: result.UpdateRuntimeArtifacts{
						ProjectDir:  in.projectDir,
						ComposeFile: in.composeFile,
						EnvFile:     in.envFile,
						SiteURL:     in.siteURL,
					},
					Details: result.UpdateRuntimeDetails{
						TimeoutSeconds: in.timeoutSeconds,
						SkipPull:       in.skipPull,
						SkipHTTPProbe:  in.skipHTTPProbe,
					},
				}

				info, err := updateusecase.ApplyRuntime(updateusecase.RuntimeApplyRequest{
					ProjectDir:     in.projectDir,
					ComposeFile:    in.composeFile,
					EnvFile:        in.envFile,
					SiteURL:        in.siteURL,
					TimeoutSeconds: in.timeoutSeconds,
					SkipPull:       in.skipPull,
					SkipHTTPProbe:  in.skipHTTPProbe,
				})
				if err != nil {
					return res, err
				}

				res.Message = "update runtime completed"
				res.Details = result.UpdateRuntimeDetails{
					TimeoutSeconds: info.TimeoutSeconds,
					SkipPull:       info.SkipPull,
					SkipHTTPProbe:  info.SkipHTTPProbe,
					ServicesReady:  info.ServicesReady,
				}

				return res, nil
			})
		},
	}

	cmd.Flags().StringVar(&projectDir, "project-dir", "", "docker compose project directory")
	cmd.Flags().StringVar(&composeFile, "compose-file", "", "docker compose file path")
	cmd.Flags().StringVar(&envFile, "env-file", "", "docker compose env file path")
	cmd.Flags().StringVar(&siteURL, "site-url", "", "application URL to probe after startup")
	cmd.Flags().IntVar(&timeoutSeconds, "timeout", 300, "shared readiness timeout in seconds")
	cmd.Flags().BoolVar(&skipPull, "skip-pull", false, "skip docker compose pull")
	cmd.Flags().BoolVar(&skipHTTPProbe, "skip-http-probe", false, "skip the final HTTP probe")

	return cmd
}

type updateRuntimeInput struct {
	projectDir     string
	composeFile    string
	envFile        string
	siteURL        string
	timeoutSeconds int
	skipPull       bool
	skipHTTPProbe  bool
}

func validateUpdateRuntimeInput(cmd *cobra.Command, in *updateRuntimeInput) error {
	in.projectDir = strings.TrimSpace(in.projectDir)
	in.composeFile = strings.TrimSpace(in.composeFile)
	in.envFile = strings.TrimSpace(in.envFile)
	in.siteURL = strings.TrimSpace(in.siteURL)

	if err := requireNonBlankFlag("--project-dir", in.projectDir); err != nil {
		return err
	}
	if err := requireNonBlankFlag("--compose-file", in.composeFile); err != nil {
		return err
	}
	if err := requireNonBlankFlag("--env-file", in.envFile); err != nil {
		return err
	}
	if err := normalizeOptionalStringFlag(cmd, "site-url", &in.siteURL); err != nil {
		return err
	}
	if !in.skipHTTPProbe {
		if err := requireNonBlankFlag("--site-url", in.siteURL); err != nil {
			return err
		}
		if _, err := url.ParseRequestURI(in.siteURL); err != nil {
			return usageError(fmt.Errorf("--site-url must be a valid URL: %w", err))
		}
	}
	if in.timeoutSeconds < 0 {
		return usageError(fmt.Errorf("--timeout must be non-negative"))
	}

	return nil
}

func renderUpdateRuntimeText(w io.Writer, res result.Result) error {
	details, ok := res.Details.(result.UpdateRuntimeDetails)
	if !ok {
		return result.Render(w, res, false)
	}

	if _, err := fmt.Fprintln(w, "[4/6] Updating images"); err != nil {
		return err
	}
	if details.SkipPull {
		if _, err := fmt.Fprintln(w, "Image pull skipped because of --skip-pull"); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintln(w, "[5/6] Restarting the stack with the current configuration"); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(w, "[6/6] Checking readiness after the update"); err != nil {
		return err
	}
	if details.SkipHTTPProbe {
		if _, err := fmt.Fprintln(w, "HTTP probe skipped because of --skip-http-probe"); err != nil {
			return err
		}
	}
	_, err := fmt.Fprintln(w, "Update runtime completed")
	return err
}
