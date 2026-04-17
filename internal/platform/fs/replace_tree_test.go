package fs

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestReplaceTree_ReplacesExistingDirectory(t *testing.T) {
	tmp := t.TempDir()

	target := filepath.Join(tmp, "storage")
	prepared := filepath.Join(tmp, "prepared")

	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(target, "old.txt"), []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := os.MkdirAll(prepared, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(prepared, "new.txt"), []byte("new"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := ReplaceTree(target, prepared); err != nil {
		t.Fatalf("ReplaceTree failed: %v", err)
	}

	if _, err := os.Stat(filepath.Join(target, "new.txt")); err != nil {
		t.Fatalf("expected new file in target: %v", err)
	}
	if _, err := os.Stat(filepath.Join(target, "old.txt")); !os.IsNotExist(err) {
		t.Fatalf("expected old file to be gone, got: %v", err)
	}
}

func TestReplaceTree_CreatesMissingTarget(t *testing.T) {
	tmp := t.TempDir()

	target := filepath.Join(tmp, "storage")
	prepared := filepath.Join(tmp, "prepared")

	if err := os.MkdirAll(prepared, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(prepared, "new.txt"), []byte("new"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := ReplaceTree(target, prepared); err != nil {
		t.Fatalf("ReplaceTree failed: %v", err)
	}

	if _, err := os.Stat(filepath.Join(target, "new.txt")); err != nil {
		t.Fatalf("expected new file in target: %v", err)
	}
}

func TestReplaceTree_RejectsPreparedFile(t *testing.T) {
	tmp := t.TempDir()

	target := filepath.Join(tmp, "storage")
	prepared := filepath.Join(tmp, "prepared-file")

	if err := os.WriteFile(prepared, []byte("not a dir"), 0o644); err != nil {
		t.Fatal(err)
	}

	err := ReplaceTree(target, prepared)
	if err == nil {
		t.Fatal("expected prepared dir validation error")
	}

	var typed PreparedDirNotDirectoryError
	if !errors.As(err, &typed) {
		t.Fatalf("expected PreparedDirNotDirectoryError, got %T", err)
	}
}

func TestReplaceTree_ReturnsTypedErrorWhenTargetParentCannotBeEnsured(t *testing.T) {
	tmp := t.TempDir()

	blocker := filepath.Join(tmp, "blocker")
	target := filepath.Join(blocker, "storage")
	prepared := filepath.Join(tmp, "prepared")

	if err := os.WriteFile(blocker, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(prepared, 0o755); err != nil {
		t.Fatal(err)
	}

	err := ReplaceTree(target, prepared)
	if err == nil {
		t.Fatal("expected ensure parent dir error")
	}

	var typed EnsureDirError
	if !errors.As(err, &typed) {
		t.Fatalf("expected EnsureDirError, got %T", err)
	}
}
