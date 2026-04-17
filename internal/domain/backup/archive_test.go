package backup

import (
	"archive/tar"
	"testing"
)

func TestValidateFilesArchiveHeaderRejectsUnsafeEntries(t *testing.T) {
	tests := []tar.Header{
		{Name: "/abs/path.txt", Typeflag: tar.TypeReg},
		{Name: "../escape.txt", Typeflag: tar.TypeReg},
		{Name: "safe/link", Typeflag: tar.TypeSymlink},
		{Name: "safe/hard", Typeflag: tar.TypeLink},
	}

	for _, tt := range tests {
		if err := ValidateFilesArchiveHeader(&tt); err == nil {
			t.Fatalf("expected error for %#v", tt)
		}
	}
}

func TestValidateFilesArchiveHeaderAllowsRegularTreeEntries(t *testing.T) {
	tests := []tar.Header{
		{Name: "storage", Typeflag: tar.TypeDir},
		{Name: "storage/file.txt", Typeflag: tar.TypeReg},
		{Name: "./storage/file.txt", Typeflag: tar.TypeRegA},
	}

	for _, tt := range tests {
		if err := ValidateFilesArchiveHeader(&tt); err != nil {
			t.Fatalf("unexpected error for %#v: %v", tt, err)
		}
	}
}
