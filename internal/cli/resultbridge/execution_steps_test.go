package resultbridge

import "testing"

type fakeStatus string

func (s fakeStatus) String() string { return string(s) }

func TestNewSectionItemPreservesPlannedStatus(t *testing.T) {
	item := newSectionItem("runtime_prepare", fakeStatus("planned"), "Runtime preparation planned", "", "")

	if item.Status != "planned" {
		t.Fatalf("expected planned status to stay unchanged, got %q", item.Status)
	}
}

func TestNewSectionItemPreservesCompletedStatus(t *testing.T) {
	item := newSectionItem("runtime_prepare", fakeStatus("completed"), "Runtime preparation completed", "", "")

	if item.Status != "completed" {
		t.Fatalf("expected completed status to stay unchanged, got %q", item.Status)
	}
}
