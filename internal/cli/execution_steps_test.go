package cli

import "testing"

type fakeStatus string

func (s fakeStatus) String() string { return string(s) }

func TestWireStepStatusMapsPlannedToWouldRun(t *testing.T) {
	item := newSectionItem("runtime_prepare", fakeStatus("planned"), "Runtime preparation would run", "", "")

	if item.Status != "would_run" {
		t.Fatalf("expected planned status to map to would_run, got %q", item.Status)
	}
}

func TestWireStepStatusPreservesNonPlannedStatuses(t *testing.T) {
	item := newSectionItem("runtime_prepare", fakeStatus("completed"), "Runtime preparation completed", "", "")

	if item.Status != "completed" {
		t.Fatalf("expected completed status to stay unchanged, got %q", item.Status)
	}
}
