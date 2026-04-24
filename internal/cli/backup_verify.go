package cli

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/lazuale/espocrm-ops/internal/contract/exitcode"
	"github.com/lazuale/espocrm-ops/internal/ops"
	"github.com/spf13/cobra"
)

func newBackupVerifyCmd() *cobra.Command {
	var manifestPath string

	cmd := &cobra.Command{
		Use:   "verify",
		Short: "Verify backup set from explicit manifest",
		Args:  noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			in := backupVerifyInput{
				manifestPath: manifestPath,
			}
			if err := validateBackupVerifyInput(cmd, &in); err != nil {
				return backupVerifyCommandError{
					kind: ops.ErrorKindUsage,
					err:  err,
				}
			}

			result, err := ops.VerifyBackup(cmd.Context(), in.manifestPath)
			if err != nil {
				return backupVerifyCommandError{
					kind: backupVerifyErrorKind(err),
					err:  err,
				}
			}

			return renderBackupVerifySuccess(cmd, result)
		},
	}

	cmd.Flags().StringVar(&manifestPath, "manifest", "", "path to backup manifest.json")

	return cmd
}

type backupVerifyInput struct {
	manifestPath string
}

func validateBackupVerifyInput(cmd *cobra.Command, in *backupVerifyInput) error {
	if err := normalizeOptionalAbsolutePathFlag(cmd, "manifest", &in.manifestPath); err != nil {
		return err
	}
	if in.manifestPath != "" {
		return nil
	}

	return usageError(fmt.Errorf("--manifest is required"))
}

type backupVerifyEnvelope struct {
	Command  string                    `json:"command"`
	OK       bool                      `json:"ok"`
	Message  string                    `json:"message"`
	Error    *backupVerifyErrorPayload `json:"error"`
	Warnings []string                  `json:"warnings"`
	Result   backupVerifyResultPayload `json:"result"`
}

type backupVerifyErrorPayload struct {
	Kind    string `json:"kind"`
	Message string `json:"message"`
}

type backupVerifyResultPayload struct {
	Manifest    string `json:"manifest,omitempty"`
	Scope       string `json:"scope,omitempty"`
	CreatedAt   string `json:"created_at,omitempty"`
	DBBackup    string `json:"db_backup,omitempty"`
	FilesBackup string `json:"files_backup,omitempty"`
}

func renderBackupVerifySuccess(cmd *cobra.Command, result ops.VerifyResult) error {
	envelope := backupVerifyEnvelope{
		Command:  "backup verify",
		OK:       true,
		Message:  "backup verified",
		Warnings: []string{},
		Result: backupVerifyResultPayload{
			Manifest:    result.Manifest,
			Scope:       result.Scope,
			CreatedAt:   result.CreatedAt,
			DBBackup:    result.DBBackup,
			FilesBackup: result.FilesBackup,
		},
	}

	if appForCommand(cmd).JSONEnabled() {
		return writeBackupVerifyJSON(cmd.OutOrStdout(), envelope)
	}

	_, err := fmt.Fprintf(cmd.OutOrStdout(), "backup verified: %s\n", result.Manifest)
	return err
}

func writeBackupVerifyJSON(w io.Writer, envelope backupVerifyEnvelope) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(envelope)
}

func backupVerifyErrorKind(err error) string {
	verifyErr, ok := err.(*ops.VerifyError)
	if ok && verifyErr.Kind != "" {
		return verifyErr.Kind
	}

	return ops.ErrorKindIO
}

func backupVerifyExitCode(kind string) int {
	switch kind {
	case ops.ErrorKindUsage:
		return exitcode.UsageError
	case ops.ErrorKindManifest:
		return exitcode.ManifestError
	case ops.ErrorKindArtifact, ops.ErrorKindChecksum, ops.ErrorKindArchive:
		return exitcode.ValidationError
	case ops.ErrorKindIO:
		return exitcode.FilesystemError
	default:
		return exitcode.InternalError
	}
}

type backupVerifyCommandError struct {
	kind string
	err  error
}

func (e backupVerifyCommandError) Error() string {
	if e.err == nil {
		return "backup verify failed"
	}
	return e.err.Error()
}

func (e backupVerifyCommandError) Unwrap() error {
	return e.err
}

func (e backupVerifyCommandError) ExitCode() int {
	return backupVerifyExitCode(e.kind)
}

func (e backupVerifyCommandError) FailureCode() string {
	switch e.kind {
	case ops.ErrorKindUsage:
		return "usage_error"
	case ops.ErrorKindManifest:
		return "manifest_invalid"
	default:
		return "backup_verification_failed"
	}
}

func (e backupVerifyCommandError) ErrorKind() string {
	return e.kind
}
