package backupstore

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestInspectBackupArtifact_ReturnsStructuredChecksumErrors(t *testing.T) {
	t.Run("invalid sidecar format", func(t *testing.T) {
		root := t.TempDir()
		filesPath := filepath.Join(root, "files.tar.gz")

		writeTestTarGz(t, filesPath, tarEntries{
			"storage/a.txt": "hello",
		})
		if err := os.WriteFile(filesPath+".sha256", []byte("bad sidecar\n"), 0o644); err != nil {
			t.Fatal(err)
		}

		inspection, err := inspectBackupArtifact(filesPath, "files backup", true)
		if err != nil {
			t.Fatalf("InspectBackupArtifact failed: %v", err)
		}
		if inspection.ChecksumVerified {
			t.Fatal("expected checksum to be unverified")
		}

		var typed checksumSidecarFormatError
		if !errors.As(inspection.ChecksumError, &typed) {
			t.Fatalf("expected checksumSidecarFormatError, got %T", inspection.ChecksumError)
		}
	})

	t.Run("sidecar path is directory", func(t *testing.T) {
		root := t.TempDir()
		dbPath := filepath.Join(root, "db.sql.gz")

		writeTestGzip(t, dbPath, []byte("select 1;"))
		if err := os.MkdirAll(dbPath+".sha256", 0o755); err != nil {
			t.Fatal(err)
		}

		inspection, err := inspectBackupArtifact(dbPath, "db backup", false)
		if err != nil {
			t.Fatalf("InspectBackupArtifact failed: %v", err)
		}

		var typed checksumSidecarFormatError
		if !errors.As(inspection.ChecksumError, &typed) {
			t.Fatalf("expected checksumSidecarFormatError, got %T", inspection.ChecksumError)
		}
	})
}
