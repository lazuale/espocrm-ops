package config

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateEnvFileForLoadingRejectsDirectories(t *testing.T) {
	err := validateEnvFileForLoading(t.TempDir())
	if err == nil {
		t.Fatal("expected invalid env file error")
	}

	var invalidErr InvalidEnvFileError
	if !errors.As(err, &invalidErr) {
		t.Fatalf("expected InvalidEnvFileError, got %T: %v", err, err)
	}
	if !strings.Contains(err.Error(), "regular file") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateEnvFileForLoadingRejectsBroadPermissions(t *testing.T) {
	path := writeOperationEnvValuesFile(t, t.TempDir(), ".env.dev", baseOperationEnvValues("dev"), 0o644)

	err := validateEnvFileForLoading(path)
	if err == nil {
		t.Fatal("expected invalid env file error")
	}

	var invalidErr InvalidEnvFileError
	if !errors.As(err, &invalidErr) {
		t.Fatalf("expected InvalidEnvFileError, got %T: %v", err, err)
	}
	if !strings.Contains(err.Error(), "broader than 640") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateEnvFileOwnershipRejectsForeignOwner(t *testing.T) {
	err := validateEnvFileOwnership("/tmp/.env.dev", uint32(os.Getuid()+1), os.Getuid())
	if err == nil {
		t.Fatal("expected invalid env file error")
	}

	var invalidErr InvalidEnvFileError
	if !errors.As(err, &invalidErr) {
		t.Fatalf("expected InvalidEnvFileError, got %T: %v", err, err)
	}
	if !strings.Contains(err.Error(), "current user or root") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateEnvFilePermissionsAllowsStrictModes(t *testing.T) {
	for _, perm := range []fs.FileMode{0o400, 0o600, 0o640} {
		t.Run(perm.String(), func(t *testing.T) {
			if err := validateEnvFilePermissions("/tmp/.env.dev", perm); err != nil {
				t.Fatalf("validateEnvFilePermissions(%03o) failed: %v", perm, err)
			}
		})
	}
}

func TestValidateEnvFileForLoadingRejectsSymlink(t *testing.T) {
	dir := t.TempDir()
	target := writeOperationEnvValuesFile(t, dir, "real.env", baseOperationEnvValues("dev"), 0o640)
	link := filepath.Join(dir, "linked.env")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}

	err := validateEnvFileForLoading(link)
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
