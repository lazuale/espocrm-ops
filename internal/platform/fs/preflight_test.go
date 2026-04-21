package fs

import (
	"errors"
	"math"
	"os"
	"path/filepath"
	"testing"
)

func TestEnsureNonEmptyFileReportsTypedErrors(t *testing.T) {
	t.Run("missing file", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "missing.sql.gz")

		_, err := EnsureNonEmptyFile("db backup", path)
		if err == nil {
			t.Fatal("expected stat error")
		}

		var typedErr pathStatError
		if !errors.As(err, &typedErr) {
			t.Fatalf("expected pathStatError, got %T: %v", err, err)
		}
		if typedErr.Path != path {
			t.Fatalf("unexpected path: %s", typedErr.Path)
		}
	})

	t.Run("directory", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "dir")
		if err := os.MkdirAll(path, 0o755); err != nil {
			t.Fatal(err)
		}

		_, err := EnsureNonEmptyFile("db backup", path)
		if err == nil {
			t.Fatal("expected directory error")
		}

		var typedErr fileIsDirectoryError
		if !errors.As(err, &typedErr) {
			t.Fatalf("expected fileIsDirectoryError, got %T: %v", err, err)
		}
		if typedErr.Path != path {
			t.Fatalf("unexpected path: %s", typedErr.Path)
		}
	})

	t.Run("empty file", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "empty.sql.gz")
		if err := os.WriteFile(path, nil, 0o644); err != nil {
			t.Fatal(err)
		}

		_, err := EnsureNonEmptyFile("db backup", path)
		if err == nil {
			t.Fatal("expected empty file error")
		}

		var typedErr fileEmptyError
		if !errors.As(err, &typedErr) {
			t.Fatalf("expected fileEmptyError, got %T: %v", err, err)
		}
		if typedErr.Path != path {
			t.Fatalf("unexpected path: %s", typedErr.Path)
		}
	})
}

func TestEnsureWritableDirReportsTypedError(t *testing.T) {
	tmp := t.TempDir()
	blocker := filepath.Join(tmp, "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	err := EnsureWritableDir(filepath.Join(blocker, "child"))
	if err == nil {
		t.Fatal("expected ensure dir error")
	}

	var typedErr ensureDirError
	if !errors.As(err, &typedErr) {
		t.Fatalf("expected ensureDirError, got %T: %v", err, err)
	}
}

func TestEnsureFreeSpaceReportsTypedErrors(t *testing.T) {
	t.Run("statfs failure", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "missing")

		err := EnsureFreeSpace(path, 1)
		if err == nil {
			t.Fatal("expected free space check error")
		}

		var typedErr freeSpaceCheckError
		if !errors.As(err, &typedErr) {
			t.Fatalf("expected freeSpaceCheckError, got %T: %v", err, err)
		}
		if typedErr.Path != path {
			t.Fatalf("unexpected path: %s", typedErr.Path)
		}
	})

	t.Run("insufficient space", func(t *testing.T) {
		err := EnsureFreeSpace(t.TempDir(), math.MaxUint64)
		if err == nil {
			t.Fatal("expected insufficient free space error")
		}

		var typedErr insufficientFreeSpaceError
		if !errors.As(err, &typedErr) {
			t.Fatalf("expected insufficientFreeSpaceError, got %T: %v", err, err)
		}
	})
}
