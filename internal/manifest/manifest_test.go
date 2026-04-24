package manifest

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadValidateAndResolveArtifacts(t *testing.T) {
	root := t.TempDir()
	manifestPath := filepath.Join(root, "manifests", "set.manifest.json")
	if err := os.MkdirAll(filepath.Dir(manifestPath), 0o755); err != nil {
		t.Fatal(err)
	}

	raw, err := json.Marshal(map[string]any{
		"version":    1,
		"scope":      "prod",
		"created_at": "2026-04-24T12:00:00Z",
		"artifacts": map[string]any{
			"db_backup":    "db.sql.gz",
			"files_backup": "files.tar.gz",
		},
		"checksums": map[string]any{
			"db_backup":    "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			"files_backup": "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manifestPath, raw, 0o644); err != nil {
		t.Fatal(err)
	}

	manifest, err := Load(manifestPath)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if err := Validate(manifestPath, manifest); err != nil {
		t.Fatalf("Validate failed: %v", err)
	}

	paths, err := ResolveArtifacts(manifestPath, manifest)
	if err != nil {
		t.Fatalf("ResolveArtifacts failed: %v", err)
	}
	if paths.DBPath != filepath.Join(root, "db", "db.sql.gz") {
		t.Fatalf("unexpected db path: %s", paths.DBPath)
	}
	if paths.FilesPath != filepath.Join(root, "files", "files.tar.gz") {
		t.Fatalf("unexpected files path: %s", paths.FilesPath)
	}
	if paths.DBSidecarPath != paths.DBPath+".sha256" {
		t.Fatalf("unexpected db sidecar: %s", paths.DBSidecarPath)
	}
	if paths.FilesSidecarPath != paths.FilesPath+".sha256" {
		t.Fatalf("unexpected files sidecar: %s", paths.FilesSidecarPath)
	}
}

func TestValidateRejectsInvalidManifest(t *testing.T) {
	manifest := Manifest{
		Version:   2,
		Scope:     "",
		CreatedAt: "nope",
	}

	if err := Validate("/tmp/set.manifest.json", manifest); err == nil {
		t.Fatal("expected validation error")
	}
}

func TestResolveArtifactsRejectsManifestOutsideManifestsDirectory(t *testing.T) {
	paths, err := ResolveArtifacts("/tmp/set.manifest.json", Manifest{
		Artifacts: Artifacts{
			DBBackup:    "db.sql.gz",
			FilesBackup: "files.tar.gz",
		},
	})
	if err == nil {
		t.Fatal("expected resolve error")
	}
	if err.Error() != "manifest must be located in manifests directory" {
		t.Fatalf("unexpected error: %v", err)
	}
	if paths != (ArtifactPaths{}) {
		t.Fatalf("expected zero paths, got %#v", paths)
	}
}
