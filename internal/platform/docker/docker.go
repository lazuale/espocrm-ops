package docker

import (
	"bytes"
	"io"
	"os"
	"os/exec"
	"strings"
)

type Result struct {
	Stdout string
	Stderr string
}

func Run(name string, args ...string) (Result, error) {
	return runCommand(commandOptions{}, name, args...)
}

type commandOptions struct {
	Stdin io.Reader
	Env   []string
}

func runCommand(opts commandOptions, name string, args ...string) (Result, error) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	cmd := exec.Command(name, args...)
	cmd.Stdin = opts.Stdin
	if len(opts.Env) != 0 {
		cmd.Env = opts.Env
	}
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	res := Result{
		Stdout: stdout.String(),
		Stderr: stderr.String(),
	}
	if err != nil {
		return res, CommandRunError{
			Name:   name,
			Args:   append([]string(nil), args...),
			Err:    err,
			Stderr: res.Stderr,
		}
	}

	return res, nil
}

// dockerCommandEnv keeps only the runtime vars needed by Docker CLI and test
// doubles instead of inheriting the whole process environment into backend exec.
func dockerCommandEnv(extra ...string) []string {
	env := make([]string, 0, len(extra)+8)
	for _, entry := range os.Environ() {
		key := envKey(entry)
		if !shouldKeepDockerEnv(key) {
			continue
		}
		env = setEnvEntry(env, entry)
	}
	for _, entry := range extra {
		if entry == "" {
			continue
		}
		env = setEnvEntry(env, entry)
	}

	return env
}

func shouldKeepDockerEnv(key string) bool {
	switch key {
	case "PATH", "HOME", "USERPROFILE", "XDG_CONFIG_HOME", "XDG_RUNTIME_DIR",
		"SSH_AUTH_SOCK", "SSL_CERT_FILE", "SSL_CERT_DIR", "TMPDIR", "TMP", "TEMP",
		"LANG", "LANGUAGE", "HTTP_PROXY", "HTTPS_PROXY", "NO_PROXY", "ALL_PROXY",
		"http_proxy", "https_proxy", "no_proxy", "all_proxy":
		return true
	}

	return strings.HasPrefix(key, "DOCKER_") || strings.HasPrefix(key, "LC_")
}

func setEnvEntry(env []string, entry string) []string {
	key := envKey(entry)
	for i, current := range env {
		if envKey(current) == key {
			env[i] = entry
			return env
		}
	}

	return append(env, entry)
}

func envKey(entry string) string {
	if idx := strings.IndexByte(entry, '='); idx >= 0 {
		return entry[:idx]
	}

	return entry
}

func CheckDockerCLIAvailable() error {
	if _, err := exec.LookPath("docker"); err != nil {
		return UnavailableError{Err: err}
	}

	return nil
}

func DockerClientVersion() (string, error) {
	if err := CheckDockerCLIAvailable(); err != nil {
		return "", err
	}

	res, err := runCommand(commandOptions{Env: dockerCommandEnv()}, "docker", "version", "--format", "{{.Client.Version}}")
	if err != nil {
		return "", UnavailableError{Err: err}
	}

	return strings.TrimSpace(res.Stdout), nil
}

func DockerServerVersion() (string, error) {
	if err := CheckDockerCLIAvailable(); err != nil {
		return "", err
	}

	res, err := runCommand(commandOptions{Env: dockerCommandEnv()}, "docker", "version", "--format", "{{.Server.Version}}")
	if err != nil {
		return "", UnavailableError{Err: err}
	}

	return strings.TrimSpace(res.Stdout), nil
}

func ComposeVersion() (string, error) {
	if err := CheckDockerCLIAvailable(); err != nil {
		return "", err
	}

	res, err := runCommand(commandOptions{Env: dockerCommandEnv()}, "docker", "compose", "version", "--short")
	if err != nil {
		return "", UnavailableError{Err: err}
	}

	return strings.TrimSpace(res.Stdout), nil
}

func CheckDockerAvailable() error {
	_, err := DockerServerVersion()
	if err != nil {
		return err
	}
	return nil
}

func CheckContainerRunning(container string) error {
	res, err := Run("docker", "inspect", "--format", "{{.State.Running}}", container)
	if err != nil {
		return ContainerInspectError{Container: container, Err: err}
	}
	if strings.TrimSpace(res.Stdout) != "true" {
		return ContainerNotRunningError{Container: container}
	}

	return nil
}
