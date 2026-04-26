package manifest

import (
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const testSHA = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

func TestManifestValidate(t *testing.T) {
	path := filepath.Join(t.TempDir(), ManifestName)
	m := Manifest{
		Scope:     "prod",
		CreatedAt: time.Now().UTC().Format(time.RFC3339Nano),
		DB:        Artifact{File: DBFileName, SHA256: testSHA},
		Files:     Artifact{File: FilesFileName, SHA256: testSHA},
		DBName:    "espocrm",
	}
	if err := Validate(path, m); err != nil {
		t.Fatalf("Validate failed: %v", err)
	}
}

func TestManifestValidateRejectsWrongFileName(t *testing.T) {
	path := filepath.Join(t.TempDir(), "old.manifest.json")
	m := Manifest{
		Scope:     "prod",
		CreatedAt: time.Now().UTC().Format(time.RFC3339Nano),
		DB:        Artifact{File: "prod.sql.gz", SHA256: testSHA},
		Files:     Artifact{File: "prod.tar.gz", SHA256: testSHA},
		DBName:    "espocrm",
	}
	err := Validate(path, m)
	if err == nil || !strings.Contains(err.Error(), "manifest file must be named manifest.json") {
		t.Fatalf("expected manifest name error, got %v", err)
	}
}
