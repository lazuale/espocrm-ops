package doctor

import (
	"fmt"
	"strconv"
	"strings"
)

func (s Service) checkDocker(report *Report) dockerState {
	state := dockerState{}

	clientVersion, err := s.runtime.DockerClientVersion()
	if err != nil {
		report.fail("", "docker_cli", "Docker CLI is not available", err.Error(), "Install Docker and ensure the `docker` binary is on PATH.")
		return state
	}
	state.cliReady = true
	state.cliVersion = clientVersion
	report.ok("", "docker_cli", "Docker CLI is available", fmt.Sprintf("docker %s", clientVersion))

	serverVersion, err := s.runtime.DockerServerVersion()
	if err != nil {
		report.fail("", "docker_daemon", "Docker daemon is not reachable", err.Error(), "Start the Docker daemon and verify that `docker version` can reach the server.")
	} else {
		state.daemonReady = true
		state.serverVersion = serverVersion
		if versionAtLeast(serverVersion, "24.0.0") {
			report.ok("", "docker_daemon", "Docker daemon is reachable", fmt.Sprintf("server %s", serverVersion))
		} else {
			report.warn("", "docker_daemon", "Docker daemon is reachable but below the recommended version", fmt.Sprintf("server %s; recommended minimum 24.0.0", serverVersion), "Upgrade Docker Engine to reduce compatibility risk before running stateful operations.")
		}
	}

	composeVersion, err := s.runtime.ComposeVersion()
	if err != nil {
		report.fail("", "docker_compose", "Docker Compose is not available", err.Error(), "Install Docker Compose v2 and verify that `docker compose version` succeeds.")
		return state
	}
	state.composeReady = true
	state.composeVersion = composeVersion
	if versionAtLeast(composeVersion, "2.20.0") {
		report.ok("", "docker_compose", "Docker Compose is available", fmt.Sprintf("compose %s", composeVersion))
	} else {
		report.warn("", "docker_compose", "Docker Compose is available but below the recommended version", fmt.Sprintf("compose %s; recommended minimum 2.20.0", composeVersion), "Upgrade Docker Compose to reduce compatibility risk before running stateful operations.")
	}

	return state
}

func (s Service) checkComposeConfig(report *Report, scope string, targetPath string) {
	target := runtimeTarget(report.ProjectDir, report.ComposeFile, targetPath)
	if err := s.runtime.ValidateComposeConfig(target); err != nil {
		report.fail(scope, "compose_config", "Docker Compose config validation failed", err.Error(), "Run `docker compose config -q` for the same env file, fix the reported configuration error, and rerun doctor.")
		return
	}

	report.ok(scope, "compose_config", "Docker Compose config validation passed", fmt.Sprintf("compose file %s with env %s", report.ComposeFile, targetPath))
}

func (s Service) checkRunningServices(report *Report, scope string, envFile string) {
	target := runtimeTarget(report.ProjectDir, report.ComposeFile, envFile)
	services, err := s.runtime.RunningServices(target)
	if err != nil {
		report.fail(scope, "running_services", "Could not inspect running services", err.Error(), "Check Docker access for this contour and rerun doctor.")
		return
	}

	if len(services) == 0 {
		report.ok(scope, "running_services", "No services are currently running for this contour", "The runtime health probe is not required while the contour is stopped.")
		return
	}

	unhealthy := []string{}
	for _, service := range services {
		state, err := s.runtime.ServiceState(target, service)
		if err != nil {
			report.fail(scope, "running_services", "Could not inspect the running service health", err.Error(), "Check the service containers with `docker compose ps` and rerun doctor.")
			return
		}

		switch state.Status {
		case "", "running", "healthy":
		case "unhealthy":
			if strings.TrimSpace(state.HealthMessage) != "" {
				unhealthy = append(unhealthy, fmt.Sprintf("%s: %s", service, state.HealthMessage))
			} else {
				unhealthy = append(unhealthy, fmt.Sprintf("%s: reported unhealthy", service))
			}
		default:
			unhealthy = append(unhealthy, fmt.Sprintf("%s: reported %s", service, state.Status))
		}
	}

	if len(unhealthy) != 0 {
		report.fail(scope, "running_services", "A running service is unhealthy", strings.Join(unhealthy, "; "), "Repair the unhealthy service state before running a stateful operation.")
		return
	}

	report.ok(scope, "running_services", "Running services are healthy", strings.Join(services, ", "))
}

func versionAtLeast(current, minimum string) bool {
	currentParts := parseVersion(current)
	minimumParts := parseVersion(minimum)
	maxLen := max(len(minimumParts), len(currentParts))

	for i := 0; i < maxLen; i++ {
		currentPart := 0
		if i < len(currentParts) {
			currentPart = currentParts[i]
		}
		minimumPart := 0
		if i < len(minimumParts) {
			minimumPart = minimumParts[i]
		}
		if currentPart > minimumPart {
			return true
		}
		if currentPart < minimumPart {
			return false
		}
	}

	return true
}

func parseVersion(raw string) []int {
	raw = strings.TrimPrefix(strings.TrimSpace(raw), "v")
	parts := strings.Split(raw, ".")
	out := make([]int, 0, len(parts))
	for _, part := range parts {
		digits := strings.Builder{}
		for _, ch := range part {
			if ch < '0' || ch > '9' {
				break
			}
			digits.WriteRune(ch)
		}
		if digits.Len() == 0 {
			out = append(out, 0)
			continue
		}
		parsed, err := strconv.Atoi(digits.String())
		if err != nil {
			out = append(out, 0)
			continue
		}
		out = append(out, parsed)
	}

	return out
}
