package manifest

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestManifestValid(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "manifests", "set.manifest.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}

	raw, err := json.Marshal(map[string]any{
		"version":    2,
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
		"runtime": map[string]any{
			"espo_crm_image":     "espocrm/espocrm:9.3.4-apache",
			"mariadb_image":      "mariadb:11.4",
			"db_name":            "espocrm",
			"db_service":         "db",
			"app_services":       []string{"espocrm", "espocrm-daemon", "espocrm-websocket"},
			"backup_name_prefix": "espocrm-prod",
			"storage_contract":   StorageContractEspoCRMFullStorageV1,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatal(err)
	}

	manifest, err := Load(path)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if err := Validate(path, manifest); err != nil {
		t.Fatalf("Validate failed: %v", err)
	}

	paths, err := ResolveArtifacts(path, manifest)
	if err != nil {
		t.Fatalf("ResolveArtifacts failed: %v", err)
	}
	if paths.DBPath != filepath.Join(root, "db", "db.sql.gz") {
		t.Fatalf("unexpected db path: %s", paths.DBPath)
	}
	if paths.FilesPath != filepath.Join(root, "files", "files.tar.gz") {
		t.Fatalf("unexpected files path: %s", paths.FilesPath)
	}
}

func TestManifestVersionOneStillValidForVerify(t *testing.T) {
	manifest := Manifest{
		Version:   VersionOne,
		Scope:     "prod",
		CreatedAt: "2026-04-24T12:00:00Z",
		Artifacts: Artifacts{
			DBBackup:    "db.sql.gz",
			FilesBackup: "files.tar.gz",
		},
		Checksums: Checksums{
			DBBackup:    "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			FilesBackup: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		},
	}
	if err := Validate("/tmp/manifests/set.manifest.json", manifest); err != nil {
		t.Fatalf("Validate failed: %v", err)
	}
	if err := RequireRestoreRuntimeContract(manifest); err == nil {
		t.Fatal("expected restore runtime contract error")
	}
}

func TestManifestInvalid(t *testing.T) {
	manifest := Manifest{
		Version:   VersionCurrent,
		Scope:     "",
		CreatedAt: "nope",
	}
	if err := Validate("/tmp/manifests/set.manifest.json", manifest); err == nil {
		t.Fatal("expected validation error")
	}
}

func TestManifestVersionTwoRequiresRuntime(t *testing.T) {
	manifest := Manifest{
		Version:   VersionCurrent,
		Scope:     "prod",
		CreatedAt: "2026-04-24T12:00:00Z",
		Artifacts: Artifacts{
			DBBackup:    "db.sql.gz",
			FilesBackup: "files.tar.gz",
		},
		Checksums: Checksums{
			DBBackup:    "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			FilesBackup: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		},
	}
	if err := Validate("/tmp/manifests/set.manifest.json", manifest); err == nil {
		t.Fatal("expected validation error")
	}
}

func TestManifestOutsideManifestsDirectory(t *testing.T) {
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
