package restore

import (
	"os"
	"testing"
)

func TestRestoreDBRequestValidateRequiresCoreFields(t *testing.T) {
	req := RestoreDBRequest{
		ManifestPath: "/tmp/manifest.json",
		DBContainer:  "espocrm-db",
		DBName:       "espocrm",
		DBUser:       "espocrm",
	}
	if err := req.Validate(); err != nil {
		t.Fatalf("valid request failed: %v", err)
	}

	req.DBUser = "   "
	if err := req.Validate(); err == nil {
		t.Fatal("expected blank db user to fail")
	}
}

func TestRestoreFilesRequestValidateRejectsUnsafeTarget(t *testing.T) {
	tests := []string{"   ", ".", string(os.PathSeparator)}

	for _, targetDir := range tests {
		req := RestoreFilesRequest{
			ManifestPath: "/tmp/manifest.json",
			TargetDir:    targetDir,
		}
		if err := req.Validate(); err == nil {
			t.Fatalf("expected target %q to fail", targetDir)
		}
	}
}

func TestRestoreFilesRequestValidateAcceptsNormalTarget(t *testing.T) {
	req := RestoreFilesRequest{
		ManifestPath: "/tmp/manifest.json",
		TargetDir:    "/tmp/storage",
	}
	if err := req.Validate(); err != nil {
		t.Fatalf("valid request failed: %v", err)
	}
}

func TestRestoreFilesRequestValidateAcceptsDirectFilesBackup(t *testing.T) {
	req := RestoreFilesRequest{
		FilesBackup: "/tmp/files.tar.gz",
		TargetDir:   "/tmp/storage",
	}
	if err := req.Validate(); err != nil {
		t.Fatalf("valid direct files request failed: %v", err)
	}
}

func TestRestoreFilesRequestValidateRejectsAmbiguousSource(t *testing.T) {
	req := RestoreFilesRequest{
		ManifestPath: "/tmp/manifest.json",
		FilesBackup:  "/tmp/files.tar.gz",
		TargetDir:    "/tmp/storage",
	}
	if err := req.Validate(); err == nil {
		t.Fatal("expected ambiguous source to fail")
	}
}
