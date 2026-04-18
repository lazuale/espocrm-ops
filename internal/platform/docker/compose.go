package docker

import (
	"fmt"
	"strings"
)

type ComposeConfig struct {
	ProjectDir  string
	ComposeFile string
	EnvFile     string
}

type ComposeServiceState struct {
	Status        string
	HealthMessage string
}

func ComposePull(cfg ComposeConfig) error {
	if _, err := runCompose(cfg, "pull"); err != nil {
		return fmt.Errorf("compose pull: %w", err)
	}

	return nil
}

func ComposeUp(cfg ComposeConfig, services ...string) error {
	args := append([]string{"up", "-d"}, services...)
	if _, err := runCompose(cfg, args...); err != nil {
		return fmt.Errorf("compose up -d: %w", err)
	}

	return nil
}

func ComposeStop(cfg ComposeConfig, services ...string) error {
	args := append([]string{"stop"}, services...)
	if _, err := runCompose(cfg, args...); err != nil {
		return fmt.Errorf("compose stop: %w", err)
	}

	return nil
}

func ComposeRunningServices(cfg ComposeConfig) ([]string, error) {
	res, err := runCompose(cfg, "ps", "--status", "running", "--services")
	if err != nil {
		return nil, fmt.Errorf("compose ps --status running --services: %w", err)
	}

	services := []string{}
	for _, line := range strings.Split(strings.ReplaceAll(res.Stdout, "\r", ""), "\n") {
		service := strings.TrimSpace(line)
		if service == "" || composeServiceListContains(services, service) {
			continue
		}
		services = append(services, service)
	}

	return services, nil
}

func composeServiceListContains(services []string, target string) bool {
	for _, service := range services {
		if service == target {
			return true
		}
	}

	return false
}

func ComposeServiceStateFor(cfg ComposeConfig, service string) (ComposeServiceState, error) {
	containerID, err := composeServiceContainerID(cfg, service)
	if err != nil {
		return ComposeServiceState{}, err
	}
	if containerID == "" {
		return ComposeServiceState{}, nil
	}

	status, err := inspectContainerServiceStatus(containerID)
	if err != nil {
		return ComposeServiceState{}, err
	}

	state := ComposeServiceState{
		Status: status,
	}
	if status != "unhealthy" {
		return state, nil
	}

	healthMessage, err := inspectContainerHealthMessage(containerID)
	if err != nil {
		return state, nil
	}
	state.HealthMessage = healthMessage
	return state, nil
}

func composeServiceContainerID(cfg ComposeConfig, service string) (string, error) {
	res, err := runCompose(cfg, "ps", "-q", service)
	if err != nil {
		return "", fmt.Errorf("compose ps -q %s: %w", service, err)
	}

	return strings.TrimSpace(res.Stdout), nil
}

func inspectContainerServiceStatus(containerID string) (string, error) {
	res, err := runCommand(
		commandOptions{Env: dockerCommandEnv()},
		"docker",
		"inspect",
		"--format",
		"{{if .State.Health}}{{.State.Health.Status}}{{else}}{{.State.Status}}{{end}}",
		containerID,
	)
	if err != nil {
		return "", fmt.Errorf("inspect container status %s: %w", containerID, err)
	}

	return strings.TrimSpace(res.Stdout), nil
}

func inspectContainerHealthMessage(containerID string) (string, error) {
	res, err := runCommand(
		commandOptions{Env: dockerCommandEnv()},
		"docker",
		"inspect",
		"--format",
		"{{if .State.Health}}{{range .State.Health.Log}}{{.Output}}{{printf \"\\n\"}}{{end}}{{end}}",
		containerID,
	)
	if err != nil {
		return "", fmt.Errorf("inspect container health log %s: %w", containerID, err)
	}

	lines := strings.Split(strings.ReplaceAll(res.Stdout, "\r", ""), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line != "" {
			return line, nil
		}
	}

	return "", nil
}

func ValidateComposeConfig(cfg ComposeConfig) error {
	if _, err := runCompose(cfg, "config", "-q"); err != nil {
		return fmt.Errorf("compose config -q: %w", err)
	}

	return nil
}

func runCompose(cfg ComposeConfig, args ...string) (Result, error) {
	composeArgs := []string{
		"compose",
		"--project-directory", cfg.ProjectDir,
		"-f", cfg.ComposeFile,
		"--env-file", cfg.EnvFile,
	}
	composeArgs = append(composeArgs, args...)

	return runCommand(commandOptions{
		Env: dockerCommandEnv(),
	}, "docker", composeArgs...)
}
