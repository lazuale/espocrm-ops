package config

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestResolveDBPasswordFromFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "db-password")
	if err := os.WriteFile(path, []byte(" secret\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := ResolveDBPassword(DBConfig{PasswordFile: path})
	if err != nil {
		t.Fatalf("ResolveDBPassword failed: %v", err)
	}
	if got != "secret" {
		t.Fatalf("unexpected password: got %q", got)
	}
}

func TestResolveDBPasswordRejectsPasswordAndFile(t *testing.T) {
	_, err := ResolveDBPassword(DBConfig{
		Password:     "secret",
		PasswordFile: "secret-file",
	})
	if err == nil {
		t.Fatal("expected conflict error")
	}
	var conflictErr PasswordSourceConflictError
	if !errors.As(err, &conflictErr) {
		t.Fatalf("expected PasswordSourceConflictError, got %T: %v", err, err)
	}
}

func TestResolveDBPasswordRequiresExplicitSource(t *testing.T) {
	t.Setenv("ESPOPS_DB_PASSWORD", "env-secret")

	_, err := ResolveDBPassword(DBConfig{})
	if err == nil {
		t.Fatal("expected missing explicit password source error")
	}
	var requiredErr PasswordRequiredError
	if !errors.As(err, &requiredErr) {
		t.Fatalf("expected PasswordRequiredError, got %T: %v", err, err)
	}
}

func TestResolveDBPasswordReportsTypedFileErrors(t *testing.T) {
	t.Run("missing file", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "missing")

		_, err := ResolveDBPassword(DBConfig{PasswordFile: path})
		if err == nil {
			t.Fatal("expected password file read error")
		}
		var readErr PasswordFileReadError
		if !errors.As(err, &readErr) {
			t.Fatalf("expected PasswordFileReadError, got %T: %v", err, err)
		}
		if readErr.Path != path {
			t.Fatalf("unexpected path: %s", readErr.Path)
		}
	})

	t.Run("empty file", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "empty")
		if err := os.WriteFile(path, []byte(" \n"), 0o600); err != nil {
			t.Fatal(err)
		}

		_, err := ResolveDBPassword(DBConfig{PasswordFile: path})
		if err == nil {
			t.Fatal("expected password file empty error")
		}
		var emptyErr PasswordFileEmptyError
		if !errors.As(err, &emptyErr) {
			t.Fatalf("expected PasswordFileEmptyError, got %T: %v", err, err)
		}
		if emptyErr.Path != path {
			t.Fatalf("unexpected path: %s", emptyErr.Path)
		}
	})
}
