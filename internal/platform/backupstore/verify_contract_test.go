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

	var verificationErr VerificationError
	if !errors.As(err, &verificationErr) {
		t.Fatalf("expected VerificationError, got %T", err)
	}

	var mismatchErr checksumMismatchError
	if !errors.As(err, &mismatchErr) {
		t.Fatalf("expected checksumMismatchError, got %T", err)
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

		var verificationErr VerificationError
		if !errors.As(err, &verificationErr) {
			t.Fatalf("expected VerificationError, got %T", err)
		}

		var sidecarErr checksumSidecarFormatError
		if !errors.As(err, &sidecarErr) {
			t.Fatalf("expected checksumSidecarFormatError, got %T", err)
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

		var verificationErr VerificationError
		if !errors.As(err, &verificationErr) {
			t.Fatalf("expected VerificationError, got %T", err)
		}

		var archiveErr archiveValidationError
		if !errors.As(err, &archiveErr) {
			t.Fatalf("expected archiveValidationError, got %T", err)
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

	var verificationErr VerificationError
	if !errors.As(err, &verificationErr) {
		t.Fatalf("expected VerificationError, got %T", err)
	}

	var archiveErr archiveValidationError
	if !errors.As(err, &archiveErr) {
		t.Fatalf("expected archiveValidationError, got %T", err)
	}
}

func TestVerifyManifestDetailed_RejectsArtifactGroupDrift(t *testing.T) {
	root := t.TempDir()

	manifestPath, dbPath, filesPath := writeBackupstoreSet(t, root, tarEntries{
		"storage/a.txt": "hello",
	})
	driftFilesPath := filepath.Join(root, "files", "espocrm-prod_files_2026-04-07_02-00-00.tar.gz")
	if err := os.Rename(filesPath, driftFilesPath); err != nil {
		t.Fatal(err)
	}
	if err := WriteManifest(manifestPath, mustBuildManifest(t,
		filepath.Base(dbPath),
		filepath.Base(driftFilesPath),
		sha256OfFile(t, dbPath),
		sha256OfFile(t, driftFilesPath),
	)); err != nil {
		t.Fatal(err)
	}

	_, err := VerifyManifestDetailed(manifestPath)
	if err == nil {
		t.Fatal("expected manifest coherence drift")
	}

	var verificationErr VerificationError
	if !errors.As(err, &verificationErr) {
		t.Fatalf("expected VerificationError, got %T", err)
	}

	var coherenceErr manifestCoherenceError
	if !errors.As(err, &coherenceErr) {
		t.Fatalf("expected manifestCoherenceError, got %T", err)
	}
}

func TestVerifyManifestDetailed_AllowsNonCanonicalManifestPathWhenArtifactsStayCoherent(t *testing.T) {
	root := t.TempDir()
	dbName := "db_20260415_123456.sql.gz"
	filesName := "files_20260415_123456.tar.gz"
	dbPath := filepath.Join(root, "db", dbName)
	filesPath := filepath.Join(root, "files", filesName)
	manifestPath := filepath.Join(root, "manifests", "latest.json")

	writeTestGzip(t, dbPath, []byte("select 1;"))
	writeTestTarGz(t, filesPath, tarEntries{
		"storage/a.txt": "hello",
	})
	if err := WriteManifest(manifestPath, mustBuildManifest(t,
		dbName,
		filesName,
		sha256OfFile(t, dbPath),
		sha256OfFile(t, filesPath),
	)); err != nil {
		t.Fatal(err)
	}

	info, err := VerifyManifestDetailed(manifestPath)
	if err != nil {
		t.Fatalf("VerifyManifestDetailed failed: %v", err)
	}
	if info.ManifestPath != manifestPath {
		t.Fatalf("unexpected manifest path: %s", info.ManifestPath)
	}
	if info.DBBackupPath != dbPath || info.FilesPath != filesPath {
		t.Fatalf("unexpected artifact paths: %#v", info)
	}
}

func TestVerifyManifestAndDirectFilesShareArchiveValidationCause(t *testing.T) {
	root := t.TempDir()

	manifestPath, _, filesPath := writeBackupstoreSet(t, root, tarEntries{
		"storage/a.txt": "hello",
		"storage/link":  symlinkEntry("../escape"),
	})
	writeTestSidecar(t, filesPath)

	directErr := VerifyDirectFilesBackup(filesPath)
	if directErr == nil {
		t.Fatal("expected direct verification failure")
	}

	_, manifestErr := VerifyManifestDetailed(manifestPath)
	if manifestErr == nil {
		t.Fatal("expected manifest verification failure")
	}

	assertArchiveVerificationCause(t, directErr)
	assertArchiveVerificationCause(t, manifestErr)
}

func TestVerifyManifestAndDirectDBShareArchiveValidationCause(t *testing.T) {
	root := t.TempDir()
	dbPath := filepath.Join(root, "db", "espocrm-prod_2026-04-07_01-00-00.sql.gz")
	filesPath := filepath.Join(root, "files", "espocrm-prod_files_2026-04-07_01-00-00.tar.gz")
	manifestPath := filepath.Join(root, "manifests", "espocrm-prod_2026-04-07_01-00-00.manifest.json")

	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dbPath, []byte("not gzip"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeTestSidecar(t, dbPath)
	writeTestTarGz(t, filesPath, tarEntries{
		"storage/a.txt": "hello",
	})
	if err := WriteManifest(manifestPath, mustBuildManifest(t,
		filepath.Base(dbPath),
		filepath.Base(filesPath),
		sha256OfFile(t, dbPath),
		sha256OfFile(t, filesPath),
	)); err != nil {
		t.Fatal(err)
	}

	directErr := VerifyDirectDBBackup(dbPath)
	if directErr == nil {
		t.Fatal("expected direct verification failure")
	}

	_, manifestErr := VerifyManifestDetailed(manifestPath)
	if manifestErr == nil {
		t.Fatal("expected manifest verification failure")
	}

	assertArchiveVerificationCause(t, directErr)
	assertArchiveVerificationCause(t, manifestErr)
}

func assertArchiveVerificationCause(t *testing.T, err error) {
	t.Helper()

	var verificationErr VerificationError
	if !errors.As(err, &verificationErr) {
		t.Fatalf("expected VerificationError, got %T", err)
	}

	var archiveErr archiveValidationError
	if !errors.As(err, &archiveErr) {
		t.Fatalf("expected archiveValidationError, got %T", err)
	}
}
