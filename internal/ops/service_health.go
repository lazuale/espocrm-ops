package ops

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	runtime "github.com/lazuale/espocrm-ops/internal/runtime"
)

const (
	serviceHealthCheckTimeout  = 2 * time.Minute
	serviceHealthCheckInterval = 1 * time.Second
)

var serviceHealthSleep = sleepWithContext

type serviceHealthRuntime interface {
	RequireHealthyServices(ctx context.Context, target runtime.Target, services []string) error
}

func requireRuntimeServiceHealth(ctx context.Context, target runtime.Target, dbService string, appServices []string, rt serviceHealthRuntime) error {
	return rt.RequireHealthyServices(ctx, target, runtimeContractServices(dbService, appServices))
}

func waitForRuntimeServiceHealth(ctx context.Context, target runtime.Target, dbService string, appServices []string, rt serviceHealthRuntime) error {
	waitCtx, cancel := context.WithTimeout(ctx, serviceHealthCheckTimeout)
	defer cancel()

	services := runtimeContractServices(dbService, appServices)
	attempts := serviceHealthCheckAttempts(serviceHealthCheckTimeout, serviceHealthCheckInterval)
	var lastErr error

	for attempt := 0; attempt < attempts; attempt++ {
		if err := waitCtx.Err(); err != nil {
			if err == context.DeadlineExceeded && lastErr != nil {
				return fmt.Errorf("timed out waiting for docker compose service health: %s: %w", lastErr.Error(), err)
			}
			return err
		}

		err := rt.RequireHealthyServices(waitCtx, target, services)
		if err == nil {
			return nil
		}
		lastErr = err
		if !isRetryableServiceHealthError(err) {
			return err
		}

		if attempt == attempts-1 {
			break
		}
		if err := serviceHealthSleep(waitCtx, serviceHealthCheckInterval); err != nil {
			if err == context.DeadlineExceeded && lastErr != nil {
				return fmt.Errorf("timed out waiting for docker compose service health: %s: %w", lastErr.Error(), err)
			}
			return err
		}
	}

	if lastErr == nil {
		lastErr = context.DeadlineExceeded
	}
	return fmt.Errorf("timed out waiting for docker compose service health: %s: %w", lastErr.Error(), context.DeadlineExceeded)
}

func runtimeContractServices(dbService string, appServices []string) []string {
	services := make([]string, 0, len(appServices)+1)
	services = append(services, dbService)
	services = append(services, appServices...)
	return services
}

func serviceHealthCheckAttempts(timeout, interval time.Duration) int {
	if interval <= 0 {
		return 1
	}
	attempts := int(timeout / interval)
	if timeout%interval != 0 {
		attempts++
	}
	if attempts < 1 {
		attempts = 1
	}
	return attempts + 1
}

func sleepWithContext(ctx context.Context, delay time.Duration) error {
	if delay <= 0 {
		if err := ctx.Err(); err != nil {
			return err
		}
		return nil
	}

	timer := time.NewTimer(delay)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func isRetryableServiceHealthError(err error) bool {
	var healthErr *runtime.ServiceHealthError
	if !errors.As(err, &healthErr) {
		return false
	}
	if strings.ToLower(strings.TrimSpace(healthErr.State)) != "running" {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(healthErr.Health)) {
	case "starting", "unhealthy":
		return true
	default:
		return false
	}
}
