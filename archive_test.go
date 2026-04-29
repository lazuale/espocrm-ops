package main

import (
	"archive/tar"
	"compress/gzip"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateFilesArchiveRejectsAbsolutePath(t *testing.T) {
	path := writeTestArchive(t, []testTarEntry{{Name: "/absolute.txt", Body: "no"}})
	if err := validateFilesArchive(path); err == nil {
		t.Fatal("expected absolute archive path rejected")
	}
}

func TestValidateFilesArchiveRejectsTraversal(t *testing.T) {
	path := writeTestArchive(t, []testTarEntry{{Name: "../escape.txt", Body: "no"}})
	if err := validateFilesArchive(path); err == nil {
		t.Fatal("expected traversal archive path rejected")
	}
}

func TestValidateFilesArchiveRejectsSymlink(t *testing.T) {
	path := writeTestArchive(t, []testTarEntry{{Name: "link", Typeflag: tar.TypeSymlink, Linkname: "file"}})
	if err := validateFilesArchive(path); err == nil {
		t.Fatal("expected symlink archive entry rejected")
	}
}

func TestValidateFilesArchiveAcceptsNormalArchive(t *testing.T) {
	path := writeTestArchive(t, []testTarEntry{
		{Name: "data/", Typeflag: tar.TypeDir},
		{Name: "data/file.txt", Body: "ok"},
	})
	if err := validateFilesArchive(path); err != nil {
		t.Fatalf("expected normal archive accepted: %v", err)
	}
}

func TestValidateFilesArchiveRejectsCorruptGzipFooter(t *testing.T) {
	path := writeTestArchive(t, []testTarEntry{
		{Name: "data/", Typeflag: tar.TypeDir},
		{Name: "data/file.txt", Body: "ok"},
	})
	corruptGzipFooter(t, path)

	err := validateFilesArchive(path)
	if err == nil || !strings.Contains(err.Error(), "read files archive gzip"+" footer") {
		t.Fatalf("expected gzip"+" footer error, got %v", err)
	}
}

func TestExtractFilesArchiveRejectsEscapes(t *testing.T) {
	path := writeTestArchive(t, []testTarEntry{{Name: "../escape.txt", Body: "no"}})
	target := filepath.Join(t.TempDir(), "target")
	if err := os.Mkdir(target, 0700); err != nil {
		t.Fatal(err)
	}
	if err := extractFilesArchive(path, target); err == nil {
		t.Fatal("expected unsafe extraction rejected")
	}
}

func TestExtractFilesArchiveRejectsCorruptGzipFooter(t *testing.T) {
	path := writeTestArchive(t, []testTarEntry{
		{Name: "data/", Typeflag: tar.TypeDir},
		{Name: "data/file.txt", Body: "ok"},
	})
	corruptGzipFooter(t, path)
	target := filepath.Join(t.TempDir(), "target")
	if err := os.Mkdir(target, 0700); err != nil {
		t.Fatal(err)
	}

	err := extractFilesArchive(path, target)
	if err == nil || !strings.Contains(err.Error(), "read files archive gzip"+" footer") {
		t.Fatalf("expected gzip"+" footer error, got %v", err)
	}
}

type testTarEntry struct {
	Name     string
	Body     string
	Typeflag byte
	Linkname string
}

func writeTestArchive(t *testing.T, entries []testTarEntry) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), filesFileName)
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	gz := gzip.NewWriter(file)
	tw := tar.NewWriter(gz)
	for _, entry := range entries {
		typeflag := entry.Typeflag
		if typeflag == 0 {
			typeflag = tar.TypeReg
		}
		mode := int64(0600)
		if typeflag == tar.TypeDir {
			mode = 0700
		}
		header := &tar.Header{
			Name:     entry.Name,
			Mode:     mode,
			Typeflag: typeflag,
			Linkname: entry.Linkname,
		}
		if typeflag == tar.TypeReg {
			header.Size = int64(len(entry.Body))
		}
		if err := tw.WriteHeader(header); err != nil {
			t.Fatal(err)
		}
		if typeflag == tar.TypeReg {
			if _, err := tw.Write([]byte(entry.Body)); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	return path
}

func corruptGzipFooter(t *testing.T, path string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) < 8 {
		t.Fatalf("compressed archive too short: %d bytes", len(data))
	}
	data[len(data)-1] ^= 0xff
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
}
