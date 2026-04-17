package fs

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestNewSiblingStage_ReturnsTypedErrorWhenParentCannotBeEnsured(t *testing.T) {
	tmp := t.TempDir()
	blocker := filepath.Join(tmp, "blocker")
	targetDir := filepath.Join(blocker, "storage")

	if err := os.WriteFile(blocker, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := NewSiblingStage(targetDir, "espops-stage")
	if err == nil {
		t.Fatal("expected stage creation to fail")
	}

	var typed EnsureDirError
	if !errors.As(err, &typed) {
		t.Fatalf("expected EnsureDirError, got %T", err)
	}
}

func TestPreparedTreeRoot_ReturnsTypedErrors(t *testing.T) {
	t.Run("empty archive", func(t *testing.T) {
		stageDir := t.TempDir()

		_, err := PreparedTreeRoot(stageDir, "storage")
		if err == nil {
			t.Fatal("expected empty archive error")
		}

		var typed StageEmptyError
		if !errors.As(err, &typed) {
			t.Fatalf("expected StageEmptyError, got %T", err)
		}
	})

	t.Run("mixed target root", func(t *testing.T) {
		stageDir := t.TempDir()
		if err := os.MkdirAll(filepath.Join(stageDir, "storage"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(stageDir, "other.txt"), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}

		_, err := PreparedTreeRoot(stageDir, "storage")
		if err == nil {
			t.Fatal("expected mixed root error")
		}

		var typed StageMixedRootError
		if !errors.As(err, &typed) {
			t.Fatalf("expected StageMixedRootError, got %T", err)
		}
	})

	t.Run("exact root mismatch", func(t *testing.T) {
		stageDir := t.TempDir()
		if err := os.MkdirAll(filepath.Join(stageDir, "wrong"), 0o755); err != nil {
			t.Fatal(err)
		}

		_, err := PreparedTreeRootExact(stageDir, "storage")
		if err == nil {
			t.Fatal("expected exact root mismatch")
		}

		var typed StageRootMismatchError
		if !errors.As(err, &typed) {
			t.Fatalf("expected StageRootMismatchError, got %T", err)
		}
	})
}
