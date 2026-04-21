package config

import (
	"errors"
	"os"
	"strings"
	"testing"
)

func TestLoadOperationEnvLoadsOverrideWithExplicitContour(t *testing.T) {
	dir := t.TempDir()
	values := baseOperationEnvValues("dev")
	path := writeOperationEnvValuesFile(t, dir, "ops.env", values, 0o640)

	env, err := LoadOperationEnv("", "dev", path)
	if err != nil {
		t.Fatalf("LoadOperationEnv failed: %v", err)
	}
	if env.FilePath != path {
		t.Fatalf("unexpected env file path: %s", env.FilePath)
	}
	if env.ResolvedContour != "dev" {
		t.Fatalf("unexpected contour: %s", env.ResolvedContour)
	}
	if env.ComposeProject() != values["COMPOSE_PROJECT_NAME"] {
		t.Fatalf("unexpected compose project: %s", env.ComposeProject())
	}
}

func TestLoadOperationEnvRejectsUnsupportedContourEvenWithOverride(t *testing.T) {
	dir := t.TempDir()
	path := writeOperationEnvValuesFile(t, dir, "ops.env", baseOperationEnvValues("dev"), 0o640)

	_, err := LoadOperationEnv("", "qa", path)
	if err == nil {
		t.Fatal("expected unsupported contour error")
	}

	var contourErr UnsupportedContourError
	if !errors.As(err, &contourErr) {
		t.Fatalf("expected UnsupportedContourError, got %T: %v", err, err)
	}
}

func TestLoadOperationEnvRejectsEmptyProjectDirWithoutOverride(t *testing.T) {
	_, err := LoadOperationEnv("", "dev", "")
	if err == nil {
		t.Fatal("expected invalid env file error")
	}

	var invalidErr InvalidEnvFileError
	if !errors.As(err, &invalidErr) {
		t.Fatalf("expected InvalidEnvFileError, got %T: %v", err, err)
	}
	if !strings.Contains(err.Error(), "project dir is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLoadOperationEnvRejectsMissingRequiredValue(t *testing.T) {
	dir := t.TempDir()
	values := baseOperationEnvValues("dev")
	delete(values, "BACKUP_ROOT")
	writeOperationEnvValuesFile(t, dir, ".env.dev", values, 0o640)

	_, err := LoadOperationEnv(dir, "dev", "")
	if err == nil {
		t.Fatal("expected missing env value error")
	}

	var missingErr MissingEnvValueError
	if !errors.As(err, &missingErr) {
		t.Fatalf("expected MissingEnvValueError, got %T: %v", err, err)
	}
	if missingErr.Name != "BACKUP_ROOT" {
		t.Fatalf("unexpected missing key: %s", missingErr.Name)
	}
}

func TestLoadOperationEnvRejectsDuplicateAssignments(t *testing.T) {
	dir := t.TempDir()
	writeOperationEnvLinesFile(t, dir, ".env.dev", []string{
		"ESPO_CONTOUR=dev",
		"COMPOSE_PROJECT_NAME=one",
		"COMPOSE_PROJECT_NAME=two",
		"DB_STORAGE_DIR=./runtime/dev/db",
		"ESPO_STORAGE_DIR=./runtime/dev/espo",
		"BACKUP_ROOT=./backups/dev",
	}, 0o640)

	_, err := LoadOperationEnv(dir, "dev", "")
	if err == nil {
		t.Fatal("expected parse error")
	}

	var parseErr EnvParseError
	if !errors.As(err, &parseErr) {
		t.Fatalf("expected EnvParseError, got %T: %v", err, err)
	}
	if !strings.Contains(err.Error(), "duplicate assignment") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLoadOperationEnvRejectsSymlinkOverride(t *testing.T) {
	dir := t.TempDir()
	target := writeOperationEnvValuesFile(t, dir, "real.env", baseOperationEnvValues("dev"), 0o640)
	link := dir + "/linked.env"
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}

	_, err := LoadOperationEnv("", "dev", link)
	if err == nil {
		t.Fatal("expected invalid env file error")
	}

	var invalidErr InvalidEnvFileError
	if !errors.As(err, &invalidErr) {
		t.Fatalf("expected InvalidEnvFileError, got %T: %v", err, err)
	}
	if !strings.Contains(err.Error(), "must not be a symlink") {
		t.Fatalf("unexpected error: %v", err)
	}
}
