package cli

import (
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/lazuale/espocrm-ops/internal/contract/exitcode"
	"github.com/lazuale/espocrm-ops/internal/contract/result"
	maintenanceusecase "github.com/lazuale/espocrm-ops/internal/usecase/maintenance"
	"github.com/spf13/cobra"
)

func newRunOperationCmd() *cobra.Command {
	var scope string
	var operation string
	var projectDir string
	var envFile string

	cmd := &cobra.Command{
		Use:                "run-operation -- <command> [args...]",
		Short:              "Run a shell-owned operation body behind Go preflight",
		Args:               cobra.ArbitraryArgs,
		Hidden:             true,
		DisableFlagParsing: false,
		FParseErrWhitelist: cobra.FParseErrWhitelist{},
		SilenceUsage:       true,
		SilenceErrors:      true,
		TraverseChildren:   false,
	}

	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		in := runOperationInput{
			scope:      scope,
			operation:  operation,
			projectDir: projectDir,
			envFile:    envFile,
			command:    append([]string(nil), args...),
		}
		if err := validateRunOperationInput(&in); err != nil {
			return err
		}

		jsonEnabled := appForCommand(cmd).JSONEnabled()

		return RunResultCommand(cmd, CommandSpec{
			Name:       "run-operation",
			ErrorCode:  "operation_execute_failed",
			ExitCode:   exitcode.InternalError,
			RenderText: renderRunOperationText,
		}, func() (result.Result, error) {
			logWriter := io.Writer(cmd.ErrOrStderr())
			if jsonEnabled {
				logWriter = nil
			}

			res := result.Result{
				OK: true,
				Artifacts: result.RunOperationArtifacts{
					ProjectDir: in.projectDir,
				},
				Details: result.RunOperationDetails{
					Scope:     in.scope,
					Operation: in.operation,
					Command:   append([]string(nil), in.command...),
				},
			}

			info, err := maintenanceusecase.ExecuteShell(maintenanceusecase.ExecuteShellRequest{
				Scope:           in.scope,
				Operation:       in.operation,
				ProjectDir:      in.projectDir,
				EnvFileOverride: in.envFile,
				EnvContourHint:  envFileContourHint(),
				BaseEnv:         currentProcessEnv(),
				Command:         in.command,
				StreamOutput:    !jsonEnabled,
				Stdin:           cmd.InOrStdin(),
				Stdout:          cmd.OutOrStdout(),
				Stderr:          cmd.ErrOrStderr(),
				LogWriter:       logWriter,
			})
			if err != nil {
				var exitErr interface{ ExitCode() int }
				if errors.As(err, &exitErr) {
					codeErr := CodeError{
						Code:     exitErr.ExitCode(),
						Err:      err,
						ErrCode:  "operation_execute_failed",
						Warnings: warningMessages(err),
					}
					if !jsonEnabled {
						return res, silentCodeError{CodeError: codeErr}
					}
					return res, codeErr
				}
				return res, err
			}

			res.Message = "operation command completed"
			res.Artifacts = result.RunOperationArtifacts{
				ProjectDir: in.projectDir,
				EnvFile:    info.EnvFile,
				BackupRoot: info.BackupRoot,
			}
			res.Details = result.RunOperationDetails{
				Scope:          info.Scope,
				Operation:      info.Operation,
				Command:        append([]string(nil), info.Command...),
				ComposeProject: info.ComposeProject,
			}

			return res, nil
		})
	}

	cmd.Flags().StringVar(&scope, "scope", "", "operation contour")
	cmd.Flags().StringVar(&operation, "operation", "", "operation lock scope")
	cmd.Flags().StringVar(&projectDir, "project-dir", "", "repo project directory")
	cmd.Flags().StringVar(&envFile, "env-file", "", "override env file path")

	return cmd
}

type runOperationInput struct {
	scope      string
	operation  string
	projectDir string
	envFile    string
	command    []string
}

func validateRunOperationInput(in *runOperationInput) error {
	in.scope = strings.TrimSpace(in.scope)
	in.operation = strings.TrimSpace(in.operation)
	in.projectDir = strings.TrimSpace(in.projectDir)
	in.envFile = strings.TrimSpace(in.envFile)

	switch in.scope {
	case "dev", "prod":
	default:
		return usageError(fmt.Errorf("--scope must be dev or prod"))
	}

	switch in.operation {
	case "backup", "update":
	default:
		return usageError(fmt.Errorf("--operation must be backup or update"))
	}

	if err := requireNonBlankFlag("--project-dir", in.projectDir); err != nil {
		return err
	}
	if len(in.command) == 0 {
		return usageError(fmt.Errorf("an operation command is required after --"))
	}

	return nil
}

func renderRunOperationText(w io.Writer, res result.Result) error {
	_ = w
	_ = res
	return nil
}
