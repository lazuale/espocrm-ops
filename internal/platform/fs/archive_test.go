package fs

import (
	"archive/tar"
	"compress/gzip"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestUnpackTarGz_ReturnsTypedArchiveReadErrorForInvalidGzip(t *testing.T) {
	tmp := t.TempDir()
	archivePath := filepath.Join(tmp, "broken.tar.gz")
	destDir := filepath.Join(tmp, "dest")

	if err := os.WriteFile(archivePath, []byte("not gzip"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		t.Fatal(err)
	}

	err := UnpackTarGz(archivePath, destDir, nil)
	if err == nil {
		t.Fatal("expected invalid gzip to fail")
	}

	var typed archiveReadError
	if !errors.As(err, &typed) {
		t.Fatalf("expected archiveReadError, got %T", err)
	}
}

func TestUnpackTarGz_ReturnsTypedSemanticErrors(t *testing.T) {
	t.Run("empty archive", func(t *testing.T) {
		tmp := t.TempDir()
		archivePath := filepath.Join(tmp, "empty.tar.gz")
		destDir := filepath.Join(tmp, "dest")

		writeTarGzArchive(t, archivePath)
		if err := os.MkdirAll(destDir, 0o755); err != nil {
			t.Fatal(err)
		}

		err := UnpackTarGz(archivePath, destDir, nil)
		if err == nil {
			t.Fatal("expected empty archive error")
		}

		var typed archiveEmptyError
		if !errors.As(err, &typed) {
			t.Fatalf("expected archiveEmptyError, got %T", err)
		}
	})

	t.Run("entry escapes destination", func(t *testing.T) {
		tmp := t.TempDir()
		archivePath := filepath.Join(tmp, "escape.tar.gz")
		destDir := filepath.Join(tmp, "dest")

		writeTarGzArchive(t, archivePath, tar.Header{
			Name:     "../escape.txt",
			Typeflag: tar.TypeReg,
			Mode:     0o644,
			Size:     int64(len("escape")),
		}, []byte("escape"))
		if err := os.MkdirAll(destDir, 0o755); err != nil {
			t.Fatal(err)
		}

		err := UnpackTarGz(archivePath, destDir, nil)
		if err == nil {
			t.Fatal("expected escape validation error")
		}

		var typed archiveEntryEscapeError
		if !errors.As(err, &typed) {
			t.Fatalf("expected archiveEntryEscapeError, got %T", err)
		}
	})

	t.Run("unexpected entry type", func(t *testing.T) {
		tmp := t.TempDir()
		archivePath := filepath.Join(tmp, "symlink.tar.gz")
		destDir := filepath.Join(tmp, "dest")

		writeTarGzArchive(t, archivePath, tar.Header{
			Name:     "link",
			Typeflag: tar.TypeSymlink,
			Linkname: "target",
			Mode:     0o777,
		}, nil)
		if err := os.MkdirAll(destDir, 0o755); err != nil {
			t.Fatal(err)
		}

		err := UnpackTarGz(archivePath, destDir, nil)
		if err == nil {
			t.Fatal("expected unexpected type error")
		}

		var typed archiveUnexpectedEntryTypeError
		if !errors.As(err, &typed) {
			t.Fatalf("expected archiveUnexpectedEntryTypeError, got %T", err)
		}
	})

	t.Run("entry conflicts with extracted file path", func(t *testing.T) {
		tmp := t.TempDir()
		archivePath := filepath.Join(tmp, "conflict.tar.gz")
		destDir := filepath.Join(tmp, "dest")

		writeTarGzArchive(
			t,
			archivePath,
			tar.Header{
				Name:     "storage/a.txt",
				Typeflag: tar.TypeReg,
				Mode:     0o644,
				Size:     int64(len("hello")),
			}, []byte("hello"),
			tar.Header{
				Name:     "storage/a.txt/b.txt",
				Typeflag: tar.TypeReg,
				Mode:     0o644,
				Size:     int64(len("world")),
			}, []byte("world"),
		)
		if err := os.MkdirAll(destDir, 0o755); err != nil {
			t.Fatal(err)
		}

		err := UnpackTarGz(archivePath, destDir, nil)
		if err == nil {
			t.Fatal("expected archive entry conflict error")
		}

		var typed archiveEntryConflictError
		if !errors.As(err, &typed) {
			t.Fatalf("expected archiveEntryConflictError, got %T", err)
		}
	})

	t.Run("destination symlink path", func(t *testing.T) {
		tmp := t.TempDir()
		archivePath := filepath.Join(tmp, "symlink-conflict.tar.gz")
		destDir := filepath.Join(tmp, "dest")
		outsideDir := filepath.Join(tmp, "outside")

		writeTarGzArchive(t, archivePath, tar.Header{
			Name:     "storage/file.txt",
			Typeflag: tar.TypeReg,
			Mode:     0o644,
			Size:     int64(len("payload")),
		}, []byte("payload"))
		if err := os.MkdirAll(destDir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(outsideDir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(outsideDir, filepath.Join(destDir, "storage")); err != nil {
			t.Fatal(err)
		}

		err := UnpackTarGz(archivePath, destDir, nil)
		if err == nil {
			t.Fatal("expected symlink conflict error")
		}

		var typed archiveEntryConflictError
		if !errors.As(err, &typed) {
			t.Fatalf("expected archiveEntryConflictError, got %T", err)
		}
		if typed.ConflictPath != filepath.Join(destDir, "storage") {
			t.Fatalf("unexpected conflict path: %s", typed.ConflictPath)
		}
		if _, err := os.Stat(filepath.Join(outsideDir, "file.txt")); !os.IsNotExist(err) {
			t.Fatalf("expected archive extraction to stay inside destination, got: %v", err)
		}
	})

	t.Run("legacy regular file entry", func(t *testing.T) {
		tmp := t.TempDir()
		archivePath := filepath.Join(tmp, "legacy-regular.tar.gz")
		destDir := filepath.Join(tmp, "dest")

		writeTarGzArchive(t, archivePath, tar.Header{
			Name:     "storage/file.txt",
			Typeflag: legacyTarRegularType,
			Mode:     0o644,
			Size:     int64(len("legacy")),
		}, []byte("legacy"))
		if err := os.MkdirAll(destDir, 0o755); err != nil {
			t.Fatal(err)
		}

		if err := UnpackTarGz(archivePath, destDir, nil); err != nil {
			t.Fatalf("expected legacy regular file type to unpack: %v", err)
		}
		if _, err := os.Stat(filepath.Join(destDir, "storage", "file.txt")); err != nil {
			t.Fatalf("expected unpacked file to exist: %v", err)
		}
	})
}

func writeTarGzArchive(t *testing.T, path string, entries ...any) {
	t.Helper()

	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer closeTestArchiveWriter(t, "archive file", f)

	gz := gzip.NewWriter(f)
	defer closeTestArchiveWriter(t, "archive gzip writer", gz)

	tw := tar.NewWriter(gz)
	defer closeTestArchiveWriter(t, "archive tar writer", tw)

	for i := 0; i < len(entries); i += 2 {
		hdr, ok := entries[i].(tar.Header)
		if !ok {
			t.Fatalf("entry %d header has type %T", i, entries[i])
		}
		if err := tw.WriteHeader(&hdr); err != nil {
			t.Fatal(err)
		}
		if body, _ := entries[i+1].([]byte); len(body) != 0 {
			if _, err := tw.Write(body); err != nil {
				t.Fatal(err)
			}
		}
	}
}

func closeTestArchiveWriter(t *testing.T, label string, closer interface{ Close() error }) {
	t.Helper()

	if err := closer.Close(); err != nil {
		t.Fatalf("close %s: %v", label, err)
	}
}
