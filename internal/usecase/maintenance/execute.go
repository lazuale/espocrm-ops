package maintenance

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/lazuale/espocrm-ops/internal/contract/apperr"
	platformconfig "github.com/lazuale/espocrm-ops/internal/platform/config"
	platformlocks "github.com/lazuale/espocrm-ops/internal/platform/locks"
)

type ExecuteShellRequest struct {
	Scope           string
	Operation       string
	ProjectDir      string
	EnvFileOverride string
	EnvContourHint  string
	BaseEnv         []string
	Command         []string
	StreamOutput    bool
	Stdin           io.Reader
	Stdout          io.Writer
	Stderr          io.Writer
	LogWriter       io.Writer
}

type ExecuteShellInfo struct {
	Scope          string
	Operation      string
	EnvFile        string
	ComposeProject string
	BackupRoot     string
	Command        []string
}

type CommandExitError struct {
	Code    int
	Message string
}

func (e CommandExitError) Error() string {
	if strings.TrimSpace(e.Message) != "" {
		return e.Message
	}
	return fmt.Sprintf("command exited with code %d", e.Code)
}

func (e CommandExitError) ExitCode() int {
	return e.Code
}

func ExecuteShell(req ExecuteShellRequest) (ExecuteShellInfo, error) {
	info := ExecuteShellInfo{
		Scope:     strings.TrimSpace(req.Scope),
		Operation: strings.TrimSpace(req.Operation),
		Command:   append([]string(nil), req.Command...),
	}

	if len(req.Command) == 0 {
		return info, apperr.Wrap(apperr.KindValidation, "operation_execute_failed", fmt.Errorf("operation command is required"))
	}

	env, err := platformconfig.LoadOperationEnv(req.ProjectDir, info.Scope, req.EnvFileOverride, req.EnvContourHint)
	if err != nil {
		return info, wrapOperationEnvError(err)
	}

	info.EnvFile = env.FilePath
	info.ComposeProject = env.ComposeProject()
	info.BackupRoot = platformconfig.ResolveProjectPath(req.ProjectDir, env.BackupRoot())

	if err := ensureRuntimeDirs(req.ProjectDir, env); err != nil {
		return info, apperr.Wrap(apperr.KindIO, "operation_execute_failed", err)
	}

	opLock, err := platformlocks.AcquireSharedOperationLock(req.ProjectDir, info.Operation, req.LogWriter)
	if err != nil {
		return info, wrapOperationLockError(err)
	}
	defer func() {
		_ = opLock.Release()
	}()

	maintenanceLock, err := platformlocks.AcquireMaintenanceLock(info.BackupRoot, info.Scope, info.Operation, req.LogWriter)
	if err != nil {
		return info, wrapOperationLockError(err)
	}
	defer func() {
		_ = maintenanceLock.Release()
	}()

	childEnv := platformconfig.ApplyOperationEnv(req.BaseEnv, env, map[string]string{
		"ESPO_MAINTENANCE_LOCK":   "1",
		"ESPO_OPERATION_LOCK":     "1",
		"ESPO_SHELL_EXEC_CONTEXT": "1",
	})

	cmd := exec.Command(req.Command[0], req.Command[1:]...)
	cmd.Env = childEnv
	cmd.Stdin = req.Stdin

	var stdoutBuf bytes.Buffer
	var stderrBuf bytes.Buffer
	if req.StreamOutput {
		cmd.Stdout = req.Stdout
		cmd.Stderr = req.Stderr
	} else {
		cmd.Stdout = &stdoutBuf
		cmd.Stderr = &stderrBuf
	}

	if err := cmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			message := lastNonBlankLine(stderrBuf.String())
			if message == "" {
				message = lastNonBlankLine(stdoutBuf.String())
			}
			return info, apperr.Wrap(apperr.KindExternal, "operation_execute_failed", CommandExitError{
				Code:    exitErr.ExitCode(),
				Message: message,
			})
		}
		return info, apperr.Wrap(apperr.KindExternal, "operation_execute_failed", fmt.Errorf("run operation command: %w", err))
	}

	return info, nil
}

func ensureRuntimeDirs(projectDir string, env platformconfig.OperationEnv) error {
	paths := []string{
		platformconfig.ResolveProjectPath(projectDir, env.DBStorageDir()),
		platformconfig.ResolveProjectPath(projectDir, env.ESPOStorageDir()),
		filepath.Join(platformconfig.ResolveProjectPath(projectDir, env.BackupRoot()), "db"),
		filepath.Join(platformconfig.ResolveProjectPath(projectDir, env.BackupRoot()), "files"),
		filepath.Join(platformconfig.ResolveProjectPath(projectDir, env.BackupRoot()), "locks"),
		filepath.Join(platformconfig.ResolveProjectPath(projectDir, env.BackupRoot()), "manifests"),
		filepath.Join(platformconfig.ResolveProjectPath(projectDir, env.BackupRoot()), "reports"),
		filepath.Join(platformconfig.ResolveProjectPath(projectDir, env.BackupRoot()), "support"),
	}

	for _, path := range paths {
		if err := os.MkdirAll(path, 0o755); err != nil {
			return fmt.Errorf("create runtime directory %s: %w", path, err)
		}
	}

	return nil
}

func wrapOperationEnvError(err error) error {
	switch err.(type) {
	case platformconfig.MissingEnvFileError, platformconfig.InvalidEnvFileError, platformconfig.EnvParseError, platformconfig.MissingEnvValueError, platformconfig.UnsupportedContourError:
		return apperr.Wrap(apperr.KindValidation, "operation_execute_failed", err)
	default:
		return apperr.Wrap(apperr.KindIO, "operation_execute_failed", err)
	}
}

func wrapOperationLockError(err error) error {
	switch err.(type) {
	case platformlocks.MaintenanceConflictError, platformlocks.LegacyMetadataOnlyLockError:
		return apperr.Wrap(apperr.KindConflict, "operation_execute_failed", err)
	default:
		return apperr.Wrap(apperr.KindIO, "operation_execute_failed", err)
	}
}

func lastNonBlankLine(text string) string {
	text = strings.ReplaceAll(text, "\r", "")
	lines := strings.Split(text, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line != "" {
			return line
		}
	}

	return ""
}
