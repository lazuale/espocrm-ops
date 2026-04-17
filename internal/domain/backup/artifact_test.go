package backup

import (
	"path/filepath"
	"testing"
)

func TestBuildBackupSet(t *testing.T) {
	root := filepath.Join(t.TempDir(), "backups")

	set := BuildBackupSet(root, "espocrm-dev", "2026-04-07_01-02-03")

	if set.Group.Prefix != "espocrm-dev" {
		t.Fatalf("unexpected prefix: %s", set.Group.Prefix)
	}
	if set.Group.Stamp != "2026-04-07_01-02-03" {
		t.Fatalf("unexpected stamp: %s", set.Group.Stamp)
	}
	if set.DBBackup.Path != filepath.Join(root, "db", "espocrm-dev_2026-04-07_01-02-03.sql.gz") {
		t.Fatalf("unexpected db path: %s", set.DBBackup.Path)
	}
	if set.FilesBackup.Path != filepath.Join(root, "files", "espocrm-dev_files_2026-04-07_01-02-03.tar.gz") {
		t.Fatalf("unexpected files path: %s", set.FilesBackup.Path)
	}
	if set.ManifestTXT.Path != filepath.Join(root, "manifests", "espocrm-dev_2026-04-07_01-02-03.manifest.txt") {
		t.Fatalf("unexpected txt manifest path: %s", set.ManifestTXT.Path)
	}
	if set.ManifestJSON.Path != filepath.Join(root, "manifests", "espocrm-dev_2026-04-07_01-02-03.manifest.json") {
		t.Fatalf("unexpected json manifest path: %s", set.ManifestJSON.Path)
	}
}

func TestParseBackupArtifactNames(t *testing.T) {
	tests := []struct {
		name     string
		parse    func(string) (BackupGroup, error)
		prefix   string
		stamp    string
		wantFail bool
	}{
		{
			name:   "/tmp/espocrm-dev_2026-04-07_01-02-03.sql.gz",
			parse:  ParseDBBackupName,
			prefix: "espocrm-dev",
			stamp:  "2026-04-07_01-02-03",
		},
		{
			name:   "/tmp/espocrm-dev_files_2026-04-07_01-02-03.tar.gz",
			parse:  ParseFilesBackupName,
			prefix: "espocrm-dev",
			stamp:  "2026-04-07_01-02-03",
		},
		{
			name:     "/tmp/espocrm-dev_2026-04-07_01-02-03.tar.gz",
			parse:    ParseFilesBackupName,
			wantFail: true,
		},
	}

	for _, tt := range tests {
		t.Run(filepath.Base(tt.name), func(t *testing.T) {
			group, err := tt.parse(tt.name)
			if tt.wantFail {
				if err == nil {
					t.Fatal("expected parse error")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if group.Prefix != tt.prefix || group.Stamp != tt.stamp {
				t.Fatalf("unexpected group: %#v", group)
			}
		})
	}
}

func TestParseManifestName(t *testing.T) {
	group, kind, err := ParseManifestName("/tmp/espocrm-prod_2026-04-07_01-02-03.manifest.json")
	if err != nil {
		t.Fatal(err)
	}
	if group.Prefix != "espocrm-prod" || group.Stamp != "2026-04-07_01-02-03" {
		t.Fatalf("unexpected group: %#v", group)
	}
	if kind != ManifestJSON {
		t.Fatalf("unexpected kind: %s", kind)
	}

	_, _, err = ParseManifestName("/tmp/espocrm-prod_2026-04-07_01-02-03.json")
	if err == nil {
		t.Fatal("expected parse error")
	}
}

func TestResolveManifestArtifactPath(t *testing.T) {
	root := filepath.Join(t.TempDir(), "backups")

	got := ResolveManifestArtifactPath(filepath.Join(root, "manifests", "set.manifest.json"), "db", "db.sql.gz")
	if got != filepath.Join(root, "db", "db.sql.gz") {
		t.Fatalf("unexpected manifest-dir artifact path: %s", got)
	}

	got = ResolveManifestArtifactPath(filepath.Join(root, "set.manifest.json"), "files", "files.tar.gz")
	if got != filepath.Join(root, "files.tar.gz") {
		t.Fatalf("unexpected adjacent artifact path: %s", got)
	}
}
