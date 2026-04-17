package backupstore

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestVerifyManifestDetailed_ReturnsTypedChecksumMismatch(t *testing.T) {
	root := t.TempDir()

	manifestPath, dbPath, filesPath := writeBackupstoreSet(t, root, tarEntries{
		"storage/a.txt": "hello",
	})
	if err := WriteManifest(manifestPath, mustBuildManifest(t,
		filepath.Base(dbPath),
		filepath.Base(filesPath),
		strings.Repeat("0", 64),
		sha256OfFile(t, filesPath),
	)); err != nil {
		t.Fatal(err)
	}

	_, err := VerifyManifestDetailed(manifestPath)
	if err == nil {
		t.Fatal("expected checksum mismatch")
	}

	var validationErr ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("expected ValidationError, got %T", err)
	}

	var mismatchErr ChecksumMismatchError
	if !errors.As(err, &mismatchErr) {
		t.Fatalf("expected ChecksumMismatchError, got %T", err)
	}
}

func TestVerifyDirectFilesBackup_ReturnsTypedValidationErrors(t *testing.T) {
	t.Run("invalid checksum sidecar", func(t *testing.T) {
		root := t.TempDir()
		filesPath := filepath.Join(root, "files.tar.gz")

		writeTestTarGz(t, filesPath, tarEntries{
			"storage/a.txt": "hello",
		})
		if err := os.WriteFile(filesPath+".sha256", []byte("bad-digest  files.tar.gz\n"), 0o644); err != nil {
			t.Fatal(err)
		}

		err := VerifyDirectFilesBackup(filesPath)
		if err == nil {
			t.Fatal("expected invalid sidecar to fail")
		}

		var validationErr ValidationError
		if !errors.As(err, &validationErr) {
			t.Fatalf("expected ValidationError, got %T", err)
		}

		var sidecarErr ChecksumSidecarFormatError
		if !errors.As(err, &sidecarErr) {
			t.Fatalf("expected ChecksumSidecarFormatError, got %T", err)
		}
	})

	t.Run("unsafe archive", func(t *testing.T) {
		root := t.TempDir()
		filesPath := filepath.Join(root, "files.tar.gz")

		writeTestTarGz(t, filesPath, tarEntries{
			"storage/a.txt": "hello",
			"storage/link":  symlinkEntry("../escape"),
		})
		writeTestSidecar(t, filesPath)

		err := VerifyDirectFilesBackup(filesPath)
		if err == nil {
			t.Fatal("expected unsafe archive to fail")
		}

		var validationErr ValidationError
		if !errors.As(err, &validationErr) {
			t.Fatalf("expected ValidationError, got %T", err)
		}

		var archiveErr ArchiveValidationError
		if !errors.As(err, &archiveErr) {
			t.Fatalf("expected ArchiveValidationError, got %T", err)
		}
	})
}

func TestVerifyDirectDBBackup_ReturnsTypedArchiveValidationError(t *testing.T) {
	root := t.TempDir()
	dbPath := filepath.Join(root, "db.sql.gz")

	if err := os.WriteFile(dbPath, []byte("not gzip"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeTestSidecar(t, dbPath)

	err := VerifyDirectDBBackup(dbPath)
	if err == nil {
		t.Fatal("expected invalid gzip to fail")
	}

	var validationErr ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("expected ValidationError, got %T", err)
	}

	var archiveErr ArchiveValidationError
	if !errors.As(err, &archiveErr) {
		t.Fatalf("expected ArchiveValidationError, got %T", err)
	}
}
