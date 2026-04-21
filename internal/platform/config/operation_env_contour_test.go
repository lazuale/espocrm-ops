package config

import (
	"errors"
	"strings"
	"testing"
)

func TestResolveLoadedEnvContourUsesFilenameTokenWhenDeclaredContourMissing(t *testing.T) {
	contour, err := resolveLoadedEnvContour("/tmp/custom.dev.env", "dev", "")
	if err != nil {
		t.Fatalf("resolveLoadedEnvContour failed: %v", err)
	}
	if contour != "dev" {
		t.Fatalf("unexpected contour: %s", contour)
	}
}

func TestResolveLoadedEnvContourRejectsAmbiguousFilenameEvenWithDeclaredContour(t *testing.T) {
	_, err := resolveLoadedEnvContour("/tmp/custom-dev-prod.env", "dev", "dev")
	if err == nil {
		t.Fatal("expected invalid env file error")
	}

	var invalidErr InvalidEnvFileError
	if !errors.As(err, &invalidErr) {
		t.Fatalf("expected InvalidEnvFileError, got %T: %v", err, err)
	}
	if !strings.Contains(err.Error(), "contains both dev and prod") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestResolveLoadedEnvContourRejectsFilenameDeclaredConflict(t *testing.T) {
	_, err := resolveLoadedEnvContour("/tmp/custom.prod.env", "prod", "dev")
	if err == nil {
		t.Fatal("expected invalid env file error")
	}

	var invalidErr InvalidEnvFileError
	if !errors.As(err, &invalidErr) {
		t.Fatalf("expected InvalidEnvFileError, got %T: %v", err, err)
	}
	if !strings.Contains(err.Error(), "filename points to contour") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestResolveLoadedEnvContourRejectsMissingContourSignal(t *testing.T) {
	_, err := resolveLoadedEnvContour("/tmp/custom.env", "dev", "")
	if err == nil {
		t.Fatal("expected invalid env file error")
	}

	var invalidErr InvalidEnvFileError
	if !errors.As(err, &invalidErr) {
		t.Fatalf("expected InvalidEnvFileError, got %T: %v", err, err)
	}
	if !strings.Contains(err.Error(), "could not determine env file contour") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestInferEnvFileContourFromPathOnlyUsesBaseFilename(t *testing.T) {
	contour, err := inferEnvFileContourFromPath("/tmp/prod/config.env.dev")
	if err != nil {
		t.Fatalf("inferEnvFileContourFromPath failed: %v", err)
	}
	if contour != "dev" {
		t.Fatalf("unexpected contour: %s", contour)
	}
}
