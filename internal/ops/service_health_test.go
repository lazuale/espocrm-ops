package ops

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	runtime "github.com/lazuale/espocrm-ops/internal/runtime"
)

func TestWaitForRuntimeServiceHealthRetriesUntilHealthy(t *testing.T) {
	rt := &fakeServiceHealthRuntime{
		errors: map[int]error{
			1: &runtime.ServiceHealthError{
				Service:   "db",
				State:     "running",
				Health:    "starting",
				Message:   `service "db" health is "starting" (want "healthy")`,
				Retryable: true,
			},
			2: &runtime.ServiceHealthError{
				Service:   "db",
				State:     "running",
				Health:    "starting",
				Message:   `service "db" health is "starting" (want "healthy")`,
				Retryable: true,
			},
		},
	}

	oldSleep := serviceHealthSleep
	sleepCalls := 0
	serviceHealthSleep = func(context.Context, time.Duration) error {
		sleepCalls++
		return nil
	}
	defer func() {
		serviceHealthSleep = oldSleep
	}()

	err := waitForRuntimeServiceHealth(context.Background(), runtime.Target{}, "db", []string{"app"}, rt)
	if err != nil {
		t.Fatalf("waitForRuntimeServiceHealth failed: %v", err)
	}
	if rt.calls != 3 {
		t.Fatalf("unexpected health calls: got %d want 3", rt.calls)
	}
	if sleepCalls != 2 {
		t.Fatalf("unexpected sleep calls: got %d want 2", sleepCalls)
	}
	if got := strings.Join(rt.services[0], ","); got != "db,app" {
		t.Fatalf("unexpected service contract: %q", got)
	}
}

func TestWaitForRuntimeServiceHealthRetriesUnhealthyUntilHealthy(t *testing.T) {
	rt := &fakeServiceHealthRuntime{
		errors: map[int]error{
			1: &runtime.ServiceHealthError{
				Service: "espocrm",
				State:   "running",
				Health:  "unhealthy",
				Message: `service "espocrm" health is "unhealthy" (want "healthy")`,
			},
		},
	}

	oldSleep := serviceHealthSleep
	sleepCalls := 0
	serviceHealthSleep = func(context.Context, time.Duration) error {
		sleepCalls++
		return nil
	}
	defer func() {
		serviceHealthSleep = oldSleep
	}()

	err := waitForRuntimeServiceHealth(context.Background(), runtime.Target{}, "db", []string{"espocrm"}, rt)
	if err != nil {
		t.Fatalf("waitForRuntimeServiceHealth failed: %v", err)
	}
	if rt.calls != 2 {
		t.Fatalf("unexpected health calls: got %d want 2", rt.calls)
	}
	if sleepCalls != 1 {
		t.Fatalf("unexpected sleep calls: got %d want 1", sleepCalls)
	}
}

func TestWaitForRuntimeServiceHealthTerminalFailuresDoNotRetry(t *testing.T) {
	tests := []struct {
		name string
		err  error
	}{
		{
			name: "not found",
			err: &runtime.ServiceHealthError{
				Service: "db",
				Message: `service "db" not found in docker compose ps output`,
			},
		},
		{
			name: "exited",
			err: &runtime.ServiceHealthError{
				Service: "db",
				State:   "exited",
				Health:  "unhealthy",
				Message: `service "db" state is "exited" (want "running")`,
			},
		},
		{
			name: "no healthcheck",
			err: &runtime.ServiceHealthError{
				Service: "db",
				State:   "running",
				Message: `service "db" has no docker compose health status`,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rt := &fakeServiceHealthRuntime{
				errors: map[int]error{1: tt.err},
			}

			oldSleep := serviceHealthSleep
			sleepCalls := 0
			serviceHealthSleep = func(context.Context, time.Duration) error {
				sleepCalls++
				return nil
			}
			defer func() {
				serviceHealthSleep = oldSleep
			}()

			err := waitForRuntimeServiceHealth(context.Background(), runtime.Target{}, "db", []string{"app"}, rt)
			if err == nil {
				t.Fatal("expected error")
			}
			if rt.calls != 1 {
				t.Fatalf("unexpected health calls: got %d want 1", rt.calls)
			}
			if sleepCalls != 0 {
				t.Fatalf("unexpected sleep calls: got %d want 0", sleepCalls)
			}
		})
	}
}

func TestWaitForRuntimeServiceHealthTimeoutIncludesLastStatus(t *testing.T) {
	rt := &fakeServiceHealthRuntime{
		errors: map[int]error{
			1: &runtime.ServiceHealthError{
				Service:   "espocrm",
				State:     "running",
				Health:    "starting",
				Message:   `service "espocrm" health is "starting" (want "healthy")`,
				Retryable: true,
			},
			2: &runtime.ServiceHealthError{
				Service: "espocrm",
				State:   "running",
				Health:  "unhealthy",
				Message: `service "espocrm" health is "unhealthy" (want "healthy")`,
			},
		},
	}

	oldSleep := serviceHealthSleep
	sleepCalls := 0
	serviceHealthSleep = func(context.Context, time.Duration) error {
		sleepCalls++
		if sleepCalls == 1 {
			return nil
		}
		return context.DeadlineExceeded
	}
	defer func() {
		serviceHealthSleep = oldSleep
	}()

	err := waitForRuntimeServiceHealth(context.Background(), runtime.Target{}, "db", []string{"espocrm"}, rt)
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected deadline exceeded, got %v", err)
	}
	if !strings.Contains(err.Error(), `service "espocrm"`) {
		t.Fatalf("expected service name in error, got %v", err)
	}
	if !strings.Contains(err.Error(), `health is "unhealthy"`) {
		t.Fatalf("expected last health status in error, got %v", err)
	}
	if !strings.Contains(err.Error(), "timed out waiting for docker compose service health") {
		t.Fatalf("unexpected error: %v", err)
	}
}

type fakeServiceHealthRuntime struct {
	errors   map[int]error
	calls    int
	services [][]string
}

func (f *fakeServiceHealthRuntime) RequireHealthyServices(_ context.Context, _ runtime.Target, services []string) error {
	f.calls++
	f.services = append(f.services, append([]string(nil), services...))
	if f.errors == nil {
		return nil
	}
	return f.errors[f.calls]
}
