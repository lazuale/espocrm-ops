package fs

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInspectDirReadiness_RejectsBlankPath(t *testing.T) {
	_, err := InspectDirReadiness("", 0, false)
	if err == nil {
		t.Fatal("expected blank path to fail")
	}
	if !strings.Contains(err.Error(), "path must not be blank") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestInspectDirReadiness_UsesNearestExistingParentForMissingPath(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "nested", "storage")

	readiness, err := InspectDirReadiness(path, 0, false)
	if err != nil {
		t.Fatalf("InspectDirReadiness failed: %v", err)
	}
	if readiness.Path != path {
		t.Fatalf("unexpected path: %s", readiness.Path)
	}
	if readiness.ProbePath != tmp {
		t.Fatalf("unexpected probe path: %s", readiness.ProbePath)
	}
	if readiness.Exists {
		t.Fatal("expected missing path to report Exists=false")
	}
	if !readiness.Writable || !readiness.Creatable {
		t.Fatalf("expected missing path to be creatable via %s: %+v", tmp, readiness)
	}
}

func TestInspectDirReadiness_RejectsExistingFilePath(t *testing.T) {
	tmp := t.TempDir()
	filePath := filepath.Join(tmp, "storage")
	writeTestFile(t, filePath, []byte("not a dir"))

	_, err := InspectDirReadiness(filePath, 0, false)
	if err == nil {
		t.Fatal("expected file path to fail")
	}
	if !strings.Contains(err.Error(), "is not a directory") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func writeTestFile(t *testing.T, path string, body []byte) {
	t.Helper()

	if err := os.WriteFile(path, body, 0o644); err != nil {
		t.Fatal(err)
	}
}
