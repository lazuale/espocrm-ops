package update

import (
	"fmt"
	"net/http"
	"time"

	"github.com/lazuale/espocrm-ops/internal/contract/apperr"
	platformdocker "github.com/lazuale/espocrm-ops/internal/platform/docker"
)

const readinessPollInterval = 5 * time.Second

var runtimeServices = []string{
	"db",
	"espocrm",
	"espocrm-daemon",
	"espocrm-websocket",
}

type RuntimeApplyRequest struct {
	ProjectDir     string
	ComposeFile    string
	EnvFile        string
	SiteURL        string
	TimeoutSeconds int
	SkipPull       bool
	SkipHTTPProbe  bool
}

type RuntimeApplyInfo struct {
	TimeoutSeconds int
	SkipPull       bool
	SkipHTTPProbe  bool
	ServicesReady  []string
}

func ApplyRuntime(req RuntimeApplyRequest) (RuntimeApplyInfo, error) {
	info := RuntimeApplyInfo{
		TimeoutSeconds: req.TimeoutSeconds,
		SkipPull:       req.SkipPull,
		SkipHTTPProbe:  req.SkipHTTPProbe,
	}

	cfg := platformdocker.ComposeConfig{
		ProjectDir:  req.ProjectDir,
		ComposeFile: req.ComposeFile,
		EnvFile:     req.EnvFile,
	}

	if !req.SkipPull {
		if err := platformdocker.ComposePull(cfg); err != nil {
			return info, apperr.Wrap(apperr.KindExternal, "update_runtime_failed", err)
		}
	}

	if err := platformdocker.ComposeUp(cfg); err != nil {
		return info, apperr.Wrap(apperr.KindExternal, "update_runtime_failed", err)
	}

	deadline := time.Now().UTC().Add(time.Duration(req.TimeoutSeconds) * time.Second)
	for _, service := range runtimeServices {
		if err := waitForServiceReady(cfg, service, deadline, req.TimeoutSeconds); err != nil {
			return info, apperr.Wrap(apperr.KindExternal, "update_runtime_failed", err)
		}
		info.ServicesReady = append(info.ServicesReady, service)
	}

	if !req.SkipHTTPProbe {
		if err := httpProbe(req.SiteURL); err != nil {
			return info, apperr.Wrap(apperr.KindExternal, "update_runtime_failed", err)
		}
	}

	return info, nil
}

func waitForServiceReady(cfg platformdocker.ComposeConfig, service string, deadline time.Time, timeoutSeconds int) error {
	for {
		state, err := platformdocker.ComposeServiceStateFor(cfg, service)
		if err != nil {
			return err
		}

		switch state.Status {
		case "healthy", "running":
			return nil
		case "exited", "dead":
			return fmt.Errorf("service '%s' crashed while waiting for readiness", service)
		case "unhealthy":
			if state.HealthMessage != "" {
				return fmt.Errorf("service '%s' became unhealthy while waiting for readiness: %s", service, state.HealthMessage)
			}
			return fmt.Errorf("service '%s' became unhealthy while waiting for readiness", service)
		}

		remaining := time.Until(deadline)
		if remaining <= 0 {
			return fmt.Errorf("timed out while waiting for service readiness '%s' (%d sec.)", service, timeoutSeconds)
		}

		sleepFor := readinessPollInterval
		if remaining < sleepFor {
			sleepFor = remaining
		}
		time.Sleep(sleepFor)
	}
}

func httpProbe(siteURL string) error {
	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	resp, err := client.Get(siteURL)
	if err != nil {
		return fmt.Errorf("http probe failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusBadRequest {
		return fmt.Errorf("http probe failed: unexpected status %s", resp.Status)
	}

	return nil
}
